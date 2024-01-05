// Copyright 2019 Intel Corporation. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package client

import (
	"context"
	"fmt"
	"net"
	"os"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"

	criv1 "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/intel/cri-resource-manager/pkg/instrumentation"
	logger "github.com/intel/cri-resource-manager/pkg/log"
	"github.com/intel/cri-resource-manager/pkg/utils"

	v1 "github.com/intel/cri-resource-manager/pkg/cri/client/v1"
	v1alpha2 "github.com/intel/cri-resource-manager/pkg/cri/client/v1alpha2"
)

// DialNotifyFn is a function to call after a successful net.Dial[Timeout]().
type DialNotifyFn func(string, int, int, os.FileMode, error)

// Options contains the configurable options of our CRI client.
type Options struct {
	// ImageSocket is the socket path for the CRI image service.
	ImageSocket string
	// RuntimeSocket is the socket path for the CRI runtime service.
	RuntimeSocket string
	// DialNotify is an optional function to notify after net.Dial returns for a socket.
	DialNotify DialNotifyFn
}

// ConnectOptions contains options for connecting to the server.
type ConnectOptions struct {
	// Wait indicates whether Connect() should wait (indefinitely) for the server.
	Wait bool
	// Reconnect indicates whether CheckConnection() should attempt to Connect().
	Reconnect bool
}

// Client is the interface we expose to our CRI client.
type Client interface {
	// Connect tries to connect the client to the specified image and runtime services.
	Connect(ConnectOptions) error
	// Close closes any existing client connections.
	Close()
	// CheckConnection checks if we have (un-Close()'d as opposed to working) connections.
	CheckConnection(ConnectOptions) error
	// HasRuntimeService checks if the client is configured with runtime services.
	HasRuntimeService() bool

	// We expose full image and runtime client services.
	criv1.ImageServiceClient
	criv1.RuntimeServiceClient
}

type criClient interface {
	criv1.ImageServiceClient
	criv1.RuntimeServiceClient
}

// client is the implementation of Client.
type client struct {
	logger.Logger
	criv1.ImageServiceClient
	criv1.RuntimeServiceClient
	options Options          // client options
	icc     *grpc.ClientConn // our gRPC connection to the image service
	rcc     *grpc.ClientConn // our gRPC connection to the runtime service

	client criClient
}

const (
	// DontConnect is used to mark a socket to not be connected.
	DontConnect = "-"
)

// NewClient creates a new client instance.
func NewClient(options Options) (Client, error) {
	if options.ImageSocket == DontConnect && options.RuntimeSocket == DontConnect {
		return nil, clientError("neither image nor runtime socket specified")
	}

	c := &client{
		Logger:  logger.NewLogger("cri/client"),
		options: options,
	}

	return c, nil
}

// Connect attempts to establish gRPC client connections to the configured services.
func (c *client) Connect(options ConnectOptions) error {
	var err error

	kind, socket := "image services", c.options.ImageSocket
	if c.icc, err = c.connect(kind, socket, options); err != nil {
		return err
	}

	kind, socket = "runtime services", c.options.RuntimeSocket
	if socket == c.options.ImageSocket {
		c.rcc = c.icc
	} else {
		if c.rcc, err = c.connect(kind, socket, options); err != nil {
			c.icc = nil
			return err
		}
	}

	client, err := v1.Connect(c.rcc, c.icc)
	if err != nil {
		client, err = v1alpha2.Connect(c.rcc, c.icc)
	}
	if err != nil {
		return err
	}

	c.client = client
	return nil
}

// Close any open service connection.
func (c *client) Close() {
	if c.icc != nil {
		c.Debug("closing image service connection...")
		c.icc.Close()
	}

	if c.rcc != nil {
		c.Debug("closing runtime service connection...")
		if c.rcc != c.icc {
			c.rcc.Close()
		}
	}

	c.icc = nil
	c.rcc = nil
}

// Check if the connecton to CRI services is up, try to reconnect if requested.
func (c *client) CheckConnection(options ConnectOptions) error {
	if (c.icc == nil || c.icc.GetState() == connectivity.Ready) &&
		(c.rcc == nil || c.rcc.GetState() == connectivity.Ready) {
		return nil
	}

	c.Close()

	if options.Reconnect {
		c.Warn("client connections are down")
		if err := c.Connect(ConnectOptions{Wait: false}); err == nil {
			return nil
		}
	}

	return clientError("client connections are down")
}

// HasRuntimeService checks if the client is configured with runtime services.
func (c *client) HasRuntimeService() bool {
	return c.options.RuntimeSocket != "" && c.options.RuntimeSocket != DontConnect
}

func (c *client) checkRuntimeService() error {
	if c.client == nil || c.rcc == nil {
		return clientError("no CRI RuntimeService client")
	}
	return nil
}

func (c *client) checkImageService() error {
	if c.client == nil || c.icc == nil {
		return clientError("no CRI ImageService client")
	}
	return nil
}

// connect attempts to create a gRPC client connection to the given socket.
func (c *client) connect(kind, socket string, options ConnectOptions) (*grpc.ClientConn, error) {
	var cc *grpc.ClientConn
	var err error

	if socket == DontConnect {
		return nil, nil
	}

	dialOpts := instrumentation.InjectGrpcClientTrace(
		grpc.WithInsecure(),
		grpc.WithBlock(),
		grpc.FailOnNonTempDialError(true),
		grpc.WithDialer(func(socket string, timeout time.Duration) (net.Conn, error) {
			conn, err := net.DialTimeout("unix", socket, timeout)
			if err != nil {
				return conn, err
			}
			c.dialNotify(socket)
			return conn, err
		}))

	if options.Wait {
		c.Info("waiting for %s on socket %s...", kind, socket)
		if err = utils.WaitForServer(socket, -1, dialOpts, &cc); err != nil {
			return nil, clientError("failed to connect to %s: %v", kind, err)
		}
	} else {
		if cc, err = grpc.Dial(socket, dialOpts...); err != nil {
			return nil, clientError("failed to connect to %s: %v", kind, err)
		}
	}

	return cc, nil
}

func (c *client) dialNotify(socket string) {
	if c.options.DialNotify == nil {
		return
	}

	info, err := os.Stat(socket)
	if err != nil {
		c.options.DialNotify(socket, -1, -1, 0, err)
		return
	}

	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		err := clientError("failed to stat socket %q: %v", socket, err)
		c.options.DialNotify(socket, -1, -1, 0, err)
		return
	}

	uid, gid := int(st.Uid), int(st.Gid)
	mode := info.Mode() & os.ModePerm
	c.options.DialNotify(socket, uid, gid, mode, nil)
}

func (c *client) Version(ctx context.Context, in *criv1.VersionRequest, _ ...grpc.CallOption) (*criv1.VersionResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.client.Version(ctx, in)
}

func (c *client) RunPodSandbox(ctx context.Context, in *criv1.RunPodSandboxRequest, _ ...grpc.CallOption) (*criv1.RunPodSandboxResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.client.RunPodSandbox(ctx, in)
}

func (c *client) StopPodSandbox(ctx context.Context, in *criv1.StopPodSandboxRequest, _ ...grpc.CallOption) (*criv1.StopPodSandboxResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.client.StopPodSandbox(ctx, in)
}

func (c *client) RemovePodSandbox(ctx context.Context, in *criv1.RemovePodSandboxRequest, _ ...grpc.CallOption) (*criv1.RemovePodSandboxResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.client.RemovePodSandbox(ctx, in)
}

func (c *client) PodSandboxStatus(ctx context.Context, in *criv1.PodSandboxStatusRequest, _ ...grpc.CallOption) (*criv1.PodSandboxStatusResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.client.PodSandboxStatus(ctx, in)
}

func (c *client) ListPodSandbox(ctx context.Context, in *criv1.ListPodSandboxRequest, _ ...grpc.CallOption) (*criv1.ListPodSandboxResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.client.ListPodSandbox(ctx, in)
}

func (c *client) CreateContainer(ctx context.Context, in *criv1.CreateContainerRequest, _ ...grpc.CallOption) (*criv1.CreateContainerResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.client.CreateContainer(ctx, in)
}

func (c *client) StartContainer(ctx context.Context, in *criv1.StartContainerRequest, _ ...grpc.CallOption) (*criv1.StartContainerResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.client.StartContainer(ctx, in)
}

func (c *client) StopContainer(ctx context.Context, in *criv1.StopContainerRequest, _ ...grpc.CallOption) (*criv1.StopContainerResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.client.StopContainer(ctx, in)
}

func (c *client) RemoveContainer(ctx context.Context, in *criv1.RemoveContainerRequest, _ ...grpc.CallOption) (*criv1.RemoveContainerResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.client.RemoveContainer(ctx, in)
}

func (c *client) ListContainers(ctx context.Context, in *criv1.ListContainersRequest, _ ...grpc.CallOption) (*criv1.ListContainersResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.client.ListContainers(ctx, in)
}

func (c *client) ContainerStatus(ctx context.Context, in *criv1.ContainerStatusRequest, _ ...grpc.CallOption) (*criv1.ContainerStatusResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.client.ContainerStatus(ctx, in)
}

func (c *client) UpdateContainerResources(ctx context.Context, in *criv1.UpdateContainerResourcesRequest, _ ...grpc.CallOption) (*criv1.UpdateContainerResourcesResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.client.UpdateContainerResources(ctx, in)
}

func (c *client) ReopenContainerLog(ctx context.Context, in *criv1.ReopenContainerLogRequest, _ ...grpc.CallOption) (*criv1.ReopenContainerLogResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.client.ReopenContainerLog(ctx, in)
}

func (c *client) ExecSync(ctx context.Context, in *criv1.ExecSyncRequest, _ ...grpc.CallOption) (*criv1.ExecSyncResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.client.ExecSync(ctx, in)
}

func (c *client) Exec(ctx context.Context, in *criv1.ExecRequest, _ ...grpc.CallOption) (*criv1.ExecResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.client.Exec(ctx, in)
}

func (c *client) Attach(ctx context.Context, in *criv1.AttachRequest, _ ...grpc.CallOption) (*criv1.AttachResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.client.Attach(ctx, in)
}

func (c *client) PortForward(ctx context.Context, in *criv1.PortForwardRequest, _ ...grpc.CallOption) (*criv1.PortForwardResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.client.PortForward(ctx, in)
}

func (c *client) ContainerStats(ctx context.Context, in *criv1.ContainerStatsRequest, _ ...grpc.CallOption) (*criv1.ContainerStatsResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.client.ContainerStats(ctx, in)
}

func (c *client) ListContainerStats(ctx context.Context, in *criv1.ListContainerStatsRequest, _ ...grpc.CallOption) (*criv1.ListContainerStatsResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.client.ListContainerStats(ctx, in)
}

func (c *client) PodSandboxStats(ctx context.Context, in *criv1.PodSandboxStatsRequest, _ ...grpc.CallOption) (*criv1.PodSandboxStatsResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.client.PodSandboxStats(ctx, in)
}

func (c *client) ListPodSandboxStats(ctx context.Context, in *criv1.ListPodSandboxStatsRequest, _ ...grpc.CallOption) (*criv1.ListPodSandboxStatsResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.client.ListPodSandboxStats(ctx, in)
}

func (c *client) UpdateRuntimeConfig(ctx context.Context, in *criv1.UpdateRuntimeConfigRequest, _ ...grpc.CallOption) (*criv1.UpdateRuntimeConfigResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.client.UpdateRuntimeConfig(ctx, in)
}

func (c *client) Status(ctx context.Context, in *criv1.StatusRequest, _ ...grpc.CallOption) (*criv1.StatusResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.client.Status(ctx, in)
}

/*
func (c *client) CheckpointContainer(ctx context.Context, in *criv1.CheckpointContainerRequest, _ ...grpc.CallOption) (*criv1.CheckpointContainerResponse, error) {
	return nil, fmt.Errorf("unimplemented by CRI RuntimeService")
}

func (c *client) GetContainerEvents(ctx context.Context, in *criv1.GetContainerEventsRequest, _ ...grpc.CallOption) (criv1.RuntimeService_GetContainerEventsClient, error) {
	return nil, fmt.Errorf("unimplemented by CRI RuntimeService")
}

func (c *client) ListMetricDescriptors(ctx context.Context, in *criv1.ListMetricDescriptorsRequest, _ ...grpc.CallOption) (*criv1.ListMetricDescriptorsResponse, error) {
	return nil, fmt.Errorf("unimplemented by CRI RuntimeService")
}

func (c *client) ListPodSandboxMetrics(ctx context.Context, in *criv1.ListPodSandboxMetricsRequest, _ ...grpc.CallOption) (*criv1.ListPodSandboxMetricsResponse, error) {
	return nil, fmt.Errorf("unimplemented by CRI RuntimeService")
}
*/

func (c *client) ListImages(ctx context.Context, in *criv1.ListImagesRequest, _ ...grpc.CallOption) (*criv1.ListImagesResponse, error) {
	if err := c.checkImageService(); err != nil {
		return nil, err
	}

	return c.client.ListImages(ctx, in)
}

func (c *client) ImageStatus(ctx context.Context, in *criv1.ImageStatusRequest, _ ...grpc.CallOption) (*criv1.ImageStatusResponse, error) {
	if err := c.checkImageService(); err != nil {
		return nil, err
	}

	return c.client.ImageStatus(ctx, in)
}

func (c *client) PullImage(ctx context.Context, in *criv1.PullImageRequest, _ ...grpc.CallOption) (*criv1.PullImageResponse, error) {
	if err := c.checkImageService(); err != nil {
		return nil, err
	}

	return c.client.PullImage(ctx, in)
}

func (c *client) RemoveImage(ctx context.Context, in *criv1.RemoveImageRequest, _ ...grpc.CallOption) (*criv1.RemoveImageResponse, error) {
	if err := c.checkImageService(); err != nil {
		return nil, err
	}

	return c.client.RemoveImage(ctx, in)
}

func (c *client) ImageFsInfo(ctx context.Context, in *criv1.ImageFsInfoRequest, _ ...grpc.CallOption) (*criv1.ImageFsInfoResponse, error) {
	if err := c.checkImageService(); err != nil {
		return nil, err
	}

	return c.client.ImageFsInfo(ctx, in)
}

// Return a formatted client-specific error.
func clientError(format string, args ...interface{}) error {
	return fmt.Errorf("cri/client: "+format, args...)
}
