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

package resmgr

import (
	"context"
	"fmt"

	criapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"

	pkgcfg "github.com/intel/cri-resource-manager/pkg/config"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	config "github.com/intel/cri-resource-manager/pkg/cri/resource-manager/config"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/events"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/policy"
	"github.com/intel/cri-resource-manager/pkg/cri/server"
)

// setupRequestProcessing prepares the resource manager for CRI request processing.
func (m *resmgr) setupRequestProcessing() error {
	interceptors := map[string]server.Interceptor{
		"RunPodSandbox":    m.RunPod,
		"StopPodSandbox":   m.StopPod,
		"RemovePodSandbox": m.RemovePod,

		"CreateContainer": m.CreateContainer,
		"StartContainer":  m.StartContainer,
		"StopContainer":   m.StopContainer,
		"RemoveContainer": m.RemoveContainer,
		"ListContainers":  m.ListContainers,

		"UpdateContainerResources": m.UpdateContainer,
	}

	if err := m.relay.Server().RegisterInterceptors(interceptors); err != nil {
		return resmgrError("failed to register resource-manager CRI interceptors: %v", err)
	}

	m.relay.Server().SetBypassCheckFn(m.policy.Bypassed)

	return nil
}

// disambiguate produces disambiguation context for a request/reply dump.
func (m *resmgr) disambiguate(msg interface{}) string {
	var qualifier string

	m.RLock()
	defer m.RUnlock()

	switch req := msg.(type) {
	case *criapi.RunPodSandboxRequest:
		if req.Config != nil && req.Config.Metadata != nil {
			qualifier = req.Config.Metadata.Name
		}
	case *criapi.StopPodSandboxRequest:
		if pod, ok := m.cache.LookupPod(req.PodSandboxId); ok {
			qualifier = pod.GetName()
		} else {
			qualifier = "unknown pod " + req.PodSandboxId
		}
	case *criapi.RemovePodSandboxRequest:
		if pod, ok := m.cache.LookupPod(req.PodSandboxId); ok {
			qualifier = pod.GetName()
		} else {
			qualifier = "unknown pod " + req.PodSandboxId
		}

	case *criapi.CreateContainerRequest:
		switch {
		case req.SandboxConfig == nil || req.SandboxConfig.Metadata == nil:
			qualifier = "missing pod metadata in request"
		case req.Config == nil || req.Config.Metadata == nil:
			qualifier = "missing metadata in request"
		default:
			qualifier = req.SandboxConfig.Metadata.Name + ":" + req.Config.Metadata.Name
		}

	case *criapi.StartContainerRequest:
		if container, ok := m.cache.LookupContainer(req.ContainerId); ok {
			qualifier = container.PrettyName()
		} else {
			qualifier = "unknown container " + req.ContainerId
		}
	case *criapi.StopContainerRequest:
		if container, ok := m.cache.LookupContainer(req.ContainerId); ok {
			qualifier = container.PrettyName()
		} else {
			qualifier = "unknown container " + req.ContainerId
		}
	case *criapi.RemoveContainerRequest:
		if container, ok := m.cache.LookupContainer(req.ContainerId); ok {
			qualifier = container.PrettyName()
		} else {
			qualifier = "unknown container " + req.ContainerId
		}

	case *criapi.UpdateContainerResourcesRequest:
		if container, ok := m.cache.LookupContainer(req.ContainerId); ok {
			qualifier = container.PrettyName()
		} else {
			qualifier = "unknown container " + req.ContainerId
		}
	}

	if qualifier != "" {
		return "<" + qualifier + ">"
	}

	return ""
}

// startRequestProcessing starts request processing by starting the active policy.
func (m *resmgr) startRequestProcessing() error {
	ctx := context.Background()
	add, del, err := m.syncWithCRI(ctx)

	if err != nil {
		return err
	}

	//
	// Notes:
	//   While normally it is enough to release stale containers and allocate
	//   newly discovered ones, if we are switching policies we need to force
	//   reallocating everything. Otherwise containers already present in the
	//   cache would not get properly updated by the new policy.
	//
	if m.policySwitch {
		containers := m.cache.GetContainers()
		cache.SortContainers(containers)
		add, del = containers, containers
		m.policySwitch = false
	}

	if err := m.policy.Start(add, del); err != nil {
		return resmgrError("failed to start policy %s: %v", policy.ActivePolicy(), err)
	}

	if err := m.runPostReleaseHooks(ctx, "startup", del...); err != nil {
		m.Error("startup: failed to run post-release hooks: %v", err)
	}

	return nil
}

// syncWithCRI synchronizes cache pods and containers with the CRI runtime.
func (m *resmgr) syncWithCRI(ctx context.Context) ([]cache.Container, []cache.Container, error) {
	if m.policy.Bypassed() || !m.relay.Client().HasRuntimeService() {
		return nil, nil, nil
	}

	m.Info("synchronizing cache state with CRI runtime...")

	add, del := []cache.Container{}, []cache.Container{}

	pods, err := m.relay.Client().ListPodSandbox(ctx, &criapi.ListPodSandboxRequest{})
	if err != nil {
		return nil, nil, resmgrError("cache synchronization container query failed: %v", err)
	}
	_, _, added, deleted := m.cache.Refresh(pods)
	for _, c := range added {
		if c.GetState() != cache.ContainerStateRunning {
			m.Info("ignoring discovered container %s (in state %v)...",
				c.GetID(), c.GetState())
			continue
		}
		m.Info("discovered out-of-sync running container %s...", c.GetID())
		add = append(add, c)
	}
	for _, c := range deleted {
		m.Info("discovered stale container %s...", c.GetID())
		del = append(del, c)
	}

	containers, err := m.relay.Client().ListContainers(ctx, &criapi.ListContainersRequest{})
	if err != nil {
		return nil, nil, resmgrError("cache synchronization container query failed: %v", err)
	}
	_, _, added, deleted = m.cache.Refresh(containers)
	for _, c := range added {
		if c.GetState() != cache.ContainerStateRunning {
			m.Info("ignoring discovered container %s (in state %v)...",
				c.GetID(), c.GetState())
			continue
		}
		m.Info("discovered out-of-sync running container %s...", c.GetID())
		add = append(add, c)
	}
	for _, c := range deleted {
		m.Info("discovered stale container %s...", c.GetID())
		del = append(del, c)
	}

	return add, del, nil
}

// RunPod intercepts CRI requests for Pod creation.
func (m *resmgr) RunPod(ctx context.Context, method string, request interface{},
	handler server.Handler) (interface{}, error) {

	reply, rqerr := handler(ctx, request)
	if rqerr != nil {
		m.Error("%s: failed to create pod: %v", method, rqerr)
		return reply, rqerr
	}

	m.Lock()
	defer m.Unlock()

	podID := reply.(*criapi.RunPodSandboxResponse).PodSandboxId
	pod := m.cache.InsertPod(podID, request)
	m.updateIntrospection()

	// search for any lingering old version and clean up if found
	released := false
	del := []cache.Container{}
	for _, p := range m.cache.GetPods() {
		if p.GetUID() != pod.GetUID() || p == pod {
			continue
		}
		m.Warn("re-creation of pod %s, releasing old one", p.GetName())
		for _, c := range pod.GetInitContainers() {
			m.Info("%s: removing stale init-container %s...", method, c.PrettyName())
			m.policy.ReleaseResources(c)
			c.UpdateState(cache.ContainerStateStale)
			released = true
			del = append(del, c)
		}
		for _, c := range pod.GetContainers() {
			m.Info("%s: removing stale container %s...", method, c.PrettyName())
			m.policy.ReleaseResources(c)
			c.UpdateState(cache.ContainerStateStale)
			released = true
			del = append(del, c)
		}
		m.cache.DeletePod(p.GetID())
	}
	if released {
		if err := m.runPostReleaseHooks(ctx, method, del...); err != nil {
			m.Error("%s: failed to run post-release hooks for lingering pod %s: %v",
				method, pod.GetName(), err)
		}
	}

	m.Info("created pod %s (%s)", pod.GetName(), podID)

	return reply, nil
}

// StopPod intercepts CRI requests for stopping Pods.
func (m *resmgr) StopPod(ctx context.Context, method string, request interface{},
	handler server.Handler) (interface{}, error) {

	reply, rqerr := handler(ctx, request)

	m.Lock()
	defer m.Unlock()

	podID := request.(*criapi.StopPodSandboxRequest).PodSandboxId
	pod, ok := m.cache.LookupPod(podID)

	if !ok {
		m.Warn("%s: failed to look up pod %s, just passing request through", method, podID)
		return reply, rqerr
	}

	if rqerr != nil {
		m.Error("%s: failed to stop pod %s: %v", method, podID, rqerr)
		return reply, rqerr
	}

	m.Info("%s: stopped pod %s (%s)...", method, pod.GetName(), podID)

	released := []cache.Container{}
	for _, c := range pod.GetInitContainers() {
		m.Info("%s: releasing resources for %s...", method, c.PrettyName())
		if err := m.policy.ReleaseResources(c); err != nil {
			m.Warn("%s: failed to release init-container %s: %v", method, c.PrettyName(), err)
		}
		c.UpdateState(cache.ContainerStateExited)
		released = append(released, c)
	}
	for _, c := range pod.GetContainers() {
		m.Info("%s: releasing resources for container %s...", method, c.PrettyName())
		if err := m.policy.ReleaseResources(c); err != nil {
			m.Warn("%s: failed to release container %s: %v", method, c.PrettyName(), err)
		}
		c.UpdateState(cache.ContainerStateExited)
		released = append(released, c)
	}

	if err := m.runPostReleaseHooks(ctx, method, released...); err != nil {
		m.Error("%s: failed to run post-release hooks for pod %s: %v",
			method, pod.GetName(), err)
	}

	m.updateIntrospection()

	return reply, rqerr
}

// RemovePod intercepts CRI requests for Pod removal.
func (m *resmgr) RemovePod(ctx context.Context, method string, request interface{},
	handler server.Handler) (interface{}, error) {

	reply, rqerr := handler(ctx, request)

	m.Lock()
	defer m.Unlock()

	podID := request.(*criapi.RemovePodSandboxRequest).PodSandboxId
	pod, ok := m.cache.LookupPod(podID)

	if !ok {
		m.Warn("%s: failed to look up pod %s, just passing request through", method, podID)
		return reply, rqerr
	}

	if rqerr != nil {
		m.Error("%s: failed to remove pod %s: %v", method, podID, rqerr)
	} else {
		m.Info("%s: removed pod %s (%s)...", method, pod.GetName(), podID)
	}

	del := []cache.Container{}
	for _, c := range pod.GetInitContainers() {
		m.Info("%s: removing stale init-container %s...", method, c.PrettyName())
		if err := m.policy.ReleaseResources(c); err != nil {
			m.Warn("%s: failed to release init-container %s: %v", method, c.PrettyName(), err)
		}
		c.UpdateState(cache.ContainerStateStale)
		del = append(del, c)
	}
	for _, c := range pod.GetContainers() {
		m.Info("%s: removing stale container %s...", method, c.PrettyName())
		if err := m.policy.ReleaseResources(c); err != nil {
			m.Warn("%s: failed to release container %s: %v", method, c.PrettyName(), err)
		}
		c.UpdateState(cache.ContainerStateStale)
		del = append(del, c)
	}

	if err := m.runPostReleaseHooks(ctx, method, del...); err != nil {
		m.Error("%s: failed to run post-release hooks for pod %s: %v",
			method, pod.GetName(), err)
	}

	m.cache.DeletePod(podID)
	m.updateIntrospection()

	return reply, rqerr
}

// CreateContainer intercepts CRI requests for Container creation.
func (m *resmgr) CreateContainer(ctx context.Context, method string, request interface{},
	handler server.Handler) (interface{}, error) {

	m.Lock()
	defer m.Unlock()

	// kubelet doesn't always clean up crashed containers so we try doing it here
	if msg, ok := request.(*criapi.CreateContainerRequest); ok {
		if pod, ok := m.cache.LookupPod(msg.PodSandboxId); ok {
			if msg.Config != nil && msg.Config.Metadata != nil {
				if c, ok := pod.GetContainer(msg.Config.Metadata.Name); ok {
					m.Warn("re-creation of container %s, releasing old one", c.PrettyName())
					m.policy.ReleaseResources(c)
				}
			}
		}
	}

	container, err := m.cache.InsertContainer(request)
	if err != nil {
		m.Error("%s: failed to insert new container to cache: %v", method, err)
		return nil, resmgrError("%s: failed to insert new container to cache: %v", method, err)
	}

	container.SetCRIRequest(request)

	m.Info("%s: creating container %s...", method, container.PrettyName())

	if err := m.policy.AllocateResources(container); err != nil {
		m.Error("%s: failed to allocate resources for container %s: %v",
			method, container.PrettyName(), err)
		m.cache.DeleteContainer(container.GetCacheID())
		return nil, resmgrError("failed to allocate container resources: %v", err)
	}

	container.InsertMount(&cache.Mount{
		Container:   "/.cri-resmgr",
		Host:        m.cache.ContainerDirectory(container.GetCacheID()),
		Readonly:    true,
		Propagation: cache.MountHostToContainer,
	})

	if err := m.runPostAllocateHooks(ctx, method); err != nil {
		m.Error("%s: failed to run post-allocate hooks for %s: %v",
			method, container.PrettyName(), err)
		m.policy.ReleaseResources(container)
		m.runPostReleaseHooks(ctx, method, container)
		m.cache.DeleteContainer(container.GetCacheID())
		return nil, resmgrError("failed to allocate container resources: %v", err)
	}

	container.ClearCRIRequest()
	reply, rqerr := handler(ctx, request)

	if rqerr != nil {
		m.Error("%s: failed to create container %s: %v", method, container.PrettyName(), rqerr)
		m.policy.ReleaseResources(container)
		m.runPostReleaseHooks(ctx, method, container)
		m.cache.DeleteContainer(container.GetCacheID())
		return nil, resmgrError("failed to create container: %v", rqerr)
	}

	m.cache.UpdateContainerID(container.GetCacheID(), reply)
	container.UpdateState(cache.ContainerStateCreated)
	m.updateIntrospection()

	return reply, nil
}

// StartContainer intercepts CRI requests for starting Containers.
func (m *resmgr) StartContainer(ctx context.Context, method string, request interface{},
	handler server.Handler) (interface{}, error) {

	m.Lock()
	defer m.Unlock()

	containerID := request.(*criapi.StartContainerRequest).ContainerId
	container, ok := m.cache.LookupContainer(containerID)

	if !ok {
		m.Warn("%s: failed to look up container %s, just passing request through",
			method, containerID)
		return handler(ctx, request)
	}

	m.Info("%s: starting container %s...", method, container.PrettyName())

	if container.GetState() != cache.ContainerStateCreated {
		m.Error("%s: refusing to start container %s in unexpected state %v",
			method, container.PrettyName(), container.GetState())
		return nil, resmgrError("refusing to start container %s in unexpexted state %v",
			container.PrettyName(), container.GetState())
	}

	reply, rqerr := handler(ctx, request)

	if rqerr != nil {
		m.Error("%s: failed to start container %s: %v", method, container.PrettyName(), rqerr)
		return nil, rqerr
	}

	container.UpdateState(cache.ContainerStateRunning)

	e := &events.Policy{
		Type:   events.ContainerStarted,
		Source: "resource-manager",
		Data:   container,
	}
	if _, err := m.policy.HandleEvent(e); err != nil {
		m.Error("%s: policy failed to handle event %s: %v", method, e.Type, err)
	}

	if err := m.runPostStartHooks(ctx, method, container); err != nil {
		m.Error("%s: failed to run post-start hooks for %s: %v",
			method, container.PrettyName(), err)
	}

	m.updateIntrospection()

	return reply, rqerr
}

// StopContainer intercepts CRI requests for stopping Containers.
func (m *resmgr) StopContainer(ctx context.Context, method string, request interface{},
	handler server.Handler) (interface{}, error) {

	reply, rqerr := handler(ctx, request)

	m.Lock()
	defer m.Unlock()

	containerID := request.(*criapi.StopContainerRequest).ContainerId
	container, ok := m.cache.LookupContainer(containerID)

	if !ok {
		m.Warn("%s: failed to look up container %s, just passing request through",
			method, containerID)
		return reply, rqerr
	}

	if rqerr != nil {
		m.Error("%s: failed to stop container %s: %v", method, container.PrettyName(), rqerr)
		return reply, rqerr
	}

	m.Info("%s: stopped container %s...", method, container.PrettyName())

	// Notes:
	//   For now, we assume any error replies from CRI are about the container not
	//   being found, in which case we still go ahead and finish locally stopping it...

	if err := m.policy.ReleaseResources(container); err != nil {
		m.Error("%s: failed to release resources for container %s: %v",
			method, container.PrettyName(), err)
	}

	if err := m.runPostReleaseHooks(ctx, method, container); err != nil {
		m.Error("%s: failed to run post-release hooks for %s: %v",
			method, container.PrettyName(), err)
	}

	container.UpdateState(cache.ContainerStateExited)
	m.updateIntrospection()

	return reply, rqerr
}

// RemoveContainer intercepts CRI requests for Container removal.
func (m *resmgr) RemoveContainer(ctx context.Context, method string, request interface{},
	handler server.Handler) (interface{}, error) {

	reply, rqerr := handler(ctx, request)

	m.Lock()
	defer m.Unlock()

	containerID := request.(*criapi.RemoveContainerRequest).ContainerId
	container, ok := m.cache.LookupContainer(containerID)

	if !ok {
		m.Warn("%s: failed to look up container %s, just passing request through",
			method, containerID)
		return reply, rqerr
	}

	if rqerr != nil {
		m.Error("%s: failed to remove container %s: %v", method, container.PrettyName(), rqerr)
	} else {
		m.Info("%s: removed container %s...", method, container.PrettyName())
	}

	if err := m.policy.ReleaseResources(container); err != nil {
		m.Error("%s: failed to release resources for container %s: %v",
			method, container.PrettyName(), err)
	}

	if err := m.runPostReleaseHooks(ctx, method, container); err != nil {
		m.Error("%s: failed to run post-release hooks for %s: %v",
			method, container.PrettyName(), err)
	}

	m.cache.DeleteContainer(container.GetCacheID())
	m.updateIntrospection()

	return reply, rqerr
}

// ListContainers intercepts CRI requests for listing Containers.
func (m *resmgr) ListContainers(ctx context.Context, method string, request interface{},
	handler server.Handler) (interface{}, error) {

	reply, rqerr := handler(ctx, request)

	if rqerr != nil {
		return reply, rqerr
	}

	if f := request.(*criapi.ListContainersRequest).Filter; f != nil {
		if f.Id != "" || f.State != nil || f.PodSandboxId != "" || len(f.LabelSelector) > 0 {
			return reply, nil
		}
	}

	m.Lock()
	defer m.Unlock()

	clistmap := map[string]*criapi.Container{}
	released := []cache.Container{}
	for _, listed := range reply.(*criapi.ListContainersResponse).Containers {
		clistmap[listed.Id] = listed
		if listed.State != criapi.ContainerState_CONTAINER_EXITED {
			continue
		}
		if c, ok := m.cache.LookupContainer(listed.Id); ok {
			state := c.GetState()
			if state == cache.ContainerStateRunning || state == cache.ContainerStateCreated {
				m.Info("%s: exited, releasing its resources...", c.PrettyName())
				if err := m.policy.ReleaseResources(c); err != nil {
					m.Error("%s: failed to release resources for container %s: %v",
						method, c.PrettyName(), err)
				}
				c.UpdateState(cache.ContainerStateExited)
				released = append(released, c)
			}
		}
	}

	for _, c := range m.cache.GetContainers() {
		if c.GetState() == cache.ContainerStateRunning {
			if _, ok := clistmap[c.GetID()]; !ok {
				m.Info("%s: absent from runtime, releasing its resources...", c.PrettyName())
				if err := m.policy.ReleaseResources(c); err != nil {
					m.Error("%s: failed to release resources for container %s: %v",
						method, c.PrettyName(), err)
				}
				c.UpdateState(cache.ContainerStateStale)
				released = append(released, c)
			}
		}
	}

	if len(released) > 0 {
		if err := m.runPostReleaseHooks(ctx, method, released...); err != nil {
			m.Error("%s: failed to run post-release hooks: %v",
				method, err)
		}
	}
	m.updateIntrospection()

	return reply, nil
}

// UpdateContainer intercepts CRI requests for updating Containers.
func (m *resmgr) UpdateContainer(ctx context.Context, method string, request interface{},
	handler server.Handler) (interface{}, error) {

	m.Lock()
	defer m.Unlock()

	containerID := request.(*criapi.UpdateContainerResourcesRequest).ContainerId
	container, ok := m.cache.LookupContainer(containerID)

	if !ok {
		m.Warn("%s: silently dropping container update request for %s...",
			method, containerID)
	} else {
		m.Warn("%s: silently dropping container update request for %s...",
			method, container.PrettyName())
		m.Warn("%s: XXX TODO: we probably should reallocate the container instead...",
			method)
	}

	m.updateIntrospection()

	return &criapi.UpdateContainerResourcesResponse{}, nil
}

// RebalanceContainers tries to find a more optimal container resource allocation if necessary.
func (m *resmgr) RebalanceContainers() error {
	m.Lock()
	defer m.Unlock()

	m.Info("rebalancing (reallocating) containers...")

	return m.rebalance("Rebalance")
}

// rebalance triggers a policy-specific rebalancing cycle of containers.
func (m *resmgr) rebalance(method string) error {
	if m.policy == nil {
		return nil
	}

	changes, err := m.policy.Rebalance()

	if err != nil {
		m.Error("%s: rebalancing of containers failed: %v", method, err)
	}

	if changes {
		if err := m.runPostUpdateHooks(context.Background(), method); err != nil {
			m.Error("%s: failed to run post-update hooks: %v", method, err)
			return resmgrError("%s: failed to run post-update hooks: %v", method, err)
		}
	}

	return m.cache.Save()
}

// DeliverPolicyEvent delivers a policy-specific event to the active policy.
func (m *resmgr) DeliverPolicyEvent(e *events.Policy) error {
	m.Lock()
	defer m.Unlock()

	if e.Source == "" {
		e.Source = "unspecified"
	}

	m.Info("delivering policy event %s.%s...", e.Source, e.Type)

	method := "DeliverPolicyEvent"
	changes, err := m.policy.HandleEvent(e)

	if err != nil {
		m.Error("%s: handling event %s.%s failed: %v", method, e.Source, e.Type, err)
		return err
	}

	if changes {
		if err = m.runPostUpdateHooks(context.Background(), method); err != nil {
			m.Error("%s: failed to run post-update hooks: %v", method, err)
			return resmgrError("%s: failed to run post-update hooks: %v", method, err)
		}
	}

	m.cache.Save()
	return nil
}

// setConfig activates a new configuration, either from the agent or from a file.
func (m *resmgr) setConfig(v interface{}) error {
	var err error

	m.Lock()
	defer m.Unlock()

	switch cfg := v.(type) {
	case *config.RawConfig:
		err = pkgcfg.SetConfig(cfg.Data)
	case string:
		err = pkgcfg.SetConfigFromFile(cfg)
	default:
		err = fmt.Errorf("invalid configuration source/type %T", v)
	}
	if err != nil {
		m.Error("configuration rejected: %v", err)
		return resmgrError("configuration rejected: %v", err)
	}

	// synchronize state of controllers with new configuration
	if err = m.control.StartStopControllers(m.cache, m.relay.Client()); err != nil {
		m.Error("failed to synchronize controllers with new configuration: %v", err)
		return resmgrError("failed to synchronize controllers with new configuration: %v", err)
	}

	if err = m.runPostUpdateHooks(context.Background(), "setConfig"); err != nil {
		m.Error("failed to run post-update hooks after reconfiguration: %v", err)
		return resmgrError("failed to run post-update hooks after reconfiguration: %v", err)
	}

	// if we managed to activate a configuration from the agent, store it in the cache
	if cfg, ok := v.(*config.RawConfig); ok {
		m.cache.SetConfig(cfg)
	}

	m.Info("successfully switched to new configuration")

	return nil
}

// runPostAllocateHooks runs the necessary hooks after allocating resources for some containers.
func (m *resmgr) runPostAllocateHooks(ctx context.Context, method string) error {
	for _, c := range m.cache.GetPendingContainers() {
		switch c.GetState() {
		case cache.ContainerStateRunning, cache.ContainerStateCreated:
			if err := m.control.RunPostUpdateHooks(c); err != nil {
				m.Warn("%s post-update hook failed for %s: %v",
					method, c.PrettyName(), err)
			}
			if req, ok := c.ClearCRIRequest(); ok {
				if _, err := m.sendCRIRequest(ctx, req); err != nil {
					m.Warn("%s update of container %s failed: %v",
						method, c.PrettyName(), err)
				}
			}
			m.policy.ExportResourceData(c)
		case cache.ContainerStateCreating:
			if err := m.control.RunPreCreateHooks(c); err != nil {
				m.Warn("%s pre-create hook failed for %s: %v",
					method, c.PrettyName(), err)
			}
			m.policy.ExportResourceData(c)
		default:
			m.Warn("%s: skipping container %s (in state %v)", method,
				c.PrettyName(), c.GetState())
		}
	}
	return nil
}

// runPostStartHooks runs the necessary hooks after having started a container.
func (m *resmgr) runPostStartHooks(ctx context.Context, method string, c cache.Container) error {
	if err := m.control.RunPostStartHooks(c); err != nil {
		m.Error("%s: post-start hook failed for %s: %v", method, c.PrettyName(), err)
	}
	return nil
}

// runPostReleaseHooks runs the necessary hooks after releaseing resources of some containers
func (m *resmgr) runPostReleaseHooks(ctx context.Context, method string, del ...cache.Container) error {
	for _, c := range del {
		if err := m.control.RunPostStopHooks(c); err != nil {
			m.Warn("post-stop hook failed for %s: %v", c.PrettyName(), err)
		}
	}
	for _, c := range m.cache.GetPendingContainers() {
		switch c.GetState() {
		case cache.ContainerStateStale, cache.ContainerStateExited:
			if err := m.control.RunPostStopHooks(c); err != nil {
				m.Warn("post-stop hook failed for %s: %v", c.PrettyName(), err)
			}
			m.cache.DeleteContainer(c.GetCacheID())
		case cache.ContainerStateRunning, cache.ContainerStateCreated:
			if err := m.control.RunPostUpdateHooks(c); err != nil {
				m.Warn("post-update hook failed for %s: %v", c.PrettyName(), err)
			}
			if req, ok := c.ClearCRIRequest(); ok {
				if _, err := m.sendCRIRequest(ctx, req); err != nil {
					m.Warn("update of container %s failed: %v", c.PrettyName(), err)
				}
			}
			m.policy.ExportResourceData(c)
		default:
			m.Warn("%s: skipping pending container %s (in state %v)",
				method, c.PrettyName(), c.GetState())
		}
	}
	return nil
}

// runPostUpdateHooks runs the necessary hooks after reconcilation.
func (m *resmgr) runPostUpdateHooks(ctx context.Context, method string) error {
	for _, c := range m.cache.GetPendingContainers() {
		switch c.GetState() {
		case cache.ContainerStateRunning, cache.ContainerStateCreated:
			if err := m.control.RunPostUpdateHooks(c); err != nil {
				return err
			}
			if req, ok := c.GetCRIRequest(); ok {
				if _, err := m.sendCRIRequest(ctx, req); err != nil {
					m.Warn("%s update of container %s failed: %v",
						method, c.PrettyName(), err)
				} else {
					c.ClearCRIRequest()
				}
			}
			m.policy.ExportResourceData(c)
		default:
			m.Warn("%s: skipping container %s (in state %v)", method,
				c.PrettyName(), c.GetState())
		}
	}
	return nil
}

// sendCRIRequest sends the given CRI request, returning the received reply and error.
func (m *resmgr) sendCRIRequest(ctx context.Context, request interface{}) (interface{}, error) {
	client := m.relay.Client()
	switch request.(type) {
	case *criapi.UpdateContainerResourcesRequest:
		req := request.(*criapi.UpdateContainerResourcesRequest)
		m.Debug("sending update request for container %s...", req.ContainerId)
		return client.UpdateContainerResources(ctx, req)
	default:
		return nil, resmgrError("sendCRIRequest: unhandled request type %T", request)
	}
}
