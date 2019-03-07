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

package relay

import (
	"context"
	api "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (r *relay) Version(ctx context.Context,
	req *api.VersionRequest) (*api.VersionResponse, error) {
	return r.client.Version(ctx, req)
}

func (r *relay) RunPodSandbox(ctx context.Context,
	req *api.RunPodSandboxRequest) (*api.RunPodSandboxResponse, error) {
	return r.client.RunPodSandbox(ctx, req)
}

func (r *relay) StopPodSandbox(ctx context.Context,
	req *api.StopPodSandboxRequest) (*api.StopPodSandboxResponse, error) {
	return r.client.StopPodSandbox(ctx, req)
}

func (r *relay) RemovePodSandbox(ctx context.Context,
	req *api.RemovePodSandboxRequest) (*api.RemovePodSandboxResponse, error) {
	return r.client.RemovePodSandbox(ctx, req)
}

func (r *relay) PodSandboxStatus(ctx context.Context,
	req *api.PodSandboxStatusRequest) (*api.PodSandboxStatusResponse, error) {
	return r.client.PodSandboxStatus(ctx, req)
}

func (r *relay) ListPodSandbox(ctx context.Context,
	req *api.ListPodSandboxRequest) (*api.ListPodSandboxResponse, error) {
	return r.client.ListPodSandbox(ctx, req)
}

func (r *relay) CreateContainer(ctx context.Context,
	req *api.CreateContainerRequest) (*api.CreateContainerResponse, error) {
	return r.client.CreateContainer(ctx, req)
}

func (r *relay) StartContainer(ctx context.Context,
	req *api.StartContainerRequest) (*api.StartContainerResponse, error) {
	return r.client.StartContainer(ctx, req)
}

func (r *relay) StopContainer(ctx context.Context,
	req *api.StopContainerRequest) (*api.StopContainerResponse, error) {
	return r.client.StopContainer(ctx, req)
}

func (r *relay) RemoveContainer(ctx context.Context,
	req *api.RemoveContainerRequest) (*api.RemoveContainerResponse, error) {
	return r.client.RemoveContainer(ctx, req)
}

func (r *relay) ListContainers(ctx context.Context,
	req *api.ListContainersRequest) (*api.ListContainersResponse, error) {
	return r.client.ListContainers(ctx, req)
}

func (r *relay) ContainerStatus(ctx context.Context,
	req *api.ContainerStatusRequest) (*api.ContainerStatusResponse, error) {
	return r.client.ContainerStatus(ctx, req)
}

func (r *relay) UpdateContainerResources(ctx context.Context,
	req *api.UpdateContainerResourcesRequest) (*api.UpdateContainerResourcesResponse, error) {
	return r.client.UpdateContainerResources(ctx, req)
}

func (r *relay) ReopenContainerLog(ctx context.Context,
	req *api.ReopenContainerLogRequest) (*api.ReopenContainerLogResponse, error) {
	return r.client.ReopenContainerLog(ctx, req)
}

func (r *relay) ExecSync(ctx context.Context,
	req *api.ExecSyncRequest) (*api.ExecSyncResponse, error) {
	return r.client.ExecSync(ctx, req)
}

func (r *relay) Exec(ctx context.Context,
	req *api.ExecRequest) (*api.ExecResponse, error) {
	return r.client.Exec(ctx, req)
}

func (r *relay) Attach(ctx context.Context,
	req *api.AttachRequest) (*api.AttachResponse, error) {
	return r.client.Attach(ctx, req)
}

func (r *relay) PortForward(ctx context.Context,
	req *api.PortForwardRequest) (*api.PortForwardResponse, error) {
	return r.client.PortForward(ctx, req)
}

func (r *relay) ContainerStats(ctx context.Context,
	req *api.ContainerStatsRequest) (*api.ContainerStatsResponse, error) {
	return r.client.ContainerStats(ctx, req)
}

func (r *relay) ListContainerStats(ctx context.Context,
	req *api.ListContainerStatsRequest) (*api.ListContainerStatsResponse, error) {
	return r.client.ListContainerStats(ctx, req)
}

func (r *relay) UpdateRuntimeConfig(ctx context.Context,
	req *api.UpdateRuntimeConfigRequest) (*api.UpdateRuntimeConfigResponse, error) {
	return r.client.UpdateRuntimeConfig(ctx, req)
}

func (r *relay) Status(ctx context.Context,
	req *api.StatusRequest) (*api.StatusResponse, error) {
	return r.client.Status(ctx, req)
}
