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

package topologyaware

import (
	"testing"

	system "github.com/intel/cri-resource-manager/pkg/sysfs"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
)

func TestBuildPoolsByTopology(t *testing.T) {
	tcases := []struct {
		name             string
		policy           *policy
		expectedError    bool
		expectedRootCPUs string
	}{
		{
			name: "empty",
			policy: &policy{
				sys: &mockSystem{
					packageIDs: []system.ID{0, 1},
					nodeIDs:    []system.ID{0, 1},
					nodes: map[system.ID]*system.Node{
						0: {
							Cpus: system.NewIDSet(0),
						},
						1: {
							Cpus: system.NewIDSet(1),
						},
					},
					pkgs: map[system.ID]*system.Package{
						0: {
							Cpus: system.NewIDSet(0),
						},
						1: {
							Cpus: system.NewIDSet(1),
						},
					},
				},
			},
			expectedRootCPUs: "<root CPU: sharable:0-1 (granted:0, free: 2000)>",
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.policy.buildPoolsByTopology()
			if tc.expectedError && err == nil {
				t.Errorf("Expected error, but got success")
			}
			if !tc.expectedError && err != nil {
				t.Errorf("Unxpected error: %+v", err)
			}
			if tc.expectedRootCPUs != tc.policy.root.freecpu.String() {
				t.Errorf("Expected %q granted to root node, but got %q", tc.expectedRootCPUs, tc.policy.root.freecpu)
			}
		})
	}
}

func TestApplyGrant(t *testing.T) {
	tcases := []struct {
		name          string
		policy        *policy
		grant         *cpuGrant
		expectedError bool
	}{
		{
			name: "with exclusive in the grant",
			policy: &policy{
				sys: &mockSystem{},
			},
			grant: &cpuGrant{
				node: &node{
					nodecpu: newCPUSupply(&node{}, cpuset.NewCPUSet(), cpuset.NewCPUSet(), 0),
					parent:  &node{},
					mem:     system.NewIDSet(0),
				},
				container: &mockContainer{},
				exclusive: cpuset.NewCPUSet(5),
			},
			expectedError: false,
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.policy.applyGrant(tc.grant)
			if tc.expectedError && err == nil {
				t.Errorf("Expected error, but got success")
			}
			if !tc.expectedError && err != nil {
				t.Errorf("Unxpected error: %+v", err)
			}
		})
	}
}

func TestUpdateSharedAllocations(t *testing.T) {
	tcases := []struct {
		name          string
		policy        *policy
		grant         *cpuGrant
		expectedError bool
	}{
		{
			name: "no error",
			policy: &policy{
				allocations: allocations{
					CPU: map[string]CPUGrant{
						"fakegrant": &cpuGrant{
							portion: 1,
							node: &node{
								freecpu: newCPUSupply(nil, cpuset.NewCPUSet(), cpuset.NewCPUSet(), 0),
								nodecpu: newCPUSupply(nil, cpuset.NewCPUSet(), cpuset.NewCPUSet(), 0),
							},
							container: &mockContainer{},
						},
					},
				},
			},
			grant: &cpuGrant{
				node: &node{
					nodecpu: newCPUSupply(&node{}, cpuset.NewCPUSet(), cpuset.NewCPUSet(), 0),
				},
				container: &mockContainer{},
			},
		},
		// TODO(rojkov): verify actual results
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.policy.updateSharedAllocations(tc.grant)
			if tc.expectedError && err == nil {
				t.Errorf("Expected error, but got success")
			}
			if !tc.expectedError && err != nil {
				t.Errorf("Unxpected error: %+v", err)
			}
		})
	}
}

func TestCompareScores(t *testing.T) {
	tcases := []struct {
		name     string
		policy   *policy
		request  *cpuRequest
		scores   map[int]CPUScore
		affinity map[int]int32
		i        int
		j        int
		expected bool
	}{
		{
			name: "fake case for lower id",
			policy: &policy{
				pools: []*node{
					{
						id: 1,
					},
				},
			},
			scores: map[int]CPUScore{
				1: &cpuScore{},
			},
			request: &cpuRequest{},
		},
		{
			name: "lower id wins",
			policy: &policy{
				pools: []*node{
					{
						id: 1,
					},
					{
						id: 2,
					},
				},
			},
			scores: map[int]CPUScore{
				1: &cpuScore{},
				2: &cpuScore{},
			},
			request:  &cpuRequest{},
			i:        0,
			j:        1,
			expected: true,
		},
		{
			name: "better topology hint score wins",
			policy: &policy{
				pools: []*node{
					{
						id: 1,
					},
					{
						id: 2,
					},
				},
			},
			scores: map[int]CPUScore{
				1: &cpuScore{
					hints: map[string]float64{
						"test":  0.999,
						"test2": 0.0999,
					},
				},
				2: &cpuScore{},
			},
			request:  &cpuRequest{},
			i:        0,
			j:        1,
			expected: true,
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			actual := tc.policy.compareScores(tc.request, tc.scores, tc.affinity, tc.i, tc.j)
			if actual != tc.expected {
				t.Errorf("Expected %v, but got %v", tc.expected, actual)
			}
		})
	}
}
