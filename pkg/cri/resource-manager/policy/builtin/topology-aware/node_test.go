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

// FIXME: how to enable logging programmatically to test dump() properly?
func TestDump(t *testing.T) {
	tcases := []struct {
		name    string
		getTree func() *node
	}{
		{
			name: "one child",
			getTree: func() *node {
				policy := &policy{
					allocations: allocations{
						CPU: make(map[string]CPUGrant),
					},
					sys: &mockSystem{},
				}
				root := newVirtualNode("root", policy, nil)
				_ = newSocketNode(0, policy, root)
				return root
			},
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			tc.getTree().dump("test", 0)
		})
	}
}

func TestGrantedCPU(t *testing.T) {
	tcases := []struct {
		name     string
		getTree  func() *node
		expected int
	}{
		{
			name: "one child",
			getTree: func() *node {
				policy := &policy{
					allocations: allocations{
						CPU: make(map[string]CPUGrant),
					},
					sys: &mockSystem{},
				}
				root := newVirtualNode("root", policy, nil)
				socket := newSocketNode(0, policy, root)
				socket.freecpu = newCPUSupply(socket, cpuset.NewCPUSet(), cpuset.NewCPUSet(), 2)
				root.freecpu = newCPUSupply(root, cpuset.NewCPUSet(), cpuset.NewCPUSet(), 999)
				return root
			},
			expected: 1001,
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			actual := tc.getTree().grantedCPU()
			if actual != tc.expected {
				t.Errorf("Expected %d, but got %d", tc.expected, actual)
			}
		})
	}
}

func TestRootDistance(t *testing.T) {
	tcases := []struct {
		name             string
		getNode          func() *node
		expectedDistance int
	}{
		{
			name: "root only",
			getNode: func() *node {
				return newVirtualNode("root", &policy{}, nil)
			},
		},
		{
			name: "one child",
			getNode: func() *node {
				policy := &policy{
					allocations: allocations{
						CPU: make(map[string]CPUGrant),
					},
					sys: &mockSystem{},
				}
				root := newVirtualNode("root", policy, nil)
				socket := newSocketNode(0, policy, root)
				return socket
			},
			expectedDistance: 1,
		},
		{
			name: "two children",
			getNode: func() *node {
				policy := &policy{
					allocations: allocations{
						CPU: make(map[string]CPUGrant),
					},
					sys: &mockSystem{},
				}
				root := newVirtualNode("root", policy, nil)
				socket := newSocketNode(0, policy, root)
				numa := newNumaNode(0, policy, socket)
				return numa
			},
			expectedDistance: 2,
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			actual := tc.getNode().rootDistance()
			if actual != tc.expectedDistance {
				t.Errorf("Expected %d, but got %d", tc.expectedDistance, actual)
			}
		})
	}
}

func TestDiscoverCPU(t *testing.T) {
	tcases := []struct {
		name     string
		getTree  func() *node
		expected string
	}{
		{
			name: "one empty child",
			getTree: func() *node {
				policy := &policy{
					allocations: allocations{
						CPU: make(map[string]CPUGrant),
					},
					sys: &mockSystem{},
				}
				root := newVirtualNode("root", policy, nil)
				_ = newSocketNode(0, policy, root)
				return root
			},
			expected: "<root CPU: ->",
		},
		{
			name: "one non-empty child",
			getTree: func() *node {
				policy := &policy{
					allocations: allocations{
						CPU: make(map[string]CPUGrant),
					},
					sys: &mockSystem{},
				}
				root := newVirtualNode("root", policy, nil)
				socket := newSocketNode(0, policy, root)
				socket.attachedCPUSet = func() cpuset.CPUSet {
					return cpuset.NewCPUSet(0, 1)
				}
				return root
			},
			expected: "<root CPU: sharable:0-1 (granted:0, free: 2000)>",
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			actual := tc.getTree().discoverCPU().String()
			if actual != tc.expected {
				t.Errorf("Expected %s, but got %s", tc.expected, actual)
			}
		})
	}
}

func TestGetMemset(t *testing.T) {
	tcases := []struct {
		name     string
		getTree  func() *node
		expected string
	}{
		{
			name: "one non-numa child",
			getTree: func() *node {
				policy := &policy{
					allocations: allocations{
						CPU: make(map[string]CPUGrant),
					},
					sys: &mockSystem{},
				}
				root := newVirtualNode("root", policy, nil)
				_ = newSocketNode(0, policy, root)
				return root
			},
		},
		{
			name: "tree with one numa node",
			getTree: func() *node {
				policy := &policy{
					allocations: allocations{
						CPU: make(map[string]CPUGrant),
					},
					sys: &mockSystem{},
				}
				root := newVirtualNode("root", policy, nil)
				socket := newSocketNode(0, policy, root)
				_ = newNumaNode(1, policy, socket)
				return root
			},
			expected: "1",
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			tree := tc.getTree()
			actual1 := tree.getMemset().String()
			actual2 := tree.getMemset().String()
			if actual1 != actual2 {
				t.Errorf("getMemset() has side effects: first result - '%s', second result - '%s'",
					actual1, actual2)
			}
			if actual2 != tc.expected {
				t.Errorf("Expected %s, but got %s", tc.expected, actual2)
			}
		})
	}
}

func TestHintScore(t *testing.T) {
	tcases := []struct {
		name          string
		getNodeData   func() hintScorer // TODO(rojkov): no need to be func, can be replaced with struct instantiation.
		hint          system.TopologyHint
		expectedScore float64
	}{
		{
			name: "empty hint for virtual node",
			getNodeData: func() hintScorer {
				return &virtualData{}
			},
		},
		{
			name: "NUMA hint for virtual node",
			getNodeData: func() hintScorer {
				return &virtualData{}
			},
			hint: system.TopologyHint{
				NUMAs: "some fake text",
			},
			expectedScore: OverfitPenalty * OverfitPenalty,
		},
		{
			name: "Socket hint for virtual node",
			getNodeData: func() hintScorer {
				return &virtualData{}
			},
			hint: system.TopologyHint{
				Sockets: "some fake text",
			},
			expectedScore: OverfitPenalty,
		},
		{
			name: "UNPARSABLE CPU hint for virtual node",
			getNodeData: func() hintScorer {
				return &virtualData{
					owner: &node{
						policy: &policy{
							sys: &system.System{},
						},
					},
				}
			},
			hint: system.TopologyHint{
				CPUs: "some fake text",
			},
		},
		{
			name: "UNPARSABLE CPU hint for socket node",
			getNodeData: func() hintScorer {
				return &socketData{
					syspkg: &system.Package{},
				}
			},
			hint: system.TopologyHint{
				CPUs: "some fake text",
			},
		},
		{
			name: "UNPARSABLE NUMA hint for socket node",
			getNodeData: func() hintScorer {
				return &socketData{
					syspkg: &system.Package{},
				}
			},
			hint: system.TopologyHint{
				NUMAs: "some fake text",
			},
		},
		{
			name: "UNPARSABLE socket hint for socket node",
			getNodeData: func() hintScorer {
				return &socketData{}
			},
			hint: system.TopologyHint{
				Sockets: "some fake text",
			},
		},
		{
			name: "empty hint for socket node",
			getNodeData: func() hintScorer {
				return &socketData{}
			},
		},
		{
			name: "UNPARSABLE CPU hint for NUMA node",
			getNodeData: func() hintScorer {
				return &numaData{
					sysnode: &system.Node{},
				}
			},
			hint: system.TopologyHint{
				CPUs: "some fake text",
			},
		},
		{
			name: "UNPARSABLE NUMA hint for NUMA node",
			getNodeData: func() hintScorer {
				return &numaData{}
			},
			hint: system.TopologyHint{
				NUMAs: "some fake text",
			},
		},
		{
			name: "UNPARSABLE socket hint for NUMA node",
			getNodeData: func() hintScorer {
				return &numaData{
					sysnode: &system.Node{},
				}
			},
			hint: system.TopologyHint{
				Sockets: "some fake text",
			},
		},
		{
			name: "empty hint for NUMA node",
			getNodeData: func() hintScorer {
				return &numaData{}
			},
		},
		{
			name: "socket hint for NUMA node",
			getNodeData: func() hintScorer {
				return &numaData{
					sysnode: &system.Node{},
					owner: &node{
						policy: &policy{
							sys: &mockSystem{},
						},
					},
				}
			},
			hint: system.TopologyHint{
				Sockets: "0",
			},
			expectedScore: 1.0,
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			actual := tc.getNodeData().hintScore(tc.hint)
			if actual != tc.expectedScore {
				t.Errorf("Expected %f, but got %f", tc.expectedScore, actual)
			}
		})
	}
}
