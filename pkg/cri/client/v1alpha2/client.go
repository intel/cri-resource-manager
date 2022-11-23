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

package v1alpha2

import (
	"context"
	"fmt"

	"google.golang.org/grpc"

	criv1 "k8s.io/cri-api/pkg/apis/runtime/v1"
	criv1alpha2 "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"

	logger "github.com/intel/cri-resource-manager/pkg/log"
)

type Client interface {
	criv1.ImageServiceClient
	criv1.RuntimeServiceClient
}

type client struct {
	logger.Logger
	isc criv1alpha2.ImageServiceClient
	rsc criv1alpha2.RuntimeServiceClient
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
		c.Info("probing CRI v1alpha2 RuntimeService client...")
		c.rsc = criv1alpha2.NewRuntimeServiceClient(c.rcc)
		_, err := c.rsc.Version(context.Background(), &criv1alpha2.VersionRequest{})
		if err != nil {
			return nil, err
		}
	}

	if c.icc != nil {
		c.Info("probing CRI v1alpha2 ImageService client...")
		c.isc = criv1alpha2.NewImageServiceClient(c.icc)
		_, err := c.isc.ListImages(context.Background(), &criv1alpha2.ListImagesRequest{})
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

	var (
		req    = &criv1alpha2.VersionRequest{}
		v1resp = &criv1.VersionResponse{}
	)

	if err := v1ReqToAlpha(in, req); err != nil {
		return nil, err
	}

	resp, err := c.rsc.Version(ctx, req)
	if err != nil {
		return nil, err
	}

	if err := alphaRespToV1(resp, v1resp); err != nil {
		return nil, err
	}

	return v1resp, nil
}

func (c *client) RunPodSandbox(ctx context.Context, in *criv1.RunPodSandboxRequest, opts ...grpc.CallOption) (*criv1.RunPodSandboxResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	var (
		req    = &criv1alpha2.RunPodSandboxRequest{}
		v1resp = &criv1.RunPodSandboxResponse{}
	)

	if err := v1ReqToAlpha(in, req); err != nil {
		return nil, err
	}

	resp, err := c.rsc.RunPodSandbox(ctx, req)
	if err != nil {
		return nil, err
	}

	if err := alphaRespToV1(resp, v1resp); err != nil {
		return nil, err
	}

	return v1resp, nil
}

func (c *client) StopPodSandbox(ctx context.Context, in *criv1.StopPodSandboxRequest, opts ...grpc.CallOption) (*criv1.StopPodSandboxResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	var (
		req    = &criv1alpha2.StopPodSandboxRequest{}
		v1resp = &criv1.StopPodSandboxResponse{}
	)

	if err := v1ReqToAlpha(in, req); err != nil {
		return nil, err
	}

	resp, err := c.rsc.StopPodSandbox(ctx, req)
	if err != nil {
		return nil, err
	}

	if err := alphaRespToV1(resp, v1resp); err != nil {
		return nil, err
	}

	return v1resp, nil
}

func (c *client) RemovePodSandbox(ctx context.Context, in *criv1.RemovePodSandboxRequest, opts ...grpc.CallOption) (*criv1.RemovePodSandboxResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	var (
		req    = &criv1alpha2.RemovePodSandboxRequest{}
		v1resp = &criv1.RemovePodSandboxResponse{}
	)

	if err := v1ReqToAlpha(in, req); err != nil {
		return nil, err
	}

	resp, err := c.rsc.RemovePodSandbox(ctx, req)
	if err != nil {
		return nil, err
	}

	if err := alphaRespToV1(resp, v1resp); err != nil {
		return nil, err
	}

	return v1resp, nil
}

func (c *client) PodSandboxStatus(ctx context.Context, in *criv1.PodSandboxStatusRequest, opts ...grpc.CallOption) (*criv1.PodSandboxStatusResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	var (
		req    = &criv1alpha2.PodSandboxStatusRequest{}
		v1resp = &criv1.PodSandboxStatusResponse{}
	)

	if err := v1ReqToAlpha(in, req); err != nil {
		return nil, err
	}

	resp, err := c.rsc.PodSandboxStatus(ctx, req)
	if err != nil {
		return nil, err
	}

	if err := alphaRespToV1(resp, v1resp); err != nil {
		return nil, err
	}

	return v1resp, nil
}

func (c *client) ListPodSandbox(ctx context.Context, in *criv1.ListPodSandboxRequest, opts ...grpc.CallOption) (*criv1.ListPodSandboxResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	var (
		req    = &criv1alpha2.ListPodSandboxRequest{}
		v1resp = &criv1.ListPodSandboxResponse{}
	)

	if err := v1ReqToAlpha(in, req); err != nil {
		return nil, err
	}

	resp, err := c.rsc.ListPodSandbox(ctx, req)
	if err != nil {
		return nil, err
	}

	if err := alphaRespToV1(resp, v1resp); err != nil {
		return nil, err
	}

	return v1resp, nil
}

func (c *client) CreateContainer(ctx context.Context, in *criv1.CreateContainerRequest, opts ...grpc.CallOption) (*criv1.CreateContainerResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	var (
		req    = &criv1alpha2.CreateContainerRequest{}
		v1resp = &criv1.CreateContainerResponse{}
	)

	if err := v1ReqToAlpha(in, req); err != nil {
		return nil, err
	}

	resp, err := c.rsc.CreateContainer(ctx, req)
	if err != nil {
		return nil, err
	}

	if err := alphaRespToV1(resp, v1resp); err != nil {
		return nil, err
	}

	return v1resp, nil
}

func (c *client) StartContainer(ctx context.Context, in *criv1.StartContainerRequest, opts ...grpc.CallOption) (*criv1.StartContainerResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	var (
		req    = &criv1alpha2.StartContainerRequest{}
		v1resp = &criv1.StartContainerResponse{}
	)

	if err := v1ReqToAlpha(in, req); err != nil {
		return nil, err
	}

	resp, err := c.rsc.StartContainer(ctx, req)
	if err != nil {
		return nil, err
	}

	if err := alphaRespToV1(resp, v1resp); err != nil {
		return nil, err
	}

	return v1resp, nil
}

func (c *client) StopContainer(ctx context.Context, in *criv1.StopContainerRequest, opts ...grpc.CallOption) (*criv1.StopContainerResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	var (
		req    = &criv1alpha2.StopContainerRequest{}
		v1resp = &criv1.StopContainerResponse{}
	)

	if err := v1ReqToAlpha(in, req); err != nil {
		return nil, err
	}

	resp, err := c.rsc.StopContainer(ctx, req)
	if err != nil {
		return nil, err
	}

	if err := alphaRespToV1(resp, v1resp); err != nil {
		return nil, err
	}

	return v1resp, nil
}

func (c *client) RemoveContainer(ctx context.Context, in *criv1.RemoveContainerRequest, opts ...grpc.CallOption) (*criv1.RemoveContainerResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	var (
		req    = &criv1alpha2.RemoveContainerRequest{}
		v1resp = &criv1.RemoveContainerResponse{}
	)

	if err := v1ReqToAlpha(in, req); err != nil {
		return nil, err
	}

	resp, err := c.rsc.RemoveContainer(ctx, req)
	if err != nil {
		return nil, err
	}

	if err := alphaRespToV1(resp, v1resp); err != nil {
		return nil, err
	}

	return v1resp, nil
}

func (c *client) ListContainers(ctx context.Context, in *criv1.ListContainersRequest, opts ...grpc.CallOption) (*criv1.ListContainersResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	var (
		req    = &criv1alpha2.ListContainersRequest{}
		v1resp = &criv1.ListContainersResponse{}
	)

	if err := v1ReqToAlpha(in, req); err != nil {
		return nil, err
	}

	resp, err := c.rsc.ListContainers(ctx, req)
	if err != nil {
		return nil, err
	}

	if err := alphaRespToV1(resp, v1resp); err != nil {
		return nil, err
	}

	return v1resp, nil
}

func (c *client) ContainerStatus(ctx context.Context, in *criv1.ContainerStatusRequest, opts ...grpc.CallOption) (*criv1.ContainerStatusResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	var (
		req    = &criv1alpha2.ContainerStatusRequest{}
		v1resp = &criv1.ContainerStatusResponse{}
	)

	if err := v1ReqToAlpha(in, req); err != nil {
		return nil, err
	}

	resp, err := c.rsc.ContainerStatus(ctx, req)
	if err != nil {
		return nil, err
	}

	if err := alphaRespToV1(resp, v1resp); err != nil {
		return nil, err
	}

	return v1resp, nil
}

func (c *client) UpdateContainerResources(ctx context.Context, in *criv1.UpdateContainerResourcesRequest, opts ...grpc.CallOption) (*criv1.UpdateContainerResourcesResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	var (
		req    = &criv1alpha2.UpdateContainerResourcesRequest{}
		v1resp = &criv1.UpdateContainerResourcesResponse{}
	)

	if err := v1ReqToAlpha(in, req); err != nil {
		return nil, err
	}

	resp, err := c.rsc.UpdateContainerResources(ctx, req)
	if err != nil {
		return nil, err
	}

	if err := alphaRespToV1(resp, v1resp); err != nil {
		return nil, err
	}

	return v1resp, nil
}

func (c *client) ReopenContainerLog(ctx context.Context, in *criv1.ReopenContainerLogRequest, opts ...grpc.CallOption) (*criv1.ReopenContainerLogResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	var (
		req    = &criv1alpha2.ReopenContainerLogRequest{}
		v1resp = &criv1.ReopenContainerLogResponse{}
	)

	if err := v1ReqToAlpha(in, req); err != nil {
		return nil, err
	}

	resp, err := c.rsc.ReopenContainerLog(ctx, req)
	if err != nil {
		return nil, err
	}

	if err := alphaRespToV1(resp, v1resp); err != nil {
		return nil, err
	}

	return v1resp, nil
}

func (c *client) ExecSync(ctx context.Context, in *criv1.ExecSyncRequest, opts ...grpc.CallOption) (*criv1.ExecSyncResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	var (
		req    = &criv1alpha2.ExecSyncRequest{}
		v1resp = &criv1.ExecSyncResponse{}
	)

	if err := v1ReqToAlpha(in, req); err != nil {
		return nil, err
	}

	resp, err := c.rsc.ExecSync(ctx, req)
	if err != nil {
		return nil, err
	}

	if err := alphaRespToV1(resp, v1resp); err != nil {
		return nil, err
	}

	return v1resp, nil
}

func (c *client) Exec(ctx context.Context, in *criv1.ExecRequest, opts ...grpc.CallOption) (*criv1.ExecResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	var (
		req    = &criv1alpha2.ExecRequest{}
		v1resp = &criv1.ExecResponse{}
	)

	if err := v1ReqToAlpha(in, req); err != nil {
		return nil, err
	}

	resp, err := c.rsc.Exec(ctx, req)
	if err != nil {
		return nil, err
	}

	if err := alphaRespToV1(resp, v1resp); err != nil {
		return nil, err
	}

	return v1resp, nil
}

func (c *client) Attach(ctx context.Context, in *criv1.AttachRequest, opts ...grpc.CallOption) (*criv1.AttachResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	var (
		req    = &criv1alpha2.AttachRequest{}
		v1resp = &criv1.AttachResponse{}
	)

	if err := v1ReqToAlpha(in, req); err != nil {
		return nil, err
	}

	resp, err := c.rsc.Attach(ctx, req)
	if err != nil {
		return nil, err
	}

	if err := alphaRespToV1(resp, v1resp); err != nil {
		return nil, err
	}

	return v1resp, nil
}

func (c *client) PortForward(ctx context.Context, in *criv1.PortForwardRequest, opts ...grpc.CallOption) (*criv1.PortForwardResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	var (
		req    = &criv1alpha2.PortForwardRequest{}
		v1resp = &criv1.PortForwardResponse{}
	)

	if err := v1ReqToAlpha(in, req); err != nil {
		return nil, err
	}

	resp, err := c.rsc.PortForward(ctx, req)
	if err != nil {
		return nil, err
	}

	if err := alphaRespToV1(resp, v1resp); err != nil {
		return nil, err
	}

	return v1resp, nil
}

func (c *client) ContainerStats(ctx context.Context, in *criv1.ContainerStatsRequest, opts ...grpc.CallOption) (*criv1.ContainerStatsResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	var (
		req    = &criv1alpha2.ContainerStatsRequest{}
		v1resp = &criv1.ContainerStatsResponse{}
	)

	if err := v1ReqToAlpha(in, req); err != nil {
		return nil, err
	}

	resp, err := c.rsc.ContainerStats(ctx, req)
	if err != nil {
		return nil, err
	}

	if err := alphaRespToV1(resp, v1resp); err != nil {
		return nil, err
	}

	return v1resp, nil
}

func (c *client) ListContainerStats(ctx context.Context, in *criv1.ListContainerStatsRequest, opts ...grpc.CallOption) (*criv1.ListContainerStatsResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	var (
		req    = &criv1alpha2.ListContainerStatsRequest{}
		v1resp = &criv1.ListContainerStatsResponse{}
	)

	if err := v1ReqToAlpha(in, req); err != nil {
		return nil, err
	}

	resp, err := c.rsc.ListContainerStats(ctx, req)
	if err != nil {
		return nil, err
	}

	if err := alphaRespToV1(resp, v1resp); err != nil {
		return nil, err
	}

	return v1resp, nil
}

func (c *client) PodSandboxStats(ctx context.Context, in *criv1.PodSandboxStatsRequest, opts ...grpc.CallOption) (*criv1.PodSandboxStatsResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	var (
		req    = &criv1alpha2.PodSandboxStatsRequest{}
		v1resp = &criv1.PodSandboxStatsResponse{}
	)

	if err := v1ReqToAlpha(in, req); err != nil {
		return nil, err
	}

	resp, err := c.rsc.PodSandboxStats(ctx, req)
	if err != nil {
		return nil, err
	}

	if err := alphaRespToV1(resp, v1resp); err != nil {
		return nil, err
	}

	return v1resp, nil
}

func (c *client) ListPodSandboxStats(ctx context.Context, in *criv1.ListPodSandboxStatsRequest, opts ...grpc.CallOption) (*criv1.ListPodSandboxStatsResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	var (
		req    = &criv1alpha2.ListPodSandboxStatsRequest{}
		v1resp = &criv1.ListPodSandboxStatsResponse{}
	)

	if err := v1ReqToAlpha(in, req); err != nil {
		return nil, err
	}

	resp, err := c.rsc.ListPodSandboxStats(ctx, req)
	if err != nil {
		return nil, err
	}

	if err := alphaRespToV1(resp, v1resp); err != nil {
		return nil, err
	}

	return v1resp, nil
}

func (c *client) UpdateRuntimeConfig(ctx context.Context, in *criv1.UpdateRuntimeConfigRequest, opts ...grpc.CallOption) (*criv1.UpdateRuntimeConfigResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	var (
		req    = &criv1alpha2.UpdateRuntimeConfigRequest{}
		v1resp = &criv1.UpdateRuntimeConfigResponse{}
	)

	if err := v1ReqToAlpha(in, req); err != nil {
		return nil, err
	}

	resp, err := c.rsc.UpdateRuntimeConfig(ctx, req)
	if err != nil {
		return nil, err
	}

	if err := alphaRespToV1(resp, v1resp); err != nil {
		return nil, err
	}

	return v1resp, nil
}

func (c *client) Status(ctx context.Context, in *criv1.StatusRequest, opts ...grpc.CallOption) (*criv1.StatusResponse, error) {
	if err := c.checkRuntimeService(); err != nil {
		return nil, err
	}

	var (
		req    = &criv1alpha2.StatusRequest{}
		v1resp = &criv1.StatusResponse{}
	)

	if err := v1ReqToAlpha(in, req); err != nil {
		return nil, err
	}

	resp, err := c.rsc.Status(ctx, req)
	if err != nil {
		return nil, err
	}

	if err := alphaRespToV1(resp, v1resp); err != nil {
		return nil, err
	}

	return v1resp, nil
}

//
// These are being introduced but they are not defined yet for the
// CRI API version we are compiling against.
/*
func (c *client) CheckpointContainer(ctx context.Context, in *criv1.CheckpointContainerRequest, opts ...grpc.CallOption) (*criv1.CheckpointContainerResponse, error) {
	return nil, fmt.Errorf("unimplemented by CRI v1alpha2 RuntimeService")
}

func (c *client) GetContainerEvents(ctx context.Context, in *criv1.GetContainerEventsRequest, opts ...grpc.CallOption) (criv1.RuntimeService_GetContainerEventsClient, error) {
	return nil, fmt.Errorf("unimplemented by CRI v1alpha2 RuntimeService")
}

func (c *client) ListMetricDescriptors(ctx context.Context, in *criv1.ListMetricDescriptorsRequest, opts ...grpc.CallOption) (*criv1.ListMetricDescriptorsResponse, error) {
	return nil, fmt.Errorf("unimplemented by CRI v1alpha2 RuntimeService")
}

func (c *client) ListPodSandboxMetrics(ctx context.Context, in *criv1.ListPodSandboxMetricsRequest, opts ...grpc.CallOption) (*criv1.ListPodSandboxMetricsResponse, error) {
	return nil, fmt.Errorf("unimplemented by CRI v1alpha2 RuntimeService")
}
*/

func (c *client) ListImages(ctx context.Context, in *criv1.ListImagesRequest, opts ...grpc.CallOption) (*criv1.ListImagesResponse, error) {
	if err := c.checkImageService(); err != nil {
		return nil, err
	}

	var (
		req    = &criv1alpha2.ListImagesRequest{}
		v1resp = &criv1.ListImagesResponse{}
	)

	if err := v1ReqToAlpha(in, req); err != nil {
		return nil, err
	}

	resp, err := c.isc.ListImages(ctx, req)
	if err != nil {
		return nil, err
	}

	if err := alphaRespToV1(resp, v1resp); err != nil {
		return nil, err
	}

	return v1resp, nil
}

func (c *client) ImageStatus(ctx context.Context, in *criv1.ImageStatusRequest, opts ...grpc.CallOption) (*criv1.ImageStatusResponse, error) {
	if err := c.checkImageService(); err != nil {
		return nil, err
	}

	var (
		req    = &criv1alpha2.ImageStatusRequest{}
		v1resp = &criv1.ImageStatusResponse{}
	)

	if err := v1ReqToAlpha(in, req); err != nil {
		return nil, err
	}

	resp, err := c.isc.ImageStatus(ctx, req)
	if err != nil {
		return nil, err
	}

	if err := alphaRespToV1(resp, v1resp); err != nil {
		return nil, err
	}

	return v1resp, nil
}

func (c *client) PullImage(ctx context.Context, in *criv1.PullImageRequest, opts ...grpc.CallOption) (*criv1.PullImageResponse, error) {
	if err := c.checkImageService(); err != nil {
		return nil, err
	}

	var (
		req    = &criv1alpha2.PullImageRequest{}
		v1resp = &criv1.PullImageResponse{}
	)

	if err := v1ReqToAlpha(in, req); err != nil {
		return nil, err
	}

	resp, err := c.isc.PullImage(ctx, req)
	if err != nil {
		return nil, err
	}

	if err := alphaRespToV1(resp, v1resp); err != nil {
		return nil, err
	}

	return v1resp, nil
}

func (c *client) RemoveImage(ctx context.Context, in *criv1.RemoveImageRequest, opts ...grpc.CallOption) (*criv1.RemoveImageResponse, error) {
	if err := c.checkImageService(); err != nil {
		return nil, err
	}

	var (
		req    = &criv1alpha2.RemoveImageRequest{}
		v1resp = &criv1.RemoveImageResponse{}
	)

	if err := v1ReqToAlpha(in, req); err != nil {
		return nil, err
	}

	resp, err := c.isc.RemoveImage(ctx, req)
	if err != nil {
		return nil, err
	}

	if err := alphaRespToV1(resp, v1resp); err != nil {
		return nil, err
	}

	return v1resp, nil
}

func (c *client) ImageFsInfo(ctx context.Context, in *criv1.ImageFsInfoRequest, opts ...grpc.CallOption) (*criv1.ImageFsInfoResponse, error) {
	if err := c.checkImageService(); err != nil {
		return nil, err
	}

	var (
		req    = &criv1alpha2.ImageFsInfoRequest{}
		v1resp = &criv1.ImageFsInfoResponse{}
	)

	if err := v1ReqToAlpha(in, req); err != nil {
		return nil, err
	}

	resp, err := c.isc.ImageFsInfo(ctx, req)
	if err != nil {
		return nil, err
	}

	if err := alphaRespToV1(resp, v1resp); err != nil {
		return nil, err
	}

	return v1resp, nil

}

func v1ReqToAlpha(
	v1req interface{ Marshal() ([]byte, error) },
	alpha interface{ Unmarshal(_ []byte) error },
) error {
	p, err := v1req.Marshal()
	if err != nil {
		return err
	}

	if err = alpha.Unmarshal(p); err != nil {
		return err
	}
	return nil
}

func alphaRespToV1(
	alpha interface{ Marshal() ([]byte, error) },
	v1res interface{ Unmarshal(_ []byte) error },
) error {
	p, err := alpha.Marshal()
	if err != nil {
		return err
	}

	if err = v1res.Unmarshal(p); err != nil {
		return err
	}
	return nil
}

// Return a formatted client-specific error.
func clientError(format string, args ...interface{}) error {
	return fmt.Errorf("cri/client: "+format, args...)
}
