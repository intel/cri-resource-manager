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
	"io/ioutil"
	"os"
	"path"
	"testing"

	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"

	"github.com/intel/cri-resource-manager/pkg/sysfs"
	"github.com/intel/cri-resource-manager/pkg/utils"
)

func TestAllocatorHelper(t *testing.T) {
	// Create tmpdir and decompress testdata there
	tmpdir, err := ioutil.TempDir("", "cri-resource-manager-test-")
	if err != nil {
		t.Fatalf("failed to create tmpdir: %v", err)
	}
	defer os.RemoveAll(tmpdir)

	if err := utils.UncompressTbz2(path.Join("testdata", "sysfs.tar.bz2"), tmpdir); err != nil {
		t.Fatalf("failed to decompress testdata: %v", err)
	}

	// Discover mock system from the testdata
	sys, err := sysfs.DiscoverSystemAt(path.Join(tmpdir, "sysfs", "2-socket-4-node-40-core", "sys"))
	if err != nil {
		t.Fatalf("failed to discover mock system: %v", err)
	}
	topoCache := newTopologyCache(sys)

	// Fake cpu priorities: 5 cores from pkg #0 as high prio
	// Package CPUs: #0: [0-19,40-59], #1: [20-39,60-79]
	topoCache.cpuPriorities = [NumCPUPriorities]cpuset.CPUSet{
		cpuset.MustParse("2,5,8,15,17,42,45,48,55,57"),
		cpuset.MustParse("20-39,60-79"),
		cpuset.MustParse("0,1,3,4,6,7,9-14,16,18,19,40,41,43,44,46,47,49-54,56,58,59"),
	}

	tcs := []struct {
		description string
		from        cpuset.CPUSet
		prefer      CPUPriority
		cnt         int
		expected    cpuset.CPUSet
	}{
		{
			description: "too few available CPUs",
			from:        cpuset.MustParse("2,3,10-14,20"),
			prefer:      PriorityNormal,
			cnt:         9,
			expected:    cpuset.NewCPUSet(),
		},
		{
			description: "request all available CPUs",
			from:        cpuset.MustParse("2,3,10-14,20"),
			prefer:      PriorityNormal,
			cnt:         8,
			expected:    cpuset.MustParse("2,3,10-14,20"),
		},
		{
			description: "prefer high priority cpus",
			from:        cpuset.MustParse("2,3,10-25"),
			prefer:      PriorityHigh,
			cnt:         4,
			expected:    cpuset.NewCPUSet(2, 3, 15, 17),
		},
	}

	// Run tests
	for _, tc := range tcs {
		t.Run(tc.description, func(t *testing.T) {
			a := newAllocatorHelper(sys, topoCache)
			a.from = tc.from
			a.prefer = tc.prefer
			a.cnt = tc.cnt
			result := a.allocate()
			if !result.Equals(tc.expected) {
				t.Errorf("expected %q, result was %q", tc.expected, result)
			}
		})
	}
}
