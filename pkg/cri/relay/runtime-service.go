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
	"fmt"
	"time"

	criv1 "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/intel/cri-resource-manager/pkg/dump"
)

func (r *relay) dump(method string, req interface{}) {
	if r.DebugEnabled() {
		qualif := r.qualifier(req)
		dump.RequestMessage("relayed", method, qualif, req, true)
	}
}

// qualifier pulls a qualifier for disambiguation from a CRI request message.
func (r *relay) qualifier(msg interface{}) string {
	if fn := r.options.QualifyReqFn; fn != nil {
		return fn(msg)
	}
	return ""
}

func (r *relay) Version(ctx context.Context,
	req *criv1.VersionRequest) (*criv1.VersionResponse, error) {
	r.dump("Version", req)
	return r.client.Version(ctx, req)
}

func (r *relay) RunPodSandbox(ctx context.Context,
	req *criv1.RunPodSandboxRequest) (*criv1.RunPodSandboxResponse, error) {
	r.dump("RunPodSandbox", req)
	return r.client.RunPodSandbox(ctx, req)
}

func (r *relay) StopPodSandbox(ctx context.Context,
	req *criv1.StopPodSandboxRequest) (*criv1.StopPodSandboxResponse, error) {
	r.dump("StopPodSandbox", req)
	return r.client.StopPodSandbox(ctx, req)
}

func (r *relay) RemovePodSandbox(ctx context.Context,
	req *criv1.RemovePodSandboxRequest) (*criv1.RemovePodSandboxResponse, error) {
	r.dump("RemovePodSandbox", req)
	return r.client.RemovePodSandbox(ctx, req)
}

func (r *relay) PodSandboxStatus(ctx context.Context,
	req *criv1.PodSandboxStatusRequest) (*criv1.PodSandboxStatusResponse, error) {
	r.dump("PodSandboxStatus", req)
	return r.client.PodSandboxStatus(ctx, req)
}

func (r *relay) ListPodSandbox(ctx context.Context,
	req *criv1.ListPodSandboxRequest) (*criv1.ListPodSandboxResponse, error) {
	r.dump("ListPodSandbox", req)
	return r.client.ListPodSandbox(ctx, req)
}

func (r *relay) CreateContainer(ctx context.Context,
	req *criv1.CreateContainerRequest) (*criv1.CreateContainerResponse, error) {
	r.dump("CreateContainer", req)
	return r.client.CreateContainer(ctx, req)
}

func (r *relay) StartContainer(ctx context.Context,
	req *criv1.StartContainerRequest) (*criv1.StartContainerResponse, error) {
	r.dump("StartContainer", req)
	return r.client.StartContainer(ctx, req)
}

func (r *relay) StopContainer(ctx context.Context,
	req *criv1.StopContainerRequest) (*criv1.StopContainerResponse, error) {
	r.dump("StopContainer", req)
	return r.client.StopContainer(ctx, req)
}

func (r *relay) RemoveContainer(ctx context.Context,
	req *criv1.RemoveContainerRequest) (*criv1.RemoveContainerResponse, error) {
	r.dump("RemoveContainer", req)
	return r.client.RemoveContainer(ctx, req)
}

func (r *relay) ListContainers(ctx context.Context,
	req *criv1.ListContainersRequest) (*criv1.ListContainersResponse, error) {
	r.dump("ListContainers", req)
	return r.client.ListContainers(ctx, req)
}

func (r *relay) ContainerStatus(ctx context.Context,
	req *criv1.ContainerStatusRequest) (*criv1.ContainerStatusResponse, error) {
	r.dump("ContainerStatus", req)
	return r.client.ContainerStatus(ctx, req)
}

func (r *relay) UpdateContainerResources(ctx context.Context,
	req *criv1.UpdateContainerResourcesRequest) (*criv1.UpdateContainerResourcesResponse, error) {
	r.dump("UpdateContainerResources", req)
	return r.client.UpdateContainerResources(ctx, req)
}

func (r *relay) ReopenContainerLog(ctx context.Context,
	req *criv1.ReopenContainerLogRequest) (*criv1.ReopenContainerLogResponse, error) {
	r.dump("ReopenContainerLog", req)
	return r.client.ReopenContainerLog(ctx, req)
}

func (r *relay) ExecSync(ctx context.Context,
	req *criv1.ExecSyncRequest) (*criv1.ExecSyncResponse, error) {
	r.dump("ExecSync", req)
	return r.client.ExecSync(ctx, req)
}

func (r *relay) Exec(ctx context.Context,
	req *criv1.ExecRequest) (*criv1.ExecResponse, error) {
	r.dump("Exec", req)
	return r.client.Exec(ctx, req)
}

func (r *relay) Attach(ctx context.Context,
	req *criv1.AttachRequest) (*criv1.AttachResponse, error) {
	r.dump("Attach", req)
	return r.client.Attach(ctx, req)
}

func (r *relay) PortForward(ctx context.Context,
	req *criv1.PortForwardRequest) (*criv1.PortForwardResponse, error) {
	r.dump("PortForward", req)
	return r.client.PortForward(ctx, req)
}

func (r *relay) ContainerStats(ctx context.Context,
	req *criv1.ContainerStatsRequest) (*criv1.ContainerStatsResponse, error) {
	r.dump("ContainerStats", req)
	return r.client.ContainerStats(ctx, req)
}

func (r *relay) ListContainerStats(ctx context.Context,
	req *criv1.ListContainerStatsRequest) (*criv1.ListContainerStatsResponse, error) {
	r.dump("ListContainerStats", req)
	return r.client.ListContainerStats(ctx, req)
}

func (r *relay) PodSandboxStats(ctx context.Context,
	req *criv1.PodSandboxStatsRequest) (*criv1.PodSandboxStatsResponse, error) {
	r.dump("PodSandboxStats", req)
	return r.client.PodSandboxStats(ctx, req)
}

func (r *relay) ListPodSandboxStats(ctx context.Context,
	req *criv1.ListPodSandboxStatsRequest) (*criv1.ListPodSandboxStatsResponse, error) {
	r.dump("ListPodSandboxStats", req)
	return r.client.ListPodSandboxStats(ctx, req)
}

func (r *relay) UpdateRuntimeConfig(ctx context.Context,
	req *criv1.UpdateRuntimeConfigRequest) (*criv1.UpdateRuntimeConfigResponse, error) {
	r.dump("UpdateRuntimeConfig", req)
	return r.client.UpdateRuntimeConfig(ctx, req)
}

func (r *relay) Status(ctx context.Context,
	req *criv1.StatusRequest) (*criv1.StatusResponse, error) {
	r.dump("Status", req)
	return r.client.Status(ctx, req)
}

func (r *relay) CheckpointContainer(ctx context.Context, req *criv1.CheckpointContainerRequest) (*criv1.CheckpointContainerResponse, error) {
	r.dump("CheckpointContainer", req)
	return r.client.CheckpointContainer(ctx, req)
}

func (r *relay) GetContainerEvents(req *criv1.GetEventsRequest, srv criv1.RuntimeService_GetContainerEventsServer) error {
	evtC := r.addEventServer(req)

	if err := r.startEventRelay(req); err != nil {
		r.delEventServer(req)
		return err
	}

	for evt := range evtC {
		if err := srv.Send(evt); err != nil {
			r.Errorf("failed to relay/send container event: %v", err)
			r.delEventServer(req)
			return err
		}
	}

	return nil
}

func (r *relay) ListMetricDescriptors(ctx context.Context, req *criv1.ListMetricDescriptorsRequest) (*criv1.ListMetricDescriptorsResponse, error) {
	r.dump("ListMetricDescriptors", req)
	return r.client.ListMetricDescriptors(ctx, req)
}

func (r *relay) ListPodSandboxMetrics(ctx context.Context, req *criv1.ListPodSandboxMetricsRequest) (*criv1.ListPodSandboxMetricsResponse, error) {
	r.dump("ListPodSandboxMetrics", req)
	return r.client.ListPodSandboxMetrics(ctx, req)
}

func (r *relay) RuntimeConfig(ctx context.Context, req *criv1.RuntimeConfigRequest) (*criv1.RuntimeConfigResponse, error) {
	r.dump("RuntimeConfig", req)
	return r.client.RuntimeConfig(ctx, req)
}

const (
	eventRelayTimeout = 1 * time.Second
)

func (r *relay) addEventServer(req *criv1.GetEventsRequest) chan *criv1.ContainerEventResponse {
	r.Lock()
	defer r.Unlock()

	evtC := make(chan *criv1.ContainerEventResponse, 128)
	r.evtChans[req] = evtC

	return evtC
}

func (r *relay) delEventServer(req *criv1.GetEventsRequest) chan *criv1.ContainerEventResponse {
	r.Lock()
	defer r.Unlock()

	evtC := r.evtChans[req]
	delete(r.evtChans, req)

	return evtC
}

func (r *relay) startEventRelay(req *criv1.GetEventsRequest) error {
	r.Lock()
	defer r.Unlock()

	if r.evtClient != nil {
		return nil
	}

	c, err := r.client.GetContainerEvents(context.Background(), req)
	if err != nil {
		return fmt.Errorf("failed to create container event client: %w", err)
	}

	r.evtClient = c
	go r.relayEvents()

	return nil
}

func (r *relay) relayEvents() {
	for {
		evt, err := r.evtClient.Recv()
		if err != nil {
			r.Errorf("failed to relay/receive container event: %v", err)
		}

		r.Lock()

		if err != nil {
			for req, evtC := range r.evtChans {
				delete(r.evtChans, req)
				close(evtC)
			}
			r.evtClient = nil
		} else {
			for req, evtC := range r.evtChans {
				select {
				case evtC <- evt:
				case _ = <-time.After(eventRelayTimeout):
					delete(r.evtChans, req)
					close(evtC)
				}
			}
		}

		r.Unlock()

		if err != nil {
			return
		}
	}
}
