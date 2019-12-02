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

func TestAccountAllocate(t *testing.T) {
	tcases := []struct {
		name             string
		cs               *cpuSupply
		grant            *cpuGrant
		expectedIsolated string
		expectedSharable string
	}{
		{
			name: "same",
			cs: &cpuSupply{
				node: &node{},
			},
			grant: &cpuGrant{
				node: &node{},
			},
		},
		{
			name: "empty",
			cs: &cpuSupply{
				node: &node{
					id: 1,
				},
			},
			grant: &cpuGrant{
				node: &node{
					id: 2,
				},
			},
		},
		{
			name: "non-empty",
			cs: &cpuSupply{
				node: &node{
					id: 1,
				},
				isolated: cpuset.NewCPUSet(1, 2),
				sharable: cpuset.NewCPUSet(1, 3),
			},
			grant: &cpuGrant{
				node: &node{
					id: 2,
				},
				exclusive: cpuset.NewCPUSet(1),
			},
			expectedIsolated: "2",
			expectedSharable: "3",
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			tc.cs.AccountAllocate(tc.grant)
			if tc.expectedIsolated != tc.cs.isolated.String() {
				t.Errorf("Expected isolated '%s', but got '%s'", tc.expectedIsolated, tc.cs.isolated)
			}
			if tc.expectedSharable != tc.cs.sharable.String() {
				t.Errorf("Expected sharable '%s', but got '%s'", tc.expectedSharable, tc.cs.sharable)
			}
		})
	}
}

func TestAccountRelease(t *testing.T) {
	tcases := []struct {
		name             string
		cs               *cpuSupply
		grant            *cpuGrant
		expectedIsolated string
		expectedSharable string
	}{
		{
			name: "same",
			cs: &cpuSupply{
				node: &node{},
			},
			grant: &cpuGrant{
				node: &node{},
			},
		},
		{
			name: "empty",
			cs: &cpuSupply{
				node: &node{
					id:      1,
					nodecpu: newCPUSupply(&node{id: 1}, cpuset.NewCPUSet(), cpuset.NewCPUSet(), 0),
				},
			},
			grant: &cpuGrant{
				node: &node{
					id: 2,
				},
			},
		},
		{
			name: "non-empty",
			cs: &cpuSupply{
				node: &node{
					id:      1,
					nodecpu: newCPUSupply(&node{id: 1}, cpuset.NewCPUSet(4), cpuset.NewCPUSet(4), 1),
				},
				isolated: cpuset.NewCPUSet(1, 2),
				sharable: cpuset.NewCPUSet(1, 3),
			},
			grant: &cpuGrant{
				node: &node{
					id: 2,
				},
				exclusive: cpuset.NewCPUSet(4),
			},
			expectedIsolated: "1-2,4",
			expectedSharable: "1,3-4",
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			tc.cs.AccountRelease(tc.grant)
			if tc.expectedIsolated != tc.cs.isolated.String() {
				t.Errorf("Expected isolated '%s', but got '%s'", tc.expectedIsolated, tc.cs.isolated)
			}
			if tc.expectedSharable != tc.cs.sharable.String() {
				t.Errorf("Expected sharable '%s', but got '%s'", tc.expectedSharable, tc.cs.sharable)
			}
		})
	}
}

func TestAllocate(t *testing.T) {
	tcases := []struct {
		name          string
		cs            *cpuSupply
		req           *cpuRequest
		expectedGrant *cpuGrant
		expectedError bool
	}{
		{
			name: "empty",
			cs: &cpuSupply{
				node: &node{
					id:      1,
					freecpu: newCPUSupply(&node{id: 1}, cpuset.NewCPUSet(), cpuset.NewCPUSet(), 0),
				},
				isolated: cpuset.NewCPUSet(),
			},
			req:           &cpuRequest{},
			expectedGrant: &cpuGrant{},
		},
		{
			name: "request with full CPUs",
			cs: &cpuSupply{
				node: &node{
					id:      1,
					freecpu: newCPUSupply(&node{id: 1}, cpuset.NewCPUSet(), cpuset.NewCPUSet(), 0),
				},
				isolated: cpuset.NewCPUSet(0, 1, 2),
			},
			req: &cpuRequest{
				full: 2,
			},
			expectedGrant: &cpuGrant{},
			expectedError: true, // FIXME(rojkov): this is wrong, actually no error is expected
		},
		{
			name: "request with sharable CPUs",
			cs: &cpuSupply{
				node: &node{
					id:      1,
					freecpu: newCPUSupply(&node{id: 1}, cpuset.NewCPUSet(), cpuset.NewCPUSet(), 0),
				},
				sharable: cpuset.NewCPUSet(0, 1, 2),
			},
			req: &cpuRequest{
				full: 2,
			},
			expectedGrant: &cpuGrant{},
			expectedError: true, // TODO(rojkov): this is wrong, actually no error is expected
		},
		{
			name: "request with fraction of insufficient CPU",
			cs: &cpuSupply{
				node: &node{
					id:      1,
					freecpu: newCPUSupply(&node{id: 1}, cpuset.NewCPUSet(), cpuset.NewCPUSet(), 0),
				},
			},
			req: &cpuRequest{
				fraction: 2,
			},
			expectedGrant: &cpuGrant{},
			expectedError: true,
		},
		{
			name: "request with fraction of sufficient CPU",
			cs: &cpuSupply{
				node: &node{
					id:      1,
					freecpu: newCPUSupply(&node{id: 1}, cpuset.NewCPUSet(), cpuset.NewCPUSet(), 0),
				},
				sharable: cpuset.NewCPUSet(1),
			},
			req: &cpuRequest{
				fraction: 2,
			},
			expectedGrant: &cpuGrant{},
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := tc.cs.Allocate(tc.req)
			if tc.expectedError && err == nil {
				t.Errorf("Expected error, but got success")
			}
			if !tc.expectedError && err != nil {
				t.Errorf("Unxpected error: %+v", err)
			}
			if tc.expectedGrant == nil && actual != nil {
				t.Error("grant is not nil")
			}
		})
	}
}

func TestCpuSupplyString(t *testing.T) {
	tcases := []struct {
		name     string
		cs       *cpuSupply
		expected string
	}{
		{
			name: "non-empty",
			cs: &cpuSupply{
				node: &node{
					name: "testnode",
				},
				isolated: cpuset.NewCPUSet(0),
				sharable: cpuset.NewCPUSet(1),
			},
			expected: "<testnode CPU: isolated:0, sharable:1 (granted:0, free: 1000)>",
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			actual := tc.cs.String()
			if actual != tc.expected {
				t.Errorf("Expected %q, but got %q", tc.expected, actual)
			}
		})
	}
}

func TestCpuRequestString(t *testing.T) {
	tcases := []struct {
		name     string
		req      *cpuRequest
		expected string
	}{
		{
			name: "empty",
			req: &cpuRequest{
				container: &mockContainer{
					name: "testcontainer",
				},
			},
			expected: "<CPU request testcontainer: ->",
		},
		{
			name: "full and fraction",
			req: &cpuRequest{
				container: &mockContainer{
					name: "testcontainer",
				},
				full:     2,
				fraction: 2,
			},
			expected: "<CPU request testcontainer: full: 2, shared: 2>",
		},
		{
			name: "full only",
			req: &cpuRequest{
				container: &mockContainer{
					name: "testcontainer",
				},
				full: 2,
			},
			expected: "<CPU request testcontainer: full: 2>",
		},
		{
			name: "fraction only",
			req: &cpuRequest{
				container: &mockContainer{
					name: "testcontainer",
				},
				fraction: 2,
			},
			expected: "<CPU request testcontainer: shared: 2>",
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			actual := tc.req.String()
			if actual != tc.expected {
				t.Errorf("Expected %q, but got %q", tc.expected, actual)
			}
		})
	}
}

func TestCpuSupplyGetScore(t *testing.T) {
	tcases := []struct {
		name string
		cs   *cpuSupply
		req  *cpuRequest
	}{
		{
			name: "empty",
			cs: &cpuSupply{
				node: &node{
					name: "testnode",
					data: &virtualData{},
					policy: &policy{
						sys: &mockSystem{},
					},
					freecpu: newCPUSupply(&node{}, cpuset.NewCPUSet(), cpuset.NewCPUSet(), 0),
				},
			},
			req: &cpuRequest{
				container: &mockContainer{},
			},
		},
		{
			name: "non-empty request",
			cs: &cpuSupply{
				node: &node{
					id:   1,
					name: "testnode",
					data: &virtualData{},
					policy: &policy{
						sys: &mockSystem{},
						allocations: allocations{
							CPU: map[string]CPUGrant{
								"testcpu": &cpuGrant{
									node: &node{
										id: 1,
									},
								},
							},
						},
					},
					freecpu: newCPUSupply(&node{}, cpuset.NewCPUSet(), cpuset.NewCPUSet(), 0),
				},
			},
			req: &cpuRequest{
				container: &mockContainer{
					name: "testcontainer",
				},
				isolate: true,
			},
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			opt.FakeHints["fakepod:testcontainer"] = system.TopologyHints{
				"fakehint": system.TopologyHint{},
			}
			opt.FakeHints["testcontainer"] = system.TopologyHints{
				"fakehint": system.TopologyHint{},
			}
			_ = tc.cs.GetScore(tc.req)
			// TODO(rojkov): verify the score
		})
	}
}
