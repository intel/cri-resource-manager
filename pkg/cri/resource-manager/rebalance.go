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
	corev1 "k8s.io/api/core/v1"

	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
)

// Rebalance rebalances containers.
func (m *resmgr) Rebalance() error {
	m.Lock()
	defer m.Unlock()

	disruptible := []cache.Container{}
	for _, c := range m.cache.GetContainers() {
		if c.GetQOSClass() != corev1.PodQOSGuaranteed {
			disruptible = append(disruptible, c)
		}
	}
	if len(disruptible) == 0 {
		return nil
	}

	m.Info("rebalancing (reallocating) containers...")

	method := "rebalance"
	if err := m.policy.ReallocateResources(disruptible); err != nil {
		m.Error("%s: failed to rebalance containers: %v", method, err)
		return resmgrError("%s: failed to rebalance containers: %v", method, err)
	}

	if err := m.runPostUpdateHooks(context.Background(), method); err != nil {
		m.Error("%s: failed to run post-update hooks: %v", method, err)
		return resmgrError("%s: failed to run post-update hooks: %v", method, err)
	}

	m.cache.Save()
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
