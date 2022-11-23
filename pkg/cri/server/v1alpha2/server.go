// Copyright 2022 Intel Corporation. All Rights Reserved.
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

package server

import (
	"context"

	"google.golang.org/grpc"

	criv1 "k8s.io/cri-api/pkg/apis/runtime/v1"
	criv1alpha2 "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

type Server struct {
	server  *grpc.Server
	runtime criv1.RuntimeServiceServer
	image   criv1.ImageServiceServer
}

func NewServer(s *grpc.Server) *Server {
	return &Server{
		server: s,
	}
}

func (s *Server) RegisterImageService(image criv1.ImageServiceServer) {
	if s == nil {
		return
	}
	s.image = image
	criv1alpha2.RegisterImageServiceServer(s.server, s)
}

func (s *Server) RegisterRuntimeService(runtime criv1.RuntimeServiceServer) {
	if s == nil {
		return
	}
	s.runtime = runtime
	criv1alpha2.RegisterRuntimeServiceServer(s.server, s)
}

func (s *Server) ListImages(ctx context.Context,
	in *criv1alpha2.ListImagesRequest) (*criv1alpha2.ListImagesResponse, error) {
	var (
		v1req     = &criv1.ListImagesRequest{}
		alpharesp = &criv1alpha2.ListImagesResponse{}
	)

	if err := alphaReqToV1(in, v1req); err != nil {
		return nil, err
	}

	resp, err := s.image.ListImages(ctx, v1req)
	if err != nil {
		return nil, err
	}

	if err := v1RespToAlpha(resp, alpharesp); err != nil {
		return nil, err
	}

	return alpharesp, nil
}

func (s *Server) ImageStatus(ctx context.Context,
	in *criv1alpha2.ImageStatusRequest) (*criv1alpha2.ImageStatusResponse, error) {
	var (
		v1req     = &criv1.ImageStatusRequest{}
		alpharesp = &criv1alpha2.ImageStatusResponse{}
	)

	if err := alphaReqToV1(in, v1req); err != nil {
		return nil, err
	}

	resp, err := s.image.ImageStatus(ctx, v1req)
	if err != nil {
		return nil, err
	}

	if err := v1RespToAlpha(resp, alpharesp); err != nil {
		return nil, err
	}

	return alpharesp, nil
}

func (s *Server) PullImage(ctx context.Context,
	in *criv1alpha2.PullImageRequest) (*criv1alpha2.PullImageResponse, error) {
	var (
		v1req     = &criv1.PullImageRequest{}
		alpharesp = &criv1alpha2.PullImageResponse{}
	)

	if err := alphaReqToV1(in, v1req); err != nil {
		return nil, err
	}

	resp, err := s.image.PullImage(ctx, v1req)
	if err != nil {
		return nil, err
	}

	if err := v1RespToAlpha(resp, alpharesp); err != nil {
		return nil, err
	}

	return alpharesp, nil
}

func (s *Server) RemoveImage(ctx context.Context,
	in *criv1alpha2.RemoveImageRequest) (*criv1alpha2.RemoveImageResponse, error) {
	var (
		v1req     = &criv1.RemoveImageRequest{}
		alpharesp = &criv1alpha2.RemoveImageResponse{}
	)

	if err := alphaReqToV1(in, v1req); err != nil {
		return nil, err
	}

	resp, err := s.image.RemoveImage(ctx, v1req)
	if err != nil {
		return nil, err
	}

	if err := v1RespToAlpha(resp, alpharesp); err != nil {
		return nil, err
	}

	return alpharesp, nil
}

func (s *Server) ImageFsInfo(ctx context.Context,
	in *criv1alpha2.ImageFsInfoRequest) (*criv1alpha2.ImageFsInfoResponse, error) {
	var (
		v1req     = &criv1.ImageFsInfoRequest{}
		alpharesp = &criv1alpha2.ImageFsInfoResponse{}
	)

	if err := alphaReqToV1(in, v1req); err != nil {
		return nil, err
	}

	resp, err := s.image.ImageFsInfo(ctx, v1req)
	if err != nil {
		return nil, err
	}

	if err := v1RespToAlpha(resp, alpharesp); err != nil {
		return nil, err
	}

	return alpharesp, nil
}

func (s *Server) Version(ctx context.Context,
	in *criv1alpha2.VersionRequest) (*criv1alpha2.VersionResponse, error) {
	var (
		v1req     = &criv1.VersionRequest{}
		alpharesp = &criv1alpha2.VersionResponse{}
	)

	if err := alphaReqToV1(in, v1req); err != nil {
		return nil, err
	}

	resp, err := s.runtime.Version(ctx, v1req)
	if err != nil {
		return nil, err
	}

	if err := v1RespToAlpha(resp, alpharesp); err != nil {
		return nil, err
	}

	return alpharesp, nil
}

func (s *Server) RunPodSandbox(ctx context.Context,
	in *criv1alpha2.RunPodSandboxRequest) (*criv1alpha2.RunPodSandboxResponse, error) {
	var (
		v1req     = &criv1.RunPodSandboxRequest{}
		alpharesp = &criv1alpha2.RunPodSandboxResponse{}
	)

	if err := alphaReqToV1(in, v1req); err != nil {
		return nil, err
	}

	resp, err := s.runtime.RunPodSandbox(ctx, v1req)
	if err != nil {
		return nil, err
	}

	if err := v1RespToAlpha(resp, alpharesp); err != nil {
		return nil, err
	}

	return alpharesp, nil
}

func (s *Server) StopPodSandbox(ctx context.Context,
	in *criv1alpha2.StopPodSandboxRequest) (*criv1alpha2.StopPodSandboxResponse, error) {
	var (
		v1req     = &criv1.StopPodSandboxRequest{}
		alpharesp = &criv1alpha2.StopPodSandboxResponse{}
	)

	if err := alphaReqToV1(in, v1req); err != nil {
		return nil, err
	}

	resp, err := s.runtime.StopPodSandbox(ctx, v1req)
	if err != nil {
		return nil, err
	}

	if err := v1RespToAlpha(resp, alpharesp); err != nil {
		return nil, err
	}

	return alpharesp, nil
}

func (s *Server) RemovePodSandbox(ctx context.Context,
	in *criv1alpha2.RemovePodSandboxRequest) (*criv1alpha2.RemovePodSandboxResponse, error) {
	var (
		v1req     = &criv1.RemovePodSandboxRequest{}
		alpharesp = &criv1alpha2.RemovePodSandboxResponse{}
	)

	if err := alphaReqToV1(in, v1req); err != nil {
		return nil, err
	}

	resp, err := s.runtime.RemovePodSandbox(ctx, v1req)
	if err != nil {
		return nil, err
	}

	if err := v1RespToAlpha(resp, alpharesp); err != nil {
		return nil, err
	}

	return alpharesp, nil
}

func (s *Server) PodSandboxStatus(ctx context.Context,
	in *criv1alpha2.PodSandboxStatusRequest) (*criv1alpha2.PodSandboxStatusResponse, error) {
	var (
		v1req     = &criv1.PodSandboxStatusRequest{}
		alpharesp = &criv1alpha2.PodSandboxStatusResponse{}
	)

	if err := alphaReqToV1(in, v1req); err != nil {
		return nil, err
	}

	resp, err := s.runtime.PodSandboxStatus(ctx, v1req)
	if err != nil {
		return nil, err
	}

	if err := v1RespToAlpha(resp, alpharesp); err != nil {
		return nil, err
	}

	return alpharesp, nil
}

func (s *Server) ListPodSandbox(ctx context.Context,
	in *criv1alpha2.ListPodSandboxRequest) (*criv1alpha2.ListPodSandboxResponse, error) {
	var (
		v1req     = &criv1.ListPodSandboxRequest{}
		alpharesp = &criv1alpha2.ListPodSandboxResponse{}
	)

	if err := alphaReqToV1(in, v1req); err != nil {
		return nil, err
	}

	resp, err := s.runtime.ListPodSandbox(ctx, v1req)
	if err != nil {
		return nil, err
	}

	if err := v1RespToAlpha(resp, alpharesp); err != nil {
		return nil, err
	}

	return alpharesp, nil
}

func (s *Server) CreateContainer(ctx context.Context,
	in *criv1alpha2.CreateContainerRequest) (*criv1alpha2.CreateContainerResponse, error) {
	var (
		v1req     = &criv1.CreateContainerRequest{}
		alpharesp = &criv1alpha2.CreateContainerResponse{}
	)

	if err := alphaReqToV1(in, v1req); err != nil {
		return nil, err
	}

	resp, err := s.runtime.CreateContainer(ctx, v1req)
	if err != nil {
		return nil, err
	}

	if err := v1RespToAlpha(resp, alpharesp); err != nil {
		return nil, err
	}

	return alpharesp, nil
}

func (s *Server) StartContainer(ctx context.Context,
	in *criv1alpha2.StartContainerRequest) (*criv1alpha2.StartContainerResponse, error) {
	var (
		v1req     = &criv1.StartContainerRequest{}
		alpharesp = &criv1alpha2.StartContainerResponse{}
	)

	if err := alphaReqToV1(in, v1req); err != nil {
		return nil, err
	}

	resp, err := s.runtime.StartContainer(ctx, v1req)
	if err != nil {
		return nil, err
	}

	if err := v1RespToAlpha(resp, alpharesp); err != nil {
		return nil, err
	}

	return alpharesp, nil
}

func (s *Server) StopContainer(ctx context.Context,
	in *criv1alpha2.StopContainerRequest) (*criv1alpha2.StopContainerResponse, error) {
	var (
		v1req     = &criv1.StopContainerRequest{}
		alpharesp = &criv1alpha2.StopContainerResponse{}
	)

	if err := alphaReqToV1(in, v1req); err != nil {
		return nil, err
	}

	resp, err := s.runtime.StopContainer(ctx, v1req)
	if err != nil {
		return nil, err
	}

	if err := v1RespToAlpha(resp, alpharesp); err != nil {
		return nil, err
	}

	return alpharesp, nil
}

func (s *Server) RemoveContainer(ctx context.Context,
	in *criv1alpha2.RemoveContainerRequest) (*criv1alpha2.RemoveContainerResponse, error) {
	var (
		v1req     = &criv1.RemoveContainerRequest{}
		alpharesp = &criv1alpha2.RemoveContainerResponse{}
	)

	if err := alphaReqToV1(in, v1req); err != nil {
		return nil, err
	}

	resp, err := s.runtime.RemoveContainer(ctx, v1req)
	if err != nil {
		return nil, err
	}

	if err := v1RespToAlpha(resp, alpharesp); err != nil {
		return nil, err
	}

	return alpharesp, nil
}

func (s *Server) ListContainers(ctx context.Context,
	in *criv1alpha2.ListContainersRequest) (*criv1alpha2.ListContainersResponse, error) {
	var (
		v1req     = &criv1.ListContainersRequest{}
		alpharesp = &criv1alpha2.ListContainersResponse{}
	)

	if err := alphaReqToV1(in, v1req); err != nil {
		return nil, err
	}

	resp, err := s.runtime.ListContainers(ctx, v1req)
	if err != nil {
		return nil, err
	}

	if err := v1RespToAlpha(resp, alpharesp); err != nil {
		return nil, err
	}

	return alpharesp, nil
}

func (s *Server) ContainerStatus(ctx context.Context,
	in *criv1alpha2.ContainerStatusRequest) (*criv1alpha2.ContainerStatusResponse, error) {
	var (
		v1req     = &criv1.ContainerStatusRequest{}
		alpharesp = &criv1alpha2.ContainerStatusResponse{}
	)

	if err := alphaReqToV1(in, v1req); err != nil {
		return nil, err
	}

	resp, err := s.runtime.ContainerStatus(ctx, v1req)
	if err != nil {
		return nil, err
	}

	if err := v1RespToAlpha(resp, alpharesp); err != nil {
		return nil, err
	}

	return alpharesp, nil
}

func (s *Server) UpdateContainerResources(ctx context.Context,
	in *criv1alpha2.UpdateContainerResourcesRequest) (*criv1alpha2.UpdateContainerResourcesResponse, error) {
	var (
		v1req     = &criv1.UpdateContainerResourcesRequest{}
		alpharesp = &criv1alpha2.UpdateContainerResourcesResponse{}
	)

	if err := alphaReqToV1(in, v1req); err != nil {
		return nil, err
	}

	resp, err := s.runtime.UpdateContainerResources(ctx, v1req)
	if err != nil {
		return nil, err
	}

	if err := v1RespToAlpha(resp, alpharesp); err != nil {
		return nil, err
	}

	return alpharesp, nil
}

func (s *Server) ReopenContainerLog(ctx context.Context,
	in *criv1alpha2.ReopenContainerLogRequest) (*criv1alpha2.ReopenContainerLogResponse, error) {
	var (
		v1req     = &criv1.ReopenContainerLogRequest{}
		alpharesp = &criv1alpha2.ReopenContainerLogResponse{}
	)

	if err := alphaReqToV1(in, v1req); err != nil {
		return nil, err
	}

	resp, err := s.runtime.ReopenContainerLog(ctx, v1req)
	if err != nil {
		return nil, err
	}

	if err := v1RespToAlpha(resp, alpharesp); err != nil {
		return nil, err
	}

	return alpharesp, nil
}

func (s *Server) ExecSync(ctx context.Context,
	in *criv1alpha2.ExecSyncRequest) (*criv1alpha2.ExecSyncResponse, error) {
	var (
		v1req     = &criv1.ExecSyncRequest{}
		alpharesp = &criv1alpha2.ExecSyncResponse{}
	)

	if err := alphaReqToV1(in, v1req); err != nil {
		return nil, err
	}

	resp, err := s.runtime.ExecSync(ctx, v1req)
	if err != nil {
		return nil, err
	}

	if err := v1RespToAlpha(resp, alpharesp); err != nil {
		return nil, err
	}

	return alpharesp, nil
}

func (s *Server) Exec(ctx context.Context,
	in *criv1alpha2.ExecRequest) (*criv1alpha2.ExecResponse, error) {
	var (
		v1req     = &criv1.ExecRequest{}
		alpharesp = &criv1alpha2.ExecResponse{}
	)

	if err := alphaReqToV1(in, v1req); err != nil {
		return nil, err
	}

	resp, err := s.runtime.Exec(ctx, v1req)
	if err != nil {
		return nil, err
	}

	if err := v1RespToAlpha(resp, alpharesp); err != nil {
		return nil, err
	}

	return alpharesp, nil
}

func (s *Server) Attach(ctx context.Context,
	in *criv1alpha2.AttachRequest) (*criv1alpha2.AttachResponse, error) {
	var (
		v1req     = &criv1.AttachRequest{}
		alpharesp = &criv1alpha2.AttachResponse{}
	)

	if err := alphaReqToV1(in, v1req); err != nil {
		return nil, err
	}

	resp, err := s.runtime.Attach(ctx, v1req)
	if err != nil {
		return nil, err
	}

	if err := v1RespToAlpha(resp, alpharesp); err != nil {
		return nil, err
	}

	return alpharesp, nil
}

func (s *Server) PortForward(ctx context.Context,
	in *criv1alpha2.PortForwardRequest) (*criv1alpha2.PortForwardResponse, error) {
	var (
		v1req     = &criv1.PortForwardRequest{}
		alpharesp = &criv1alpha2.PortForwardResponse{}
	)

	if err := alphaReqToV1(in, v1req); err != nil {
		return nil, err
	}

	resp, err := s.runtime.PortForward(ctx, v1req)
	if err != nil {
		return nil, err
	}

	if err := v1RespToAlpha(resp, alpharesp); err != nil {
		return nil, err
	}

	return alpharesp, nil
}

func (s *Server) ContainerStats(ctx context.Context,
	in *criv1alpha2.ContainerStatsRequest) (*criv1alpha2.ContainerStatsResponse, error) {
	var (
		v1req     = &criv1.ContainerStatsRequest{}
		alpharesp = &criv1alpha2.ContainerStatsResponse{}
	)

	if err := alphaReqToV1(in, v1req); err != nil {
		return nil, err
	}

	resp, err := s.runtime.ContainerStats(ctx, v1req)
	if err != nil {
		return nil, err
	}

	if err := v1RespToAlpha(resp, alpharesp); err != nil {
		return nil, err
	}

	return alpharesp, nil
}

func (s *Server) ListContainerStats(ctx context.Context,
	in *criv1alpha2.ListContainerStatsRequest) (*criv1alpha2.ListContainerStatsResponse, error) {
	var (
		v1req     = &criv1.ListContainerStatsRequest{}
		alpharesp = &criv1alpha2.ListContainerStatsResponse{}
	)

	if err := alphaReqToV1(in, v1req); err != nil {
		return nil, err
	}

	resp, err := s.runtime.ListContainerStats(ctx, v1req)
	if err != nil {
		return nil, err
	}

	if err := v1RespToAlpha(resp, alpharesp); err != nil {
		return nil, err
	}

	return alpharesp, nil
}

func (s *Server) PodSandboxStats(ctx context.Context, in *criv1alpha2.PodSandboxStatsRequest) (*criv1alpha2.PodSandboxStatsResponse, error) {
	var (
		v1req     = &criv1.PodSandboxStatsRequest{}
		alpharesp = &criv1alpha2.PodSandboxStatsResponse{}
	)

	if err := alphaReqToV1(in, v1req); err != nil {
		return nil, err
	}

	resp, err := s.runtime.PodSandboxStats(ctx, v1req)
	if err != nil {
		return nil, err
	}

	if err := v1RespToAlpha(resp, alpharesp); err != nil {
		return nil, err
	}

	return alpharesp, nil
}

func (s *Server) ListPodSandboxStats(ctx context.Context, in *criv1alpha2.ListPodSandboxStatsRequest) (*criv1alpha2.ListPodSandboxStatsResponse, error) {
	var (
		v1req     = &criv1.ListPodSandboxStatsRequest{}
		alpharesp = &criv1alpha2.ListPodSandboxStatsResponse{}
	)

	if err := alphaReqToV1(in, v1req); err != nil {
		return nil, err
	}

	resp, err := s.runtime.ListPodSandboxStats(ctx, v1req)
	if err != nil {
		return nil, err
	}

	if err := v1RespToAlpha(resp, alpharesp); err != nil {
		return nil, err
	}

	return alpharesp, nil
}

func (s *Server) UpdateRuntimeConfig(ctx context.Context,
	in *criv1alpha2.UpdateRuntimeConfigRequest) (*criv1alpha2.UpdateRuntimeConfigResponse, error) {
	var (
		v1req     = &criv1.UpdateRuntimeConfigRequest{}
		alpharesp = &criv1alpha2.UpdateRuntimeConfigResponse{}
	)

	if err := alphaReqToV1(in, v1req); err != nil {
		return nil, err
	}

	resp, err := s.runtime.UpdateRuntimeConfig(ctx, v1req)
	if err != nil {
		return nil, err
	}

	if err := v1RespToAlpha(resp, alpharesp); err != nil {
		return nil, err
	}

	return alpharesp, nil
}

func (s *Server) Status(ctx context.Context,
	in *criv1alpha2.StatusRequest) (*criv1alpha2.StatusResponse, error) {
	var (
		v1req     = &criv1.StatusRequest{}
		alpharesp = &criv1alpha2.StatusResponse{}
	)

	if err := alphaReqToV1(in, v1req); err != nil {
		return nil, err
	}

	resp, err := s.runtime.Status(ctx, v1req)
	if err != nil {
		return nil, err
	}

	if err := v1RespToAlpha(resp, alpharesp); err != nil {
		return nil, err
	}

	return alpharesp, nil
}

func alphaReqToV1(
	alpha interface{ Marshal() ([]byte, error) },
	v1req interface{ Unmarshal(_ []byte) error },
) error {
	p, err := alpha.Marshal()
	if err != nil {
		return err
	}

	if err = v1req.Unmarshal(p); err != nil {
		return err
	}
	return nil
}

func v1RespToAlpha(
	v1res interface{ Marshal() ([]byte, error) },
	alpha interface{ Unmarshal(_ []byte) error },
) error {
	p, err := v1res.Marshal()
	if err != nil {
		return err
	}

	if err = alpha.Unmarshal(p); err != nil {
		return err
	}
	return nil
}
