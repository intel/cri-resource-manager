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
	"bytes"
	"testing"

	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	policyapi "github.com/intel/cri-resource-manager/pkg/cri/resource-manager/policy"
	resapi "k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
)

// TODO(rojkov): this test is skipped because the function uses log.Fatal() in non-main code and cannot be tested without introducing global state for fake 'log'.
func TestCreateTopologyAwarePolicy(t *testing.T) {
	tcases := []struct {
		name     string
		opts     *policyapi.BackendOptions
		disabled bool
	}{
		{
			name:     "constructor should handle nil opts gracefully",
			disabled: true,
		},
		{
			name:     "non-nil but empty opts",
			opts:     &policyapi.BackendOptions{},
			disabled: true,
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.disabled {
				t.Skipf("The case '%s' is skipped", tc.name)
			}
			_ = CreateTopologyAwarePolicy(tc.opts)
			/*
				if actual != tc.expected {
					t.Errorf("Expected %s, but got %s", tc.expected, actual2)
				}
			*/
		})
	}
}

func TestStart(t *testing.T) {
	tcases := []struct {
		name          string
		disabled      bool
		getPolicy     func() *policy
		cache         cache.Cache
		add           []cache.Container
		del           []cache.Container
		expectedError bool
	}{
		{
			name: "empty policy",
			getPolicy: func() *policy {
				pol := &policy{}
				pol.root = newVirtualNode("fake root", pol, nil)
				return pol
			},
			cache: &mockCache{},
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.disabled {
				t.Skipf("The case '%s' is skipped", tc.name)
			}
			pol := tc.getPolicy()
			err := pol.Start(tc.cache, tc.add, tc.del)
			if tc.expectedError && err == nil {
				t.Errorf("Expected error, but got success")
			}
			if !tc.expectedError && err != nil {
				t.Errorf("Unxpected error: %+v", err)
			}
		})
	}
}

func TestAllocateResources(t *testing.T) {
	tcases := []struct {
		name          string
		disabled      bool
		getPolicy     func() *policy
		container     cache.Container
		expectedError bool
	}{
		{
			name: "AllocateResources() should handle nil Container gracefully",
			getPolicy: func() *policy {
				pol := &policy{}
				pol.root = newVirtualNode("fake root", pol, nil)
				return pol
			},
			disabled: true,
		},
		{
			name: "no error",
			getPolicy: func() *policy {
				pol := &policy{}
				pol.root = newVirtualNode("fake root", pol, nil)
				pol.root.freecpu = newCPUSupply(pol.root, cpuset.NewCPUSet(), cpuset.NewCPUSet(), 1)
				pol.root.nodecpu = newCPUSupply(pol.root, cpuset.NewCPUSet(), cpuset.NewCPUSet(), 1)
				pol.pools = []*node{pol.root}
				pol.allocations = allocations{CPU: make(map[string]CPUGrant)}
				pol.cache = &mockCache{}
				return pol
			},
			container: &mockContainer{},
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.disabled {
				t.Skipf("The case '%s' is skipped", tc.name)
			}
			pol := tc.getPolicy()
			err := pol.AllocateResources(tc.container)
			if tc.expectedError && err == nil {
				t.Errorf("Expected error, but got success")
			}
			if !tc.expectedError && err != nil {
				t.Errorf("Unxpected error: %+v", err)
			}
		})
	}
}

func TestReleaseResources(t *testing.T) {
	tcases := []struct {
		name          string
		disabled      bool
		getPolicy     func() *policy
		container     cache.Container
		expectedError bool
	}{
		{
			name: "ReleaseResources() should handle nil Container gracefully",
			getPolicy: func() *policy {
				pol := &policy{}
				pol.root = newVirtualNode("fake root", pol, nil)
				return pol
			},
			disabled: true,
		},
		{
			name: "no grant found",
			getPolicy: func() *policy {
				pol := &policy{}
				pol.root = newVirtualNode("fake root", pol, nil)
				pol.allocations = allocations{CPU: make(map[string]CPUGrant)}
				return pol
			},
			container: &mockContainer{},
		},
		{
			name: "grant found",
			getPolicy: func() *policy {
				pol := &policy{}
				pol.root = newVirtualNode("fake root", pol, nil)
				pol.root.freecpu = newCPUSupply(pol.root, cpuset.NewCPUSet(), cpuset.NewCPUSet(), 1)
				pol.root.nodecpu = newCPUSupply(pol.root, cpuset.NewCPUSet(), cpuset.NewCPUSet(), 1)
				pol.allocations = allocations{
					CPU: map[string]CPUGrant{
						"0": &cpuGrant{
							node:      pol.root,
							container: &mockContainer{},
						},
					},
				}
				pol.cache = &mockCache{}
				return pol
			},
			container: &mockContainer{},
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.disabled {
				t.Skipf("The case '%s' is skipped", tc.name)
			}
			pol := tc.getPolicy()
			err := pol.ReleaseResources(tc.container)
			if tc.expectedError && err == nil {
				t.Errorf("Expected error, but got success")
			}
			if !tc.expectedError && err != nil {
				t.Errorf("Unxpected error: %+v", err)
			}
		})
	}
}

func TestUpdateResources(t *testing.T) {
	tcases := []struct {
		name          string
		disabled      bool
		policy        *policy
		container     cache.Container
		expectedError bool
	}{
		{
			name:     "grace handling of nil container",
			policy:   &policy{},
			disabled: true,
		},
		{
			name:      "no error",
			policy:    &policy{},
			container: &mockContainer{},
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.disabled {
				t.Skipf("The case '%s' is skipped", tc.name)
			}
			err := tc.policy.UpdateResources(tc.container)
			if tc.expectedError && err == nil {
				t.Errorf("Expected error, but got success")
			}
			if !tc.expectedError && err != nil {
				t.Errorf("Unxpected error: %+v", err)
			}
		})
	}
}

func TestPostStart(t *testing.T) {
	tcases := []struct {
		name          string
		policy        *policy
		container     cache.Container
		expectedError bool
	}{
		{
			name:   "grace handling of nil container",
			policy: &policy{},
		},
		{
			name:      "no error",
			policy:    &policy{},
			container: &mockContainer{},
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.policy.PostStart(tc.container)
			if tc.expectedError && err == nil {
				t.Errorf("Expected error, but got success")
			}
			if !tc.expectedError && err != nil {
				t.Errorf("Unxpected error: %+v", err)
			}
		})
	}
}

func TestExposrtResourceData(t *testing.T) {
	tcases := []struct {
		name      string
		disabled  bool
		getPolicy func() *policy
		container cache.Container
		expected  []byte
	}{
		{
			name: "ReleaseResources() should handle nil Container gracefully",
			getPolicy: func() *policy {
				pol := &policy{}
				pol.root = newVirtualNode("fake root", pol, nil)
				return pol
			},
			disabled: true,
		},
		{
			name: "no grant found",
			getPolicy: func() *policy {
				pol := &policy{}
				pol.root = newVirtualNode("fake root", pol, nil)
				pol.allocations = allocations{CPU: make(map[string]CPUGrant)}
				return pol
			},
			container: &mockContainer{},
		},
		{
			name: "grant found",
			getPolicy: func() *policy {
				pol := &policy{}
				pol.root = newVirtualNode("fake root", pol, nil)
				pol.root.freecpu = newCPUSupply(pol.root, cpuset.NewCPUSet(), cpuset.NewCPUSet(), 1)
				pol.root.nodecpu = newCPUSupply(pol.root, cpuset.NewCPUSet(), cpuset.NewCPUSet(), 1)
				pol.allocations = allocations{
					CPU: map[string]CPUGrant{
						"0": &cpuGrant{
							node:      pol.root,
							container: &mockContainer{},
						},
					},
				}
				pol.cache = &mockCache{}
				return pol
			},
			container: &mockContainer{},
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.disabled {
				t.Skipf("The case '%s' is skipped", tc.name)
			}
			pol := tc.getPolicy()
			output := pol.ExportResourceData(tc.container, "fake data syntax")
			if !bytes.Equal(output, tc.expected) {
				t.Errorf("Expected %q, but got %q", string(tc.expected), string(output))
			}
		})
	}
}

func TestConfigNotify(t *testing.T) {
	tcases := []struct {
		name          string
		policy        *policy
		expectedError bool
	}{
		{
			name: "no error",
			policy: &policy{
				cache: &mockCache{},
			},
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.policy.configNotify("fake event", "fake source")
			if tc.expectedError && err == nil {
				t.Errorf("Expected error, but got success")
			}
			if !tc.expectedError && err != nil {
				t.Errorf("Unxpected error: %+v", err)
			}
		})
	}
}

func TestDiscoverSystemTopology(t *testing.T) {
	tcases := []struct {
		name          string
		policy        *policy
		expectedError bool
	}{
		{
			name:          "broken system discovery",
			policy:        &policy{},
			expectedError: true,
		},
		// TODO(rojkov): add test case for successful discovery (this needs mocked system.DiscoverySystem())
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.policy.discoverSystemTopology()
			if tc.expectedError && err == nil {
				t.Errorf("Expected error, but got success")
			}
			if !tc.expectedError && err != nil {
				t.Errorf("Unxpected error: %+v", err)
			}
		})
	}
}

func TestCheckConstraints(t *testing.T) {
	tcases := []struct {
		name          string
		policy        *policy
		expectedError bool
	}{
		{
			name: "no CPU reservations",
			policy: &policy{
				sys: &mockSystem{},
			},
			expectedError: true,
		},
		{
			name: "with unknown CPU reservations",
			policy: &policy{
				options: policyapi.BackendOptions{
					Reserved: policyapi.ConstraintSet{
						policyapi.DomainCPU: struct{}{},
					},
				},
				sys: &mockSystem{},
			},
		},
		{
			name: "with cpuset CPU reservations, but not in the allowed set",
			policy: &policy{
				options: policyapi.BackendOptions{
					Reserved: policyapi.ConstraintSet{
						policyapi.DomainCPU: cpuset.NewCPUSet(0),
					},
				},
				sys: &mockSystem{},
			},
			expectedError: true,
		},
		{
			name: "with cpuset CPU reservations",
			policy: &policy{
				options: policyapi.BackendOptions{
					Reserved: policyapi.ConstraintSet{
						policyapi.DomainCPU: cpuset.NewCPUSet(1),
					},
					Available: policyapi.ConstraintSet{
						policyapi.DomainCPU: cpuset.NewCPUSet(1),
					},
				},
				sys: &mockSystem{},
			},
		},
		{
			name: "with cpuset CPU reservations, but isolated at the same time",
			policy: &policy{
				options: policyapi.BackendOptions{
					Reserved: policyapi.ConstraintSet{
						policyapi.DomainCPU: cpuset.NewCPUSet(6),
					},
					Available: policyapi.ConstraintSet{
						policyapi.DomainCPU: cpuset.NewCPUSet(6),
					},
				},
				sys: &mockSystem{
					isolatedCPU: 6,
				},
			},
			expectedError: true,
		},
		{
			name: "with Quantity reservations",
			policy: &policy{
				options: policyapi.BackendOptions{
					Reserved: policyapi.ConstraintSet{
						policyapi.DomainCPU: resapi.MustParse("12M"),
					},
				},
				sys: &mockSystem{},
			},
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.policy.checkConstraints()
			if tc.expectedError && err == nil {
				t.Errorf("Expected error, but got success")
			}
			if !tc.expectedError && err != nil {
				t.Errorf("Unxpected error: %+v", err)
			}
		})
	}
}
