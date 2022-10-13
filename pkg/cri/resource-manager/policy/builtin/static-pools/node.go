/*
Copyright 2019 Intel Corporation

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package stp

import (
	"strconv"
	"time"

	core_v1 "k8s.io/api/core/v1"

	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/agent"
	logger "github.com/intel/cri-resource-manager/pkg/log"
)

const (
	exclusiveCoreResourceName = "cmk.intel.com/exclusive-cores"
	cmkLegacyNodeLabelName    = "cmk.intel.com/cmk-node"
)

type nodeUpdater struct {
	logger.Logger
	agent agent.Interface
	conf  chan config
}

func newNodeUpdater(agent agent.Interface) *nodeUpdater {
	return &nodeUpdater{
		Logger: logger.NewLogger("static-pools-nu"),
		agent:  agent,
		conf:   make(chan config, 1),
	}
}

func (u *nodeUpdater) start() error {
	u.Info("starting node updater")

	if u.agent == nil || u.agent.IsDisabled() {
		return stpError("cri-resmgr-agent connection required")
	}

	go func() {
		var pending *config
		var retry <-chan time.Time

		for {
			select {
			case c := <-u.conf:
				pending = &c
				retry = time.After(0)
			case _ = <-retry:
				if pending != nil {
					err := u.updateNode(pending, -1)
					if err != nil {
						u.Info("node update failed: %v", err)
						retry = time.After(5 * time.Second)
					} else {
						u.Info("node successfully updated")
						pending = nil
						retry = nil
					}
				} else {
					u.Panic("BUG: node update with nil config requested")
				}
			}
		}
	}()

	return nil
}

func (u *nodeUpdater) update(c config) {
	// Pop possibly pending value from the channel
	select {
	case <-u.conf:
	default:
	}
	u.conf <- c
}

// Update Node object with STP/CMK-specific things
func (u *nodeUpdater) updateNode(conf *config, opTimeout time.Duration) error {
	// Count total number of cpu lists of all exclusive pools
	numExclusiveCPULists := 0
	for _, pool := range conf.Pools {
		if pool.Exclusive {
			numExclusiveCPULists += len(pool.CPULists)
		}
	}

	// Update extended resources
	resources := map[string]string{
		exclusiveCoreResourceName: strconv.Itoa(numExclusiveCPULists)}
	u.Info("updating node capacity (extended resources)")
	if err := u.agent.UpdateNodeCapacity(resources, opTimeout); err != nil {
		return err
	}

	// Manage legacy node label
	if conf.LabelNode {
		u.Info("creating CMK node label")
		err := u.agent.SetLabels(map[string]string{cmkLegacyNodeLabelName: "true"}, opTimeout)
		if err != nil {
			return stpError("failed to update legacy node label: %v", err)
		}
	} else {
		u.Info("removing CMK node label")
		err := u.agent.RemoveLabels([]string{cmkLegacyNodeLabelName}, opTimeout)
		if err != nil {
			return stpError("failed to update legacy node label: %v", err)
		}
	}

	// Manage legacy node taint
	nodeTaints, err := u.agent.GetTaints(opTimeout)
	if err != nil {
		return stpError("failed to fetch node taints: %v", err)
	}

	legacyTaint := core_v1.Taint{
		Key:    "cmk",
		Value:  "true",
		Effect: core_v1.TaintEffectNoSchedule,
	}
	cmkTaints := []core_v1.Taint{legacyTaint}
	_, tainted := u.agent.FindTaintIndex(nodeTaints, &legacyTaint)

	if !tainted && conf.TaintNode {
		u.Info("creating CMK node taint")
		if err := u.agent.SetTaints(cmkTaints, opTimeout); err != nil {
			return stpError("failed to set legacy node taint: %v", err)
		}
	}
	if tainted && !conf.TaintNode {
		u.Debug("removing CMK node taint")
		if err := u.agent.RemoveTaints(cmkTaints, opTimeout); err != nil {
			return stpError("failed to clear legacy node taint: %v", err)
		}
	}

	return nil
}
