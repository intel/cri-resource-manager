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

package server

import (
	"context"

	"go.opencensus.io/trace"
	"google.golang.org/grpc"
	grpccodes "google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"

	criv1 "k8s.io/cri-api/pkg/apis/runtime/v1"
)

const (
	apiVersion = "v1"

	imageService = "ImageService"
	listImages   = "ListImages"
	imageStatus  = "ImageStatus"
	pullImage    = "PullImage"
	removeImage  = "RemoveImage"
	imageFsInfo  = "ImageFsInfo"

	runtimeService           = "RuntimeService"
	version                  = "Version"
	runPodSandbox            = "RunPodSandbox"
	stopPodSandbox           = "StopPodSandbox"
	removePodSandbox         = "RemovePodSandbox"
	podSandboxStatus         = "PodSandboxStatus"
	listPodSandbox           = "ListPodSandbox"
	createContainer          = "CreateContainer"
	startContainer           = "StartContainer"
	stopContainer            = "StopContainer"
	removeContainer          = "RemoveContainer"
	listContainers           = "ListContainers"
	containerStatus          = "ContainerStatus"
	updateContainerResources = "UpdateContainerResources"
	reopenContainerLog       = "ReopenContainerLog"
	execSync                 = "ExecSync"
	exec                     = "Exec"
	attach                   = "Attach"
	portForward              = "PortForward"
	containerStats           = "ContainerStats"
	listContainerStats       = "ListContainerStats"
	podSandboxStats          = "PodSandboxStats"
	listPodSandboxStats      = "ListPodSandboxStats"
	updateRuntimeConfig      = "UpdateRuntimeConfig"
	status                   = "Status"
	checkpointContainer      = "CheckpointContainer"
)

func fqmn(service, method string) string {
	return "/runtime." + apiVersion + "." + service + "/" + method
}

func (s *server) interceptRequest(ctx context.Context, service, method string,
	req interface{}, handler grpc.UnaryHandler) (interface{}, error) {

	if span := trace.FromContext(ctx); span != nil {
		span.AddAttributes(
			trace.StringAttribute("service", service),
			trace.StringAttribute("method", method))
	}

	return s.intercept(ctx, req,
		&grpc.UnaryServerInfo{Server: s, FullMethod: fqmn(service, method)}, handler)
}

func (s *server) ListImages(ctx context.Context,
	req *criv1.ListImagesRequest) (*criv1.ListImagesResponse, error) {
	rsp, err := s.interceptRequest(ctx, imageService, listImages, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.image).ListImages(ctx, req.(*criv1.ListImagesRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*criv1.ListImagesResponse), err
}

func (s *server) ImageStatus(ctx context.Context,
	req *criv1.ImageStatusRequest) (*criv1.ImageStatusResponse, error) {
	rsp, err := s.interceptRequest(ctx, imageService, imageStatus, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.image).ImageStatus(ctx, req.(*criv1.ImageStatusRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*criv1.ImageStatusResponse), err
}

func (s *server) PullImage(ctx context.Context,
	req *criv1.PullImageRequest) (*criv1.PullImageResponse, error) {
	rsp, err := s.interceptRequest(ctx, imageService, pullImage, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.image).PullImage(ctx, req.(*criv1.PullImageRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*criv1.PullImageResponse), err
}

func (s *server) RemoveImage(ctx context.Context,
	req *criv1.RemoveImageRequest) (*criv1.RemoveImageResponse, error) {
	rsp, err := s.interceptRequest(ctx, imageService, removeImage, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.image).RemoveImage(ctx, req.(*criv1.RemoveImageRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*criv1.RemoveImageResponse), err
}

func (s *server) ImageFsInfo(ctx context.Context,
	req *criv1.ImageFsInfoRequest) (*criv1.ImageFsInfoResponse, error) {
	rsp, err := s.interceptRequest(ctx, imageService, imageFsInfo, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.image).ImageFsInfo(ctx, req.(*criv1.ImageFsInfoRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*criv1.ImageFsInfoResponse), err
}

func (s *server) Version(ctx context.Context,
	req *criv1.VersionRequest) (*criv1.VersionResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, version, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).Version(ctx, req.(*criv1.VersionRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*criv1.VersionResponse), err
}

func (s *server) RunPodSandbox(ctx context.Context,
	req *criv1.RunPodSandboxRequest) (*criv1.RunPodSandboxResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, runPodSandbox, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).RunPodSandbox(ctx, req.(*criv1.RunPodSandboxRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*criv1.RunPodSandboxResponse), err
}

func (s *server) StopPodSandbox(ctx context.Context,
	req *criv1.StopPodSandboxRequest) (*criv1.StopPodSandboxResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, stopPodSandbox, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).StopPodSandbox(ctx, req.(*criv1.StopPodSandboxRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*criv1.StopPodSandboxResponse), err
}

func (s *server) RemovePodSandbox(ctx context.Context,
	req *criv1.RemovePodSandboxRequest) (*criv1.RemovePodSandboxResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, removePodSandbox, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).RemovePodSandbox(ctx, req.(*criv1.RemovePodSandboxRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*criv1.RemovePodSandboxResponse), err
}

func (s *server) PodSandboxStatus(ctx context.Context,
	req *criv1.PodSandboxStatusRequest) (*criv1.PodSandboxStatusResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, podSandboxStatus, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).PodSandboxStatus(ctx, req.(*criv1.PodSandboxStatusRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*criv1.PodSandboxStatusResponse), err
}

func (s *server) ListPodSandbox(ctx context.Context,
	req *criv1.ListPodSandboxRequest) (*criv1.ListPodSandboxResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, listPodSandbox, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).ListPodSandbox(ctx, req.(*criv1.ListPodSandboxRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*criv1.ListPodSandboxResponse), err
}

func (s *server) CreateContainer(ctx context.Context,
	req *criv1.CreateContainerRequest) (*criv1.CreateContainerResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, createContainer, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).CreateContainer(ctx, req.(*criv1.CreateContainerRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*criv1.CreateContainerResponse), err
}

func (s *server) StartContainer(ctx context.Context,
	req *criv1.StartContainerRequest) (*criv1.StartContainerResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, startContainer, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).StartContainer(ctx, req.(*criv1.StartContainerRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*criv1.StartContainerResponse), err
}

func (s *server) StopContainer(ctx context.Context,
	req *criv1.StopContainerRequest) (*criv1.StopContainerResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, stopContainer, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).StopContainer(ctx, req.(*criv1.StopContainerRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*criv1.StopContainerResponse), err
}

func (s *server) RemoveContainer(ctx context.Context,
	req *criv1.RemoveContainerRequest) (*criv1.RemoveContainerResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, removeContainer, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).RemoveContainer(ctx, req.(*criv1.RemoveContainerRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*criv1.RemoveContainerResponse), err
}

func (s *server) ListContainers(ctx context.Context,
	req *criv1.ListContainersRequest) (*criv1.ListContainersResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, listContainers, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).ListContainers(ctx, req.(*criv1.ListContainersRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*criv1.ListContainersResponse), err
}

func (s *server) ContainerStatus(ctx context.Context,
	req *criv1.ContainerStatusRequest) (*criv1.ContainerStatusResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, containerStatus, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).ContainerStatus(ctx, req.(*criv1.ContainerStatusRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*criv1.ContainerStatusResponse), err
}

func (s *server) UpdateContainerResources(ctx context.Context,
	req *criv1.UpdateContainerResourcesRequest) (*criv1.UpdateContainerResourcesResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, updateContainerResources, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).UpdateContainerResources(ctx,
				req.(*criv1.UpdateContainerResourcesRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*criv1.UpdateContainerResourcesResponse), err
}

func (s *server) ReopenContainerLog(ctx context.Context,
	req *criv1.ReopenContainerLogRequest) (*criv1.ReopenContainerLogResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, reopenContainerLog, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).ReopenContainerLog(ctx, req.(*criv1.ReopenContainerLogRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*criv1.ReopenContainerLogResponse), err
}

func (s *server) ExecSync(ctx context.Context,
	req *criv1.ExecSyncRequest) (*criv1.ExecSyncResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, execSync, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).ExecSync(ctx, req.(*criv1.ExecSyncRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*criv1.ExecSyncResponse), err
}

func (s *server) Exec(ctx context.Context,
	req *criv1.ExecRequest) (*criv1.ExecResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, exec, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).Exec(ctx, req.(*criv1.ExecRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*criv1.ExecResponse), err
}

func (s *server) Attach(ctx context.Context,
	req *criv1.AttachRequest) (*criv1.AttachResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, attach, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).Attach(ctx, req.(*criv1.AttachRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*criv1.AttachResponse), err
}

func (s *server) PortForward(ctx context.Context,
	req *criv1.PortForwardRequest) (*criv1.PortForwardResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, portForward, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).PortForward(ctx, req.(*criv1.PortForwardRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*criv1.PortForwardResponse), err
}

func (s *server) ContainerStats(ctx context.Context,
	req *criv1.ContainerStatsRequest) (*criv1.ContainerStatsResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, containerStats, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).ContainerStats(ctx, req.(*criv1.ContainerStatsRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*criv1.ContainerStatsResponse), err
}

func (s *server) ListContainerStats(ctx context.Context,
	req *criv1.ListContainerStatsRequest) (*criv1.ListContainerStatsResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, listContainerStats, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).ListContainerStats(ctx, req.(*criv1.ListContainerStatsRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*criv1.ListContainerStatsResponse), err
}

func (s *server) PodSandboxStats(ctx context.Context, req *criv1.PodSandboxStatsRequest) (*criv1.PodSandboxStatsResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, podSandboxStats, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).PodSandboxStats(ctx, req.(*criv1.PodSandboxStatsRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*criv1.PodSandboxStatsResponse), err
}

func (s *server) ListPodSandboxStats(ctx context.Context, req *criv1.ListPodSandboxStatsRequest) (*criv1.ListPodSandboxStatsResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, listPodSandboxStats, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).ListPodSandboxStats(ctx, req.(*criv1.ListPodSandboxStatsRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*criv1.ListPodSandboxStatsResponse), err
}

func (s *server) UpdateRuntimeConfig(ctx context.Context,
	req *criv1.UpdateRuntimeConfigRequest) (*criv1.UpdateRuntimeConfigResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, updateRuntimeConfig, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).UpdateRuntimeConfig(ctx, req.(*criv1.UpdateRuntimeConfigRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*criv1.UpdateRuntimeConfigResponse), err
}

func (s *server) Status(ctx context.Context,
	req *criv1.StatusRequest) (*criv1.StatusResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, status, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).Status(ctx, req.(*criv1.StatusRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*criv1.StatusResponse), err
}

func (s *server) CheckpointContainer(ctx context.Context, req *criv1.CheckpointContainerRequest) (*criv1.CheckpointContainerResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, checkpointContainer, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).CheckpointContainer(ctx, req.(*criv1.CheckpointContainerRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*criv1.CheckpointContainerResponse), err
}

func (s *server) GetContainerEvents(_ *criv1.GetEventsRequest, _ criv1.RuntimeService_GetContainerEventsServer) error {
	return grpcstatus.Errorf(grpccodes.Unimplemented, "GetContainerEvents not implemented")
}
