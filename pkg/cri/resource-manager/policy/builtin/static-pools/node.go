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

	core_v1 "k8s.io/api/core/v1"
)

const (
	exclusiveCoreResourceName = "cmk.intel.com/exclusive-cores"
	cmkLegacyNodeLabelName    = "cmk.intel.com/cmk-node"
)

// Update Node object with STP/CMK-specific things
func (stp *stp) updateNode(conf conf) error {
	// We require an agent connection
	if stp.agent == nil {
		return stpError("stp requires cri-resource-manageent-agent connection")
	}

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
	if err := stp.agent.UpdateNodeCapacity(resources, -1); err != nil {
		return err
	}

	// Manage legacy node label
	if conf.LabelNode {
		stp.Info("creating CMK node label")
		err := stp.agent.SetLabels(map[string]string{cmkLegacyNodeLabelName: "true"}, -1)
		if err != nil {
			return stpError("failed to update legacy node label: %v", err)
		}
	} else {
		err := stp.agent.RemoveLabels([]string{cmkLegacyNodeLabelName}, -1)
		if err != nil {
			return stpError("failed to update legacy node label: %v", err)
		}
	}

	// Manage legacy node taint
	nodeTaints, err := stp.agent.GetTaints(-1)
	if err != nil {
		return stpError("failed to fetch node taints: %v", err)
	}

	legacyTaint := core_v1.Taint{
		Key:    "cmk",
		Value:  "true",
		Effect: core_v1.TaintEffectNoSchedule,
	}
	cmkTaints := []core_v1.Taint{legacyTaint}
	_, tainted := stp.agent.FindTaintIndex(nodeTaints, &legacyTaint)

	if !tainted && conf.TaintNode {
		if err := stp.agent.SetTaints(cmkTaints, -1); err != nil {
			return stpError("failed to set legacy node taint: %v", err)
		}
	}
	if tainted && !conf.TaintNode {
		if err := stp.agent.RemoveTaints(cmkTaints, -1); err != nil {
			return stpError("failed to clear legacy node taint: %v", err)
		}
	}

	return nil
}
