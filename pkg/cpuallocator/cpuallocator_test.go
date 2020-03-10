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

package cpuallocator

import (
	"fmt"
	"testing"

	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
	//"github.com/google/go-cmp/cmp"
)

func TestCPUAllocator(t *testing.T) {
	tcs := []struct {
		description string
		from        cpuset.CPUSet
		preferred   cpuset.CPUSet
		cnt         int
		expected    cpuset.CPUSet
	}{
		{
			description: "too few available CPUs",
			from:        cpuset.NewCPUSet(2, 3, 10, 11, 12, 13, 14, 20),
			preferred:   cpuset.NewCPUSet(10, 13, 20, 23),
			cnt:         9,
			expected:    cpuset.NewCPUSet(),
		},
		{
			description: "request all available CPUs",
			from:        cpuset.NewCPUSet(2, 3, 10, 11, 12, 13, 14, 20),
			preferred:   cpuset.NewCPUSet(2, 3),
			cnt:         8,
			expected:    cpuset.NewCPUSet(2, 3, 10, 11, 12, 13, 14, 20),
		},
		{
			description: "prefer high priority cpus",
			from:        cpuset.NewCPUSet(2, 3, 10, 11, 12, 13, 14, 20),
			preferred:   cpuset.NewCPUSet(10, 13, 20, 23),
			cnt:         4,
			expected:    cpuset.NewCPUSet(2, 10, 13, 20),
		},
	}

	// Mock system discovery failure
	system = sysfsSingleton{sys: nil, err: fmt.Errorf("mock sysfs discovery error")}

	// Run tests
	for _, tc := range tcs {
		t.Run(tc.description, func(t *testing.T) {
			a := NewCPUAllocator(nil)
			a.from = tc.from
			a.preferred = tc.preferred
			a.cnt = tc.cnt
			result := a.allocate()
			if !result.Equals(tc.expected) {
				t.Errorf("expected %q, result was %q", tc.expected, result)
			}
		})
	}
}
