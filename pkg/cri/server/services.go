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

	api "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

const (
	apiVersion = "v1alpha2"

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
	req *api.ListImagesRequest) (*api.ListImagesResponse, error) {
	rsp, err := s.interceptRequest(ctx, imageService, listImages, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.image).ListImages(ctx, req.(*api.ListImagesRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*api.ListImagesResponse), err
}

func (s *server) ImageStatus(ctx context.Context,
	req *api.ImageStatusRequest) (*api.ImageStatusResponse, error) {
	rsp, err := s.interceptRequest(ctx, imageService, imageStatus, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.image).ImageStatus(ctx, req.(*api.ImageStatusRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*api.ImageStatusResponse), err
}

func (s *server) PullImage(ctx context.Context,
	req *api.PullImageRequest) (*api.PullImageResponse, error) {
	rsp, err := s.interceptRequest(ctx, imageService, pullImage, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.image).PullImage(ctx, req.(*api.PullImageRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*api.PullImageResponse), err
}

func (s *server) RemoveImage(ctx context.Context,
	req *api.RemoveImageRequest) (*api.RemoveImageResponse, error) {
	rsp, err := s.interceptRequest(ctx, imageService, removeImage, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.image).RemoveImage(ctx, req.(*api.RemoveImageRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*api.RemoveImageResponse), err
}

func (s *server) ImageFsInfo(ctx context.Context,
	req *api.ImageFsInfoRequest) (*api.ImageFsInfoResponse, error) {
	rsp, err := s.interceptRequest(ctx, imageService, imageFsInfo, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.image).ImageFsInfo(ctx, req.(*api.ImageFsInfoRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*api.ImageFsInfoResponse), err
}

func (s *server) Version(ctx context.Context,
	req *api.VersionRequest) (*api.VersionResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, version, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).Version(ctx, req.(*api.VersionRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*api.VersionResponse), err
}

func (s *server) RunPodSandbox(ctx context.Context,
	req *api.RunPodSandboxRequest) (*api.RunPodSandboxResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, runPodSandbox, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).RunPodSandbox(ctx, req.(*api.RunPodSandboxRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*api.RunPodSandboxResponse), err
}

func (s *server) StopPodSandbox(ctx context.Context,
	req *api.StopPodSandboxRequest) (*api.StopPodSandboxResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, stopPodSandbox, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).StopPodSandbox(ctx, req.(*api.StopPodSandboxRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*api.StopPodSandboxResponse), err
}

func (s *server) RemovePodSandbox(ctx context.Context,
	req *api.RemovePodSandboxRequest) (*api.RemovePodSandboxResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, removePodSandbox, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).RemovePodSandbox(ctx, req.(*api.RemovePodSandboxRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*api.RemovePodSandboxResponse), err
}

func (s *server) PodSandboxStatus(ctx context.Context,
	req *api.PodSandboxStatusRequest) (*api.PodSandboxStatusResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, podSandboxStatus, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).PodSandboxStatus(ctx, req.(*api.PodSandboxStatusRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*api.PodSandboxStatusResponse), err
}

func (s *server) ListPodSandbox(ctx context.Context,
	req *api.ListPodSandboxRequest) (*api.ListPodSandboxResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, listPodSandbox, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).ListPodSandbox(ctx, req.(*api.ListPodSandboxRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*api.ListPodSandboxResponse), err
}

func (s *server) CreateContainer(ctx context.Context,
	req *api.CreateContainerRequest) (*api.CreateContainerResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, createContainer, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).CreateContainer(ctx, req.(*api.CreateContainerRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*api.CreateContainerResponse), err
}

func (s *server) StartContainer(ctx context.Context,
	req *api.StartContainerRequest) (*api.StartContainerResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, startContainer, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).StartContainer(ctx, req.(*api.StartContainerRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*api.StartContainerResponse), err
}

func (s *server) StopContainer(ctx context.Context,
	req *api.StopContainerRequest) (*api.StopContainerResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, stopContainer, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).StopContainer(ctx, req.(*api.StopContainerRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*api.StopContainerResponse), err
}

func (s *server) RemoveContainer(ctx context.Context,
	req *api.RemoveContainerRequest) (*api.RemoveContainerResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, removeContainer, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).RemoveContainer(ctx, req.(*api.RemoveContainerRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*api.RemoveContainerResponse), err
}

func (s *server) ListContainers(ctx context.Context,
	req *api.ListContainersRequest) (*api.ListContainersResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, listContainers, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).ListContainers(ctx, req.(*api.ListContainersRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*api.ListContainersResponse), err
}

func (s *server) ContainerStatus(ctx context.Context,
	req *api.ContainerStatusRequest) (*api.ContainerStatusResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, containerStatus, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).ContainerStatus(ctx, req.(*api.ContainerStatusRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*api.ContainerStatusResponse), err
}

func (s *server) UpdateContainerResources(ctx context.Context,
	req *api.UpdateContainerResourcesRequest) (*api.UpdateContainerResourcesResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, updateContainerResources, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).UpdateContainerResources(ctx,
				req.(*api.UpdateContainerResourcesRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*api.UpdateContainerResourcesResponse), err
}

func (s *server) ReopenContainerLog(ctx context.Context,
	req *api.ReopenContainerLogRequest) (*api.ReopenContainerLogResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, reopenContainerLog, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).ReopenContainerLog(ctx, req.(*api.ReopenContainerLogRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*api.ReopenContainerLogResponse), err
}

func (s *server) ExecSync(ctx context.Context,
	req *api.ExecSyncRequest) (*api.ExecSyncResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, execSync, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).ExecSync(ctx, req.(*api.ExecSyncRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*api.ExecSyncResponse), err
}

func (s *server) Exec(ctx context.Context,
	req *api.ExecRequest) (*api.ExecResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, exec, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).Exec(ctx, req.(*api.ExecRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*api.ExecResponse), err
}

func (s *server) Attach(ctx context.Context,
	req *api.AttachRequest) (*api.AttachResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, attach, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).Attach(ctx, req.(*api.AttachRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*api.AttachResponse), err
}

func (s *server) PortForward(ctx context.Context,
	req *api.PortForwardRequest) (*api.PortForwardResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, portForward, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).PortForward(ctx, req.(*api.PortForwardRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*api.PortForwardResponse), err
}

func (s *server) ContainerStats(ctx context.Context,
	req *api.ContainerStatsRequest) (*api.ContainerStatsResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, containerStats, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).ContainerStats(ctx, req.(*api.ContainerStatsRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*api.ContainerStatsResponse), err
}

func (s *server) ListContainerStats(ctx context.Context,
	req *api.ListContainerStatsRequest) (*api.ListContainerStatsResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, listContainerStats, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).ListContainerStats(ctx, req.(*api.ListContainerStatsRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*api.ListContainerStatsResponse), err
}

func (s *server) PodSandboxStats(ctx context.Context, req *api.PodSandboxStatsRequest) (*api.PodSandboxStatsResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, podSandboxStats, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).PodSandboxStats(ctx, req.(*api.PodSandboxStatsRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*api.PodSandboxStatsResponse), err
}

func (s *server) ListPodSandboxStats(ctx context.Context, req *api.ListPodSandboxStatsRequest) (*api.ListPodSandboxStatsResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, listPodSandboxStats, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).ListPodSandboxStats(ctx, req.(*api.ListPodSandboxStatsRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*api.ListPodSandboxStatsResponse), err
}

func (s *server) UpdateRuntimeConfig(ctx context.Context,
	req *api.UpdateRuntimeConfigRequest) (*api.UpdateRuntimeConfigResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, updateRuntimeConfig, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).UpdateRuntimeConfig(ctx, req.(*api.UpdateRuntimeConfigRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*api.UpdateRuntimeConfigResponse), err
}

func (s *server) Status(ctx context.Context,
	req *api.StatusRequest) (*api.StatusResponse, error) {
	rsp, err := s.interceptRequest(ctx, runtimeService, status, req,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return (*s.runtime).Status(ctx, req.(*api.StatusRequest))
		})

	if err != nil {
		return nil, err
	}

	return rsp.(*api.StatusResponse), err
}
