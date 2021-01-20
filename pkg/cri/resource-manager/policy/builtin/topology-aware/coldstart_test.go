// Copyright 2020 Intel Corporation. All Rights Reserved.
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

package topologyaware

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/events"
	policyapi "github.com/intel/cri-resource-manager/pkg/cri/resource-manager/policy"
	system "github.com/intel/cri-resource-manager/pkg/sysfs"
)

var globalPolicy *policy
var mutex sync.Mutex

func sendEvent(param interface{}) error {
	// Simulate event synchronization in the upper levels.
	mutex.Lock()
	defer mutex.Unlock()

	fmt.Printf("Event received: %v", param)
	event := param.(*events.Policy)
	globalPolicy.HandleEvent(event)
	return nil
}

func TestColdStart(t *testing.T) {

	// Idea with cold start is that the workload is first allocated only PMEM node. Only when timer expires
	// (or some other event is triggered) is the DRAM node added to the memset. This causes the initial
	// memory allocations to be made from PMEM only.

	tcases := []struct {
		name                     string
		numaNodes                []system.Node
		req                      Request
		affinities               map[int]int32
		container                cache.Container
		expectedColdStartTimeout time.Duration
		expectedDRAMNodeID       int
		expectedPMEMNodeID       int
		expectedDRAMSystemNodeID system.ID
		expectedPMEMSystemNodeID system.ID
	}{
		{
			name: "three node cold start",
			numaNodes: []system.Node{
				&mockSystemNode{id: 1, memFree: 10000, memTotal: 10000, memType: system.MemoryTypeDRAM, distance: []int{5, 5, 1}},
				&mockSystemNode{id: 2, memFree: 50000, memTotal: 50000, memType: system.MemoryTypePMEM, distance: []int{5, 1, 5}},
			},
			container: &mockContainer{
				name:                     "demo-coldstart-container",
				returnValueForGetCacheID: "1234",
				pod: &mockPod{
					coldStartTimeout:                   1000 * time.Millisecond,
					returnValue1FotGetResmgrAnnotation: "demo-coldstart-container: pmem,dram",
					returnValue2FotGetResmgrAnnotation: true,
					coldStartContainerName:             "demo-coldstart-container",
				},
			},
			expectedColdStartTimeout: 1000 * time.Millisecond,
			expectedDRAMNodeID:       101,
			expectedDRAMSystemNodeID: system.ID(1),
			expectedPMEMSystemNodeID: system.ID(2),
			expectedPMEMNodeID:       102,
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			policy := &policy{
				sys: &mockSystem{
					nodes: tc.numaNodes,
				},
				cache: &mockCache{
					returnValue1ForLookupContainer: tc.container,
					returnValue2ForLookupContainer: true,
				},
				allocations: allocations{
					grants: make(map[string]Grant, 0),
				},
				options: &policyapi.BackendOptions{},
			}
			policy.allocations.policy = policy
			policy.options.SendEvent = sendEvent

			if err := policy.buildPoolsByTopology(); err != nil {
				t.Errorf("failed to build topology pool")
			}

			grant, err := policy.allocatePool(tc.container, "")
			if err != nil {
				panic(err)
			}
			if grant.ColdStart() != tc.expectedColdStartTimeout {
				t.Errorf("Expected coldstart value '%v', but got '%v'", tc.expectedColdStartTimeout, grant.ColdStart())
			}

			policy.allocations.grants[tc.container.GetCacheID()] = grant

			mems := grant.Memset()
			if len(mems) != 1 || mems.Members()[0] != tc.expectedPMEMSystemNodeID {
				t.Errorf("Expected one memory controller %v, got: %v", tc.expectedPMEMSystemNodeID, mems)
			}

			if grant.MemoryType()&memoryDRAM != 0 {
				// FIXME: should we report only the limited memory types or the granted types
				// while the cold start is going on?
				// t.Errorf("No DRAM was expected before coldstart timer: %v", grant.MemoryType())
			}

			globalPolicy = policy

			policy.options.SendEvent(&events.Policy{
				Type: events.ContainerStarted,
				Data: tc.container,
			})

			time.Sleep(tc.expectedColdStartTimeout * 2)

			newMems := grant.Memset()
			if len(newMems) != 2 {
				t.Errorf("Expected two memory controllers, got %d: %v", len(newMems), newMems)
			}
			if !newMems.Has(tc.expectedPMEMSystemNodeID) || !newMems.Has(tc.expectedDRAMSystemNodeID) {
				t.Errorf("Didn't get all expected system nodes in mems, got: %v", newMems)
			}
		})
	}
}
