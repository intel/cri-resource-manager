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

	api "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"

	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/policy"
	"github.com/intel/cri-resource-manager/pkg/cri/server"
)

// setupPolicyHooks sets up relay hooks for CRI resource manager policies.
func (m *resmgr) setupPolicyHooks() error {
	if policy.ActivePolicy() == policy.NullPolicy {
		return nil
	}

	hooks := map[string]server.Interceptor{
		// these requests are transparently relayed with an update of the local cache
		"RunPodSandbox": m.relayAndUpdateCache,

		// these requests trigger local policy decisions and might get modified
		"CreateContainer":  m.processWithPolicy,
		"StartContainer":   m.processWithPolicy,
		"RemoveContainer":  m.processWithPolicy,
		"RemovePodSandbox": m.processWithPolicy,

		// these requests are filtered with a synthetic response
		"UpdateContainerResources": m.filterWithSyntheticResponse,
	}

	if err := m.relay.Server().RegisterInterceptors(hooks); err != nil {
		return resmgrError("failed to register policy hooks: %v", err)
	}

	return nil
}

// syncCache synchronizes the cache with the container runtime.
func (m *resmgr) syncCache() ([]cache.Container, []cache.Container, error) {
	if !m.relay.Client().HasRuntimeService() {
		return nil, nil, nil
	}

	ctx := context.Background()

	pl, err := m.relay.Client().ListPodSandbox(ctx, &api.ListPodSandboxRequest{})
	if err != nil {
		return nil, nil, resmgrError("cache synchronization pod query failed: %v", err)
	}
	_, _, add, del := m.cache.Refresh(pl)

	cl, err := m.relay.Client().ListContainers(ctx, &api.ListContainersRequest{})
	if err != nil {
		return nil, nil, resmgrError("cache synchronization container query failed: %v", err)
	}
	_, _, a, d := m.cache.Refresh(cl)

	add = append(add, a...)
	del = append(del, d...)

	return add, del, nil
}

// startPolicy prepares resource manager for making policy decisions for requests.
func (m *resmgr) startPolicy() error {
	added, deleted, err := m.syncCache()
	if err != nil {
		return resmgrError("failed to synchronize cache to start policy: %v", err)
	}

	if policy.ActivePolicy() == policy.NullPolicy {
		return nil
	}

	add := make([]cache.Container, 0, len(added))
	for _, c := range deleted {
		m.Info("discovered stale container %s...", c.GetID())
	}
	for _, c := range added {
		if c.GetState() != cache.ContainerStateRunning {
			continue
		}
		m.Info("discovered out-of-sync running container %s...", c.GetID())
		add = append(add, c)
	}

	if err := m.policy.Start(m.cache, add, deleted); err != nil {
		return resmgrError("failed to start policy: %v", err)
	}

	m.enforcePendingDecisions(context.Background())
	m.policy.CommitDecisions()

	return nil
}

// filterWithSyntheticResponse filters the request and replies with a synthetic response.
func (m *resmgr) filterWithSyntheticResponse(ctx context.Context, method string, req interface{},
	handler server.Handler) (interface{}, error) {
	m.Lock()
	defer m.Unlock()

	switch req.(type) {
	case *api.UpdateContainerResourcesRequest:
		// TODO: we need to update container resources and TopologyHints in our cache (VPA)
		return &api.UpdateContainerResourcesResponse{}, nil
	}

	return handler(ctx, req)
}

// relayAndUpdateCache relays the request after the local cache has been updated.
func (m *resmgr) relayAndUpdateCache(ctx context.Context, method string, req interface{},
	handler server.Handler) (interface{}, error) {
	m.Lock()
	defer m.Unlock()

	rpl, err := handler(ctx, req)
	if err != nil {
		return nil, err
	}

	switch req.(type) {
	case *api.RunPodSandboxRequest:
		m.cache.InsertPod(rpl.(*api.RunPodSandboxResponse).PodSandboxId, req)
	}

	return rpl, nil
}

// processWithPolicy applies local policy decisions to the request before relaying it.
func (m *resmgr) processWithPolicy(ctx context.Context, method string, req interface{},
	handler server.Handler) (interface{}, error) {
	m.Lock()
	defer m.Unlock()

	switch req.(type) {
	case *api.CreateContainerRequest:
		//
		// When creating a new container
		//
		//   - create a policy transaction
		//   - insert the container into the cache
		//   - allocate resources for the container
		//   - enforce potential pending decisions on affected containers
		//   - create container (relay creation request)
		//   - commit the decisions/transaction
		//
		// If anything goes wrong we abort the transaction instead.
		//
		m.policy.PrepareDecisions()
		c := m.cache.InsertContainer(req)

		err := m.policy.AllocateResources(c)
		if err != nil {
			m.policy.AbortDecisions()
			return nil, err
		}

		updates := m.policy.QueryDecisions()
		m.enforcePendingDecisions(ctx, updates...)

		rpl, err := m.createContainer(ctx, handler, req.(*api.CreateContainerRequest), c)
		if err != nil {
			m.policy.AbortDecisions()
			// m.enforcePendingDecisions(ctx, updates...) // undo previous changes
			return nil, err
		}
		m.policy.CommitDecisions()

		return rpl, err

	case *api.RemoveContainerRequest:
		//
		// When removing a container
		//
		//   - look up the container
		//   - delete it (relay removal request)
		//   - create policy transaction
		//   - release resources allocated to the container
		//   - enforce potential pending decisions on affected containers
		//   - commit the decisions/transaction
		//
		// Note that we don't abort and never try to prevent the removal of the
		// container even if something goes wrong.
		//

		c, ok := m.cache.LookupContainer(req.(*api.RemoveContainerRequest).ContainerId)
		if !ok {
			return handler(ctx, req)
		}
		m.cache.DeleteContainer(c.GetCacheID())

		m.policy.PrepareDecisions()
		if err := m.policy.ReleaseResources(c); err != nil {
			m.Warn("failed to release resources for container %s: %v", c.GetID(), err)
		}

		rpl, err := handler(ctx, req)
		m.enforcePendingDecisions(ctx)

		m.policy.CommitDecisions()

		return rpl, err

	case *api.RemovePodSandboxRequest:
		//
		// When removing a pod
		//
		//   - remove the pod (relay removal request)
		//   - create policy transaction
		//   - release resources for any remaining containers of the pod
		//   - enforce potential pending decision on affected containers
		//   - commit the decision
		//
		// Note that we don't abort and never try to prevent the removal of the
		// pod even if something goes wrong.
		//

		rpl, err := handler(ctx, req)
		m.cache.DeletePod(req.(*api.RemovePodSandboxRequest).PodSandboxId)

		m.Info("cleaning up pod %s", req.(*api.RemovePodSandboxRequest).PodSandboxId)

		m.policy.PrepareDecisions()
		for _, id := range m.cache.GetContainerIds() {
			c, ok := m.cache.LookupContainer(id)
			if !ok || c.GetPodID() != req.(*api.RemovePodSandboxRequest).PodSandboxId {
				continue
			}

			m.Info("removing container %s", c.GetPodID())
			m.cache.DeleteContainer(c.GetCacheID())
			if e := m.policy.ReleaseResources(c); e != nil {
				m.Warn("failed to release resources for container %s: %v", c.GetID(), e)
			}
		}
		m.enforcePendingDecisions(ctx)

		m.policy.CommitDecisions()
		return rpl, err

	case *api.StartContainerRequest:
		// find the container, relaying the request verbatim, if we can't find it
		c, ok := m.cache.LookupContainer(req.(*api.StartContainerRequest).ContainerId)
		if !ok {
			return handler(ctx, req)
		}

		rpl, err := handler(ctx, req)

		m.Info("StartContainerRequest response %v", rpl)

		if err == nil {
			c.UpdateState(cache.ContainerStateRunning)
		}

		policyErr := m.policy.PostStart(c)
		if policyErr != nil {
			m.Warn("failed to update affected container %s: %v", c.GetID(), policyErr)
		}
		return rpl, err

	default:
		return handler(ctx, req)
	}
}

// Enforce pending policy decisions.
func (m *resmgr) enforcePendingDecisions(ctx context.Context, updates ...cache.Container) error {
	if len(updates) == 0 {
		updates = m.policy.QueryDecisions()
	}

	for _, c := range updates {
		if c.GetState() == cache.ContainerStateCreating {
			continue
		}

		err := m.updateContainer(ctx, c)
		if err != nil {
			m.Warn("failed to update container %s during startup: %v", c.GetID(), err)
		}
	}

	return nil
}

// Forward a(n updated) container create request, followed by an update of resources.
func (m *resmgr) createContainer(ctx context.Context, handler server.Handler,
	req *api.CreateContainerRequest, c cache.Container) (*api.CreateContainerResponse, error) {

	c.InsertMount(&cache.Mount{
		Container:   "/.cri-resmgr",
		Host:        m.cache.ContainerDirectory(c.GetCacheID()),
		Readonly:    true,
		Propagation: cache.MountHostToContainer,
	})

	if err := c.UpdateCriCreateRequest(req); err != nil {
		return nil, err
	}

	rpl, err := handler(ctx, req)
	if err != nil {
		return nil, err
	}
	m.cache.UpdateContainerID(c.GetCacheID(), rpl)

	upd, err := c.CriUpdateRequest()
	if err != nil {
		return nil, resmgrError("failed to update container %s: %v", upd.ContainerId, err)
	}

	_, err = m.relay.Client().UpdateContainerResources(ctx, upd)
	if err != nil {
		m.Warn("failed to update container %s: %v", upd.ContainerId, err)
	}

	return rpl.(*api.CreateContainerResponse), nil
}

// Send a synthetic UpdateContainerResources request to the runtime service.
func (m *resmgr) updateContainer(ctx context.Context, c cache.Container) error {
	switch c.GetState() {
	case cache.ContainerStateExited, cache.ContainerStateStale:
		return nil
	}

	req, err := c.CriUpdateRequest()
	if err != nil {
		return resmgrError("can't update container %s: %v", c.PrettyName(), err)
	}

	_, err = m.relay.Client().UpdateContainerResources(ctx, req)

	return err
}
