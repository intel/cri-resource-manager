// Copyright Intel Corporation. All Rights Reserved.
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

package v1

import (
	"context"
	"fmt"

	"google.golang.org/grpc"

	criv1 "k8s.io/cri-api/pkg/apis/runtime/v1"

	logger "github.com/intel/cri-resource-manager/pkg/log"
)

type Client interface {
	criv1.ImageServiceClient
	criv1.RuntimeServiceClient
}

type client struct {
	logger.Logger
	isc criv1.ImageServiceClient
	rsc criv1.RuntimeServiceClient
	rcc *grpc.ClientConn
	icc *grpc.ClientConn
}

// Connect v2alpha1 RuntimeService and ImageService clients.
func Connect(runtime, image *grpc.ClientConn) (Client, error) {
	c := &client{
		Logger: logger.Get("cri/client"),
		rcc:    runtime,
		icc:    image,
	}

	if c.rcc != nil {
		c.Info("probing CRI v1 RuntimeService client...")
		c.rsc = criv1.NewRuntimeServiceClient(c.rcc)
		_, err := c.rsc.Version(context.Background(), &criv1.VersionRequest{})
		if err != nil {
			return nil, err
		}
	}

	if c.icc != nil {
		c.Info("probing CRI v1 ImageService client...")
		c.isc = criv1.NewImageServiceClient(c.icc)
		_, err := c.isc.ListImages(context.Background(), &criv1.ListImagesRequest{})
		if err != nil {
			return nil, err
		}
	}

	return c, nil
}

func (c *client) checkRuntimeService() error {
	if c.rcc == nil {
		return fmt.Errorf("no CRI v1alpha2 RuntimeService client")
	}
	return nil
}

func (c *client) checkImageService() error {
	if c.icc == nil {
		return fmt.Errorf("no CRI v1alpha2 ImageService client")
	}
	return nil
}

func (c *client) Version(ctx context.Context, in *criv1.VersionRequest, opts ...grpc.CallOption) (*criv1.VersionResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.rsc.Version(ctx, in)
}

func (c *client) RunPodSandbox(ctx context.Context, in *criv1.RunPodSandboxRequest, opts ...grpc.CallOption) (*criv1.RunPodSandboxResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.rsc.RunPodSandbox(ctx, in)
}

func (c *client) StopPodSandbox(ctx context.Context, in *criv1.StopPodSandboxRequest, opts ...grpc.CallOption) (*criv1.StopPodSandboxResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.rsc.StopPodSandbox(ctx, in)
}

func (c *client) RemovePodSandbox(ctx context.Context, in *criv1.RemovePodSandboxRequest, opts ...grpc.CallOption) (*criv1.RemovePodSandboxResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.rsc.RemovePodSandbox(ctx, in)
}

func (c *client) PodSandboxStatus(ctx context.Context, in *criv1.PodSandboxStatusRequest, opts ...grpc.CallOption) (*criv1.PodSandboxStatusResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.rsc.PodSandboxStatus(ctx, in)
}

func (c *client) ListPodSandbox(ctx context.Context, in *criv1.ListPodSandboxRequest, opts ...grpc.CallOption) (*criv1.ListPodSandboxResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.rsc.ListPodSandbox(ctx, in)
}

func (c *client) CreateContainer(ctx context.Context, in *criv1.CreateContainerRequest, opts ...grpc.CallOption) (*criv1.CreateContainerResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.rsc.CreateContainer(ctx, in)
}

func (c *client) StartContainer(ctx context.Context, in *criv1.StartContainerRequest, opts ...grpc.CallOption) (*criv1.StartContainerResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.rsc.StartContainer(ctx, in)
}

func (c *client) StopContainer(ctx context.Context, in *criv1.StopContainerRequest, opts ...grpc.CallOption) (*criv1.StopContainerResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.rsc.StopContainer(ctx, in)
}

func (c *client) RemoveContainer(ctx context.Context, in *criv1.RemoveContainerRequest, opts ...grpc.CallOption) (*criv1.RemoveContainerResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.rsc.RemoveContainer(ctx, in)
}

func (c *client) ListContainers(ctx context.Context, in *criv1.ListContainersRequest, opts ...grpc.CallOption) (*criv1.ListContainersResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.rsc.ListContainers(ctx, in)
}

func (c *client) ContainerStatus(ctx context.Context, in *criv1.ContainerStatusRequest, opts ...grpc.CallOption) (*criv1.ContainerStatusResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.rsc.ContainerStatus(ctx, in)
}

func (c *client) UpdateContainerResources(ctx context.Context, in *criv1.UpdateContainerResourcesRequest, opts ...grpc.CallOption) (*criv1.UpdateContainerResourcesResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.rsc.UpdateContainerResources(ctx, in)
}

func (c *client) ReopenContainerLog(ctx context.Context, in *criv1.ReopenContainerLogRequest, opts ...grpc.CallOption) (*criv1.ReopenContainerLogResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.rsc.ReopenContainerLog(ctx, in)
}

func (c *client) ExecSync(ctx context.Context, in *criv1.ExecSyncRequest, opts ...grpc.CallOption) (*criv1.ExecSyncResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.rsc.ExecSync(ctx, in)
}

func (c *client) Exec(ctx context.Context, in *criv1.ExecRequest, opts ...grpc.CallOption) (*criv1.ExecResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.rsc.Exec(ctx, in)
}

func (c *client) Attach(ctx context.Context, in *criv1.AttachRequest, opts ...grpc.CallOption) (*criv1.AttachResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.rsc.Attach(ctx, in)
}

func (c *client) PortForward(ctx context.Context, in *criv1.PortForwardRequest, opts ...grpc.CallOption) (*criv1.PortForwardResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.rsc.PortForward(ctx, in)
}

func (c *client) ContainerStats(ctx context.Context, in *criv1.ContainerStatsRequest, opts ...grpc.CallOption) (*criv1.ContainerStatsResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.rsc.ContainerStats(ctx, in)
}

func (c *client) ListContainerStats(ctx context.Context, in *criv1.ListContainerStatsRequest, opts ...grpc.CallOption) (*criv1.ListContainerStatsResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.rsc.ListContainerStats(ctx, in)
}

func (c *client) PodSandboxStats(ctx context.Context, in *criv1.PodSandboxStatsRequest, opts ...grpc.CallOption) (*criv1.PodSandboxStatsResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.rsc.PodSandboxStats(ctx, in)
}

func (c *client) ListPodSandboxStats(ctx context.Context, in *criv1.ListPodSandboxStatsRequest, opts ...grpc.CallOption) (*criv1.ListPodSandboxStatsResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.rsc.ListPodSandboxStats(ctx, in)
}

func (c *client) UpdateRuntimeConfig(ctx context.Context, in *criv1.UpdateRuntimeConfigRequest, opts ...grpc.CallOption) (*criv1.UpdateRuntimeConfigResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.rsc.UpdateRuntimeConfig(ctx, in)
}

func (c *client) Status(ctx context.Context, in *criv1.StatusRequest, opts ...grpc.CallOption) (*criv1.StatusResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.rsc.Status(ctx, in)
}

func (c *client) CheckpointContainer(ctx context.Context, in *criv1.CheckpointContainerRequest, opts ...grpc.CallOption) (*criv1.CheckpointContainerResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.rsc.CheckpointContainer(ctx, in)
}

func (c *client) GetContainerEvents(ctx context.Context, in *criv1.GetEventsRequest, opts ...grpc.CallOption) (criv1.RuntimeService_GetContainerEventsClient, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	eventsClient, err := c.rsc.GetContainerEvents(ctx, in)
	if err != nil {
		return nil, err
	}

	return eventsClient, err
}

//
// These are being introduced but they are not defined yet for the
// CRI API version we are compiling against.
/*
func (c *client) ListMetricDescriptors(ctx context.Context, in *criv1.ListMetricDescriptorsRequest, opts ...grpc.CallOption) (*criv1.ListMetricDescriptorsResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.rsc.ListMetricDescriptors(ctx, in)
}

func (c *client) ListPodSandboxMetrics(ctx context.Context, in *criv1.ListPodSandboxMetricsRequest, opts ...grpc.CallOption) (*criv1.ListPodSandboxMetricsResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	return c.rsc.ListPodSandboxMetrics(ctx, in)
}
*/

func (c *client) ListImages(ctx context.Context, in *criv1.ListImagesRequest, opts ...grpc.CallOption) (*criv1.ListImagesResponse, error) {
	if err := c.checkImageService(); err != nil {
		return nil, err
	}

	return c.isc.ListImages(ctx, in)
}

func (c *client) ImageStatus(ctx context.Context, in *criv1.ImageStatusRequest, opts ...grpc.CallOption) (*criv1.ImageStatusResponse, error) {
	if err := c.checkImageService(); err != nil {
		return nil, err
	}

	return c.isc.ImageStatus(ctx, in)
}

func (c *client) PullImage(ctx context.Context, in *criv1.PullImageRequest, opts ...grpc.CallOption) (*criv1.PullImageResponse, error) {
	if err := c.checkImageService(); err != nil {
		return nil, err
	}

	return c.isc.PullImage(ctx, in)
}

func (c *client) RemoveImage(ctx context.Context, in *criv1.RemoveImageRequest, opts ...grpc.CallOption) (*criv1.RemoveImageResponse, error) {
	if err := c.checkImageService(); err != nil {
		return nil, err
	}

	return c.isc.RemoveImage(ctx, in)
}

func (c *client) ImageFsInfo(ctx context.Context, in *criv1.ImageFsInfoRequest, opts ...grpc.CallOption) (*criv1.ImageFsInfoResponse, error) {
	if err := c.checkImageService(); err != nil {
		return nil, err
	}

	return c.isc.ImageFsInfo(ctx, in)
}

// Return a formatted client-specific error.
func clientError(format string, args ...interface{}) error {
	return fmt.Errorf("cri/client: "+format, args...)
}
