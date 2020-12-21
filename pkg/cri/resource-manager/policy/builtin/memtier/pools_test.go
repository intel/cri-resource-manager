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

package memtier

import (
	"archive/tar"
	"compress/bzip2"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	policyapi "github.com/intel/cri-resource-manager/pkg/cri/resource-manager/policy"

	v1 "k8s.io/api/core/v1"
	resapi "k8s.io/apimachinery/pkg/api/resource"

	system "github.com/intel/cri-resource-manager/pkg/sysfs"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
)

func findNodeWithID(id int, nodes []Node) Node {
	for _, node := range nodes {
		if node.NodeID() == id {
			return node
		}
	}
	panic("No node found")
}

func findNodeWithName(name string, nodes []Node) Node {
	for _, node := range nodes {
		if node.Name() == name {
			return node
		}
	}
	panic("No node found")
}

func setLinks(nodes []Node, tree map[int][]int) {
	hasParent := map[int]struct{}{}
	for parent, children := range tree {
		parentNode := findNodeWithID(parent, nodes)
		for _, child := range children {
			childNode := findNodeWithID(child, nodes)
			childNode.LinkParent(parentNode)
			hasParent[child] = struct{}{}
		}
	}
	orphans := []int{}
	for id := range tree {
		if _, ok := hasParent[id]; !ok {
			node := findNodeWithID(id, nodes)
			node.LinkParent(nilnode)
			orphans = append(orphans, id)
		}
	}
	if len(orphans) != 1 {
		panic(fmt.Sprintf("expected one root node, got %d with IDs %v", len(orphans), orphans))
	}
}

func uncompress(archive string, dir string) error {
	file, err := os.Open(archive)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	data := bzip2.NewReader(file)
	tr := tar.NewReader(data)
	for {
		header, err := tr.Next()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if header.Typeflag == tar.TypeDir {
			// Create a directory.
			err = os.MkdirAll(path.Join(dir, header.Name), 0755)
			if err != nil {
				return err
			}
		} else if header.Typeflag == tar.TypeReg {
			// Create a regular file.
			targetFile, err := os.Create(path.Join(dir, header.Name))
			if err != nil {
				return err
			}
			_, err = io.Copy(targetFile, tr)
			targetFile.Close()
			if err != nil {
				return err
			}
		} else if header.Typeflag == tar.TypeSymlink {
			// Create a symlink and all the directories it needs.
			err = os.MkdirAll(path.Dir(path.Join(dir, header.Name)), 0755)
			if err != nil {
				return err
			}
			err := os.Symlink(header.Linkname, path.Join(dir, header.Name))
			if err != nil {
				return err
			}
		}
	}
}

func TestMemoryLimitFiltering(t *testing.T) {

	// Test the scoring algorithm with synthetic data. The assumptions are:

	// 1. The first node in "nodes" is the root of the tree.

	tcases := []struct {
		name                   string
		nodes                  []Node
		numaNodes              []system.Node
		req                    Request
		affinities             map[int]int32
		tree                   map[int][]int
		expectedRemainingNodes []int
	}{
		{
			name: "single node memory limit (fits)",
			nodes: []Node{
				&numanode{
					node: node{
						id:      100,
						name:    "testnode0",
						kind:    UnknownNode,
						noderes: newSupply(&node{}, cpuset.NewCPUSet(), cpuset.NewCPUSet(), 0, createMemoryMap(10001, 0, 0), createMemoryMap(0, 0, 0)),
						freeres: newSupply(&node{}, cpuset.NewCPUSet(), cpuset.NewCPUSet(), 0, createMemoryMap(10001, 0, 0), createMemoryMap(0, 0, 0)),
					},
					id: 0, // system node id
				},
			},
			numaNodes: []system.Node{
				&mockSystemNode{id: 0, memFree: 10001, memTotal: 10001},
			},
			req: &request{
				memReq:    10000,
				memLim:    10000,
				memType:   defaultMemoryType,
				container: &mockContainer{},
			},
			expectedRemainingNodes: []int{100},
			tree:                   map[int][]int{100: {}},
		},
		{
			name: "single node memory limit (doesn't fit)",
			nodes: []Node{
				&numanode{
					node: node{
						id:      100,
						name:    "testnode0",
						kind:    UnknownNode,
						noderes: newSupply(&node{}, cpuset.NewCPUSet(), cpuset.NewCPUSet(), 0, createMemoryMap(9999, 0, 0), createMemoryMap(0, 0, 0)),
						freeres: newSupply(&node{}, cpuset.NewCPUSet(), cpuset.NewCPUSet(), 0, createMemoryMap(9999, 0, 0), createMemoryMap(0, 0, 0)),
					},
					id: 0, // system node id
				},
			},
			numaNodes: []system.Node{
				&mockSystemNode{id: 0, memFree: 9999, memTotal: 9999},
			},
			req: &request{
				memReq:    10000,
				memLim:    10000,
				memType:   defaultMemoryType,
				container: &mockContainer{},
			},
			expectedRemainingNodes: []int{},
			tree:                   map[int][]int{100: {}},
		},
		{
			name: "two node memory limit (fits to leaf)",
			nodes: []Node{
				&virtualnode{
					node: node{
						id:      100,
						name:    "testnode0",
						kind:    UnknownNode,
						noderes: newSupply(&node{}, cpuset.NewCPUSet(), cpuset.NewCPUSet(), 0, createMemoryMap(10001, 0, 0), createMemoryMap(0, 0, 0)),
						freeres: newSupply(&node{}, cpuset.NewCPUSet(), cpuset.NewCPUSet(), 0, createMemoryMap(10001, 0, 0), createMemoryMap(0, 0, 0)),
					},
				},
				&numanode{
					node: node{
						id:      101,
						name:    "testnode1",
						kind:    UnknownNode,
						noderes: newSupply(&node{}, cpuset.NewCPUSet(), cpuset.NewCPUSet(), 0, createMemoryMap(10001, 0, 0), createMemoryMap(0, 0, 0)),
						freeres: newSupply(&node{}, cpuset.NewCPUSet(), cpuset.NewCPUSet(), 0, createMemoryMap(10001, 0, 0), createMemoryMap(0, 0, 0)),
					},
					id: 0, // system node id
				},
			},
			numaNodes: []system.Node{
				&mockSystemNode{id: 0, memFree: 10001, memTotal: 10001},
			},
			req: &request{
				memReq:    10000,
				memLim:    10000,
				memType:   defaultMemoryType,
				container: &mockContainer{},
			},
			expectedRemainingNodes: []int{100, 101},
			tree:                   map[int][]int{100: {101}, 101: {}},
		},
		{
			name: "three node memory limit (fits to root)",
			nodes: []Node{
				&virtualnode{
					node: node{
						id:      100,
						name:    "testnode0",
						kind:    UnknownNode,
						noderes: newSupply(&node{}, cpuset.NewCPUSet(), cpuset.NewCPUSet(), 0, createMemoryMap(12000, 0, 0), createMemoryMap(0, 0, 0)),
						freeres: newSupply(&node{}, cpuset.NewCPUSet(), cpuset.NewCPUSet(), 0, createMemoryMap(12000, 0, 0), createMemoryMap(0, 0, 0)),
					},
				},
				&numanode{
					node: node{
						id:      101,
						name:    "testnode1",
						kind:    UnknownNode,
						noderes: newSupply(&node{}, cpuset.NewCPUSet(), cpuset.NewCPUSet(), 0, createMemoryMap(6000, 0, 0), createMemoryMap(0, 0, 0)),
						freeres: newSupply(&node{}, cpuset.NewCPUSet(), cpuset.NewCPUSet(), 0, createMemoryMap(6000, 0, 0), createMemoryMap(0, 0, 0)),
					},
					id: 0, // system node id
				},
				&numanode{
					node: node{
						id:      102,
						name:    "testnode2",
						kind:    UnknownNode,
						noderes: newSupply(&node{}, cpuset.NewCPUSet(), cpuset.NewCPUSet(), 0, createMemoryMap(6000, 0, 0), createMemoryMap(0, 0, 0)),
						freeres: newSupply(&node{}, cpuset.NewCPUSet(), cpuset.NewCPUSet(), 0, createMemoryMap(6000, 0, 0), createMemoryMap(0, 0, 0)),
					},
					id: 1, // system node id
				},
			},
			numaNodes: []system.Node{
				&mockSystemNode{id: 0, memFree: 6000, memTotal: 6000},
				&mockSystemNode{id: 1, memFree: 6000, memTotal: 6000},
			},
			req: &request{
				memReq:    10000,
				memLim:    10000,
				memType:   defaultMemoryType,
				container: &mockContainer{},
			},
			expectedRemainingNodes: []int{100},
			tree:                   map[int][]int{100: {101, 102}, 101: {}, 102: {}},
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			setLinks(tc.nodes, tc.tree)
			policy := &policy{
				sys: &mockSystem{
					nodes: tc.numaNodes,
				},
				pools:       tc.nodes,
				cache:       &mockCache{},
				root:        tc.nodes[0],
				nodeCnt:     len(tc.nodes),
				allocations: allocations{},
			}
			// back pointers
			for _, node := range tc.nodes {
				switch node.(type) {
				case *numanode:
					numaNode := node.(*numanode)
					numaNode.self.node = numaNode
					noderes := numaNode.noderes.(*supply)
					noderes.node = node
					freeres := numaNode.freeres.(*supply)
					freeres.node = node
					numaNode.policy = policy
				case *virtualnode:
					virtualNode := node.(*virtualnode)
					virtualNode.self.node = virtualNode
					noderes := virtualNode.noderes.(*supply)
					noderes.node = node
					freeres := virtualNode.freeres.(*supply)
					freeres.node = node
					virtualNode.policy = policy
				}
			}
			policy.allocations.policy = policy

			scores, filteredPools := policy.sortPoolsByScore(tc.req, tc.affinities)
			fmt.Printf("scores: %v, remaining pools: %v\n", scores, filteredPools)

			if len(filteredPools) != len(tc.expectedRemainingNodes) {
				t.Errorf("Wrong nodes in the filtered pool: expected %v but got %v", tc.expectedRemainingNodes, filteredPools)
			}

			for _, id := range tc.expectedRemainingNodes {
				found := false
				for _, node := range filteredPools {
					if node.NodeID() == id {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Did not find id %d in filtered pools: %v", id, filteredPools)
				}
			}
		})
	}
}

func TestPoolCreation(t *testing.T) {

	// Test pool creation with "real" sysfs data.

	// Create a temporary directory for the test data.
	dir, err := ioutil.TempDir("", "cri-resource-manager-test-sysfs-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	// Uncompress the test data to the directory.
	err = uncompress(path.Join("testdata", "sysfs.tar.bz2"), dir)
	if err != nil {
		panic(err)
	}

	tcases := []struct {
		path                    string
		name                    string
		req                     Request
		affinities              map[int]int32
		expectedRemainingNodes  []int
		expectedFirstNodeMemory memoryType
		expectedLeafNodeCPUs    int
		expectedRootNodeCPUs    int
		// TODO: expectedRootNodeMemory   int
	}{
		{
			path: path.Join(dir, "sysfs", "desktop", "sys"),
			name: "sysfs pool creation from a desktop system",
			req: &request{
				memReq:    10000,
				memLim:    10000,
				memType:   memoryAll,
				container: &mockContainer{},
			},
			expectedRemainingNodes:  []int{0},
			expectedFirstNodeMemory: memoryUnspec,
			expectedLeafNodeCPUs:    20,
			expectedRootNodeCPUs:    20,
		},
		{
			path: path.Join(dir, "sysfs", "server", "sys"),
			name: "sysfs pool creation from a server system",
			req: &request{
				memReq:    10000,
				memLim:    10000,
				memType:   memoryDRAM,
				container: &mockContainer{},
			},
			expectedRemainingNodes:  []int{0, 1, 2, 3, 4, 5, 6},
			expectedFirstNodeMemory: memoryDRAM | memoryPMEM,
			expectedLeafNodeCPUs:    28,
			expectedRootNodeCPUs:    112,
		},
		{
			path: path.Join(dir, "sysfs", "server", "sys"),
			name: "pmem request on a server system",
			req: &request{
				memReq:    10000,
				memLim:    10000,
				memType:   memoryDRAM | memoryPMEM,
				container: &mockContainer{},
			},
			expectedRemainingNodes:  []int{0, 1, 2, 3, 4, 5, 6},
			expectedFirstNodeMemory: memoryDRAM | memoryPMEM,
			expectedLeafNodeCPUs:    28,
			expectedRootNodeCPUs:    112,
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			sys, err := system.DiscoverSystemAt(tc.path)
			if err != nil {
				panic(err)
			}

			reserved, _ := resapi.ParseQuantity("750m")
			policyOptions := &policyapi.BackendOptions{
				Cache:  &mockCache{},
				System: sys,
				Reserved: policyapi.ConstraintSet{
					policyapi.DomainCPU: reserved,
				},
			}

			log.EnableDebug(true)
			policy := CreateMemtierPolicy(policyOptions).(*policy)
			log.EnableDebug(false)

			if policy.root.GetSupply().SharableCPUs().Size()+policy.root.GetSupply().IsolatedCPUs().Size() != tc.expectedRootNodeCPUs {
				t.Errorf("Expected %d CPUs, got %d", tc.expectedRootNodeCPUs,
					policy.root.GetSupply().SharableCPUs().Size()+policy.root.GetSupply().IsolatedCPUs().Size())
			}

			for _, p := range policy.pools {
				if p.IsLeafNode() {
					if len(p.Children()) != 0 {
						t.Errorf("Leaf node %v had %d children", p, len(p.Children()))
					}
					if p.GetSupply().SharableCPUs().Size()+p.GetSupply().IsolatedCPUs().Size() != tc.expectedLeafNodeCPUs {
						t.Errorf("Expected %d CPUs, got %d (%s)", tc.expectedLeafNodeCPUs,
							p.GetSupply().SharableCPUs().Size()+p.GetSupply().IsolatedCPUs().Size(),
							p.GetSupply().DumpCapacity())
					}
				}
			}

			scores, filteredPools := policy.sortPoolsByScore(tc.req, tc.affinities)
			fmt.Printf("scores: %v, remaining pools: %v\n", scores, filteredPools)

			if len(filteredPools) != len(tc.expectedRemainingNodes) {
				t.Errorf("Wrong number of nodes in the filtered pool: expected %d but got %d", len(tc.expectedRemainingNodes), len(filteredPools))
			}

			for _, id := range tc.expectedRemainingNodes {
				found := false
				for _, node := range filteredPools {
					if node.NodeID() == id {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Did not find id %d in filtered pools: %s", id, filteredPools)
				}
			}

			if len(filteredPools) > 0 && filteredPools[0].GetMemoryType() != tc.expectedFirstNodeMemory {
				t.Errorf("Expected first node memory type %v, got %v", tc.expectedFirstNodeMemory, filteredPools[0].GetMemoryType())
			}
		})
	}
}

func TestWorkloadPlacement(t *testing.T) {

	// Do some workloads (containers) and see how they are placed in the
	// server system.

	// Create a temporary directory for the test data.
	dir, err := ioutil.TempDir("", "cri-resource-manager-test-sysfs-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	// Uncompress the test data to the directory.
	err = uncompress(path.Join("testdata", "sysfs.tar.bz2"), dir)
	if err != nil {
		panic(err)
	}

	tcases := []struct {
		path                   string
		name                   string
		req                    Request
		affinities             map[int]int32
		expectedRemainingNodes []int
		expectedLeafNode       bool
	}{
		{
			path: path.Join(dir, "sysfs", "server", "sys"),
			name: "workload placement on a server system leaf node",
			req: &request{
				memReq:  10000,
				memLim:  10000,
				memType: memoryUnspec,
				isolate: false,
				full:    25, // 28 - 2 isolated = 26: but fully exhausting the shared CPU subpool is disallowed

				container: &mockContainer{},
			},
			expectedRemainingNodes: []int{0, 1, 2, 3, 4, 5, 6},
			expectedLeafNode:       true,
		},
		{
			path: path.Join(dir, "sysfs", "server", "sys"),
			name: "workload placement on a server system root node: CPUs don't fit to leaf",
			req: &request{
				memReq:    10000,
				memLim:    10000,
				memType:   memoryUnspec,
				isolate:   false,
				full:      29,
				container: &mockContainer{},
			},
			expectedRemainingNodes: []int{0, 1, 2, 3, 4, 5, 6},
			expectedLeafNode:       false,
		},
		{
			path: path.Join(dir, "sysfs", "server", "sys"),
			name: "workload placement on a server system root node: memory doesn't fit to leaf",
			req: &request{
				memReq:    190000000000,
				memLim:    190000000000,
				memType:   memoryUnspec,
				isolate:   false,
				full:      28,
				container: &mockContainer{},
			},
			expectedRemainingNodes: []int{2, 6},
			expectedLeafNode:       false,
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			sys, err := system.DiscoverSystemAt(tc.path)
			if err != nil {
				panic(err)
			}

			reserved, _ := resapi.ParseQuantity("750m")
			policyOptions := &policyapi.BackendOptions{
				Cache:  &mockCache{},
				System: sys,
				Reserved: policyapi.ConstraintSet{
					policyapi.DomainCPU: reserved,
				},
			}

			log.EnableDebug(true)
			policy := CreateMemtierPolicy(policyOptions).(*policy)
			log.EnableDebug(false)

			scores, filteredPools := policy.sortPoolsByScore(tc.req, tc.affinities)
			fmt.Printf("scores: %v, remaining pools: %v\n", scores, filteredPools)

			if len(filteredPools) != len(tc.expectedRemainingNodes) {
				t.Errorf("Wrong number of nodes in the filtered pool: expected %d but got %d", len(tc.expectedRemainingNodes), len(filteredPools))
			}

			for _, id := range tc.expectedRemainingNodes {
				found := false
				for _, node := range filteredPools {
					if node.NodeID() == id {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Did not find id %d in filtered pools: %s", id, filteredPools)
				}
			}
			if filteredPools[0].IsLeafNode() != tc.expectedLeafNode {
				t.Errorf("Workload should have been placed in a leaf node: %t", tc.expectedLeafNode)
			}
		})
	}
}

func TestContainerMove(t *testing.T) {

	// In case there's not enough memory to guarantee that the
	// containers running on child nodes won't get OOM killed, they need
	// to be moved upwards in the tree.

	// Create a temporary directory for the test data.
	dir, err := ioutil.TempDir("", "cri-resource-manager-test-sysfs-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	// Uncompress the test data to the directory.
	err = uncompress(path.Join("testdata", "sysfs.tar.bz2"), dir)
	if err != nil {
		panic(err)
	}

	tcases := []struct {
		path                          string
		name                          string
		container1                    cache.Container
		container2                    cache.Container
		container3                    cache.Container
		affinities                    map[int]int32
		expectedLeafNodeForContainer1 bool
		expectedLeafNodeForContainer2 bool
		expectedLeafNodeForContainer3 bool
		expectedChangeForContainer1   bool
		expectedChangeForContainer2   bool
		expectedChangeForContainer3   bool
	}{
		{
			path: path.Join(dir, "sysfs", "server", "sys"),
			name: "workload placement on a server system leaf node",
			container1: &mockContainer{
				returnValueForGetResourceRequirements: v1.ResourceRequirements{
					Limits: v1.ResourceList{
						v1.ResourceCPU:    resapi.MustParse("2"),
						v1.ResourceMemory: resapi.MustParse("1000"),
					},
				},
				returnValueForGetCacheID: "first",
			},
			container2: &mockContainer{
				returnValueForGetResourceRequirements: v1.ResourceRequirements{
					Limits: v1.ResourceList{
						v1.ResourceCPU:    resapi.MustParse("2"),
						v1.ResourceMemory: resapi.MustParse("1000"),
					},
				},
				returnValueForGetCacheID: "second",
			},
			container3: &mockContainer{
				returnValueForGetResourceRequirements: v1.ResourceRequirements{
					Limits: v1.ResourceList{
						v1.ResourceCPU:    resapi.MustParse("2"),
						v1.ResourceMemory: resapi.MustParse("1000"),
					},
				},
				returnValueForGetCacheID: "third",
			},
			expectedLeafNodeForContainer1: true,
			expectedLeafNodeForContainer2: true,
			expectedLeafNodeForContainer3: true,
		},
		{
			path: path.Join(dir, "sysfs", "server", "sys"),
			name: "workload placement on a server system non-leaf node",
			container1: &mockContainer{
				returnValueForGetResourceRequirements: v1.ResourceRequirements{
					Limits: v1.ResourceList{
						v1.ResourceCPU:    resapi.MustParse("2"),
						v1.ResourceMemory: resapi.MustParse("1000"),
					},
				},
				returnValueForGetCacheID: "first",
			},
			container2: &mockContainer{
				returnValueForGetResourceRequirements: v1.ResourceRequirements{
					Limits: v1.ResourceList{
						v1.ResourceCPU:    resapi.MustParse("2"),
						v1.ResourceMemory: resapi.MustParse("190000000000"), // 800 GB
					},
				},
				returnValueForGetCacheID: "second",
			},
			container3: &mockContainer{
				returnValueForGetResourceRequirements: v1.ResourceRequirements{
					Limits: v1.ResourceList{
						v1.ResourceCPU:    resapi.MustParse("2"),
						v1.ResourceMemory: resapi.MustParse("140000000000"), // 900 GB
					},
				},
				returnValueForGetCacheID: "third",
			},
			expectedLeafNodeForContainer1: false,
			expectedLeafNodeForContainer2: false,
			expectedLeafNodeForContainer3: true,
			expectedChangeForContainer1:   true,
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			sys, err := system.DiscoverSystemAt(tc.path)
			if err != nil {
				panic(err)
			}

			reserved, _ := resapi.ParseQuantity("750m")
			policyOptions := &policyapi.BackendOptions{
				Cache:  &mockCache{},
				System: sys,
				Reserved: policyapi.ConstraintSet{
					policyapi.DomainCPU: reserved,
				},
			}

			log.EnableDebug(true)
			policy := CreateMemtierPolicy(policyOptions).(*policy)
			log.EnableDebug(false)

			grant1, err := policy.allocatePool(tc.container1, "")
			if err != nil {
				panic(err)
			}
			fmt.Printf("grant 1 memsets: dram %s, pmem %s\n", grant1.GetMemoryNode().GetMemset(memoryDRAM), grant1.GetMemoryNode().GetMemset(memoryPMEM))

			grant2, err := policy.allocatePool(tc.container2, "")
			if err != nil {
				panic(err)
			}
			fmt.Printf("grant 2 memsets: dram %s, pmem %s\n", grant2.GetMemoryNode().GetMemset(memoryDRAM), grant2.GetMemoryNode().GetMemset(memoryPMEM))

			grant3, err := policy.allocatePool(tc.container3, "")
			if err != nil {
				panic(err)
			}
			fmt.Printf("grant 3 memsets: dram %s, pmem %s\n", grant3.GetMemoryNode().GetMemset(memoryDRAM), grant3.GetMemoryNode().GetMemset(memoryPMEM))

			if (grant1.GetCPUNode().IsSameNode(grant1.GetMemoryNode())) && tc.expectedChangeForContainer1 {
				t.Errorf("Workload 1 should have been relocated: %t, node: %s", tc.expectedChangeForContainer1, grant1.GetMemoryNode().Name())
			}
			if (grant2.GetCPUNode().IsSameNode(grant2.GetMemoryNode())) && tc.expectedChangeForContainer2 {
				t.Errorf("Workload 2 should have been relocated: %t, node: %s", tc.expectedChangeForContainer2, grant2.GetMemoryNode().Name())
			}
			if (grant3.GetCPUNode().IsSameNode(grant3.GetMemoryNode())) && tc.expectedChangeForContainer3 {
				t.Errorf("Workload 3 should have been relocated: %t, node: %s", tc.expectedChangeForContainer3, grant3.GetMemoryNode().Name())
			}

			if grant1.GetMemoryNode().IsLeafNode() != tc.expectedLeafNodeForContainer1 {
				t.Errorf("Workload 1 should have been placed in a leaf node: %t, node: %s", tc.expectedLeafNodeForContainer1, grant1.GetMemoryNode().Name())
			}
			if grant2.GetMemoryNode().IsLeafNode() != tc.expectedLeafNodeForContainer2 {
				t.Errorf("Workload 2 should have been placed in a leaf node: %t, node: %s", tc.expectedLeafNodeForContainer2, grant2.GetMemoryNode().Name())
			}
			if grant3.GetMemoryNode().IsLeafNode() != tc.expectedLeafNodeForContainer3 {
				t.Errorf("Workload 3 should have been placed in a leaf node: %t, node: %s", tc.expectedLeafNodeForContainer3, grant3.GetMemoryNode().Name())
			}
		})
	}
}

func TestAffinities(t *testing.T) {
	//
	// Test how (already pre-calculated) affinities affect workload placement.
	//

	// Create a temporary directory for the test data.
	dir, err := ioutil.TempDir("", "cri-resource-manager-test-sysfs-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	// Uncompress the test data to the directory.
	err = uncompress(path.Join("testdata", "sysfs.tar.bz2"), dir)
	if err != nil {
		panic(err)
	}

	tcases := []struct {
		path       string
		name       string
		req        Request
		affinities map[string]int32
		expected   string
	}{
		{
			path: path.Join(dir, "sysfs", "server", "sys"),
			name: "no affinities",
			req: &request{
				memReq:    10000,
				memLim:    10000,
				memType:   memoryUnspec,
				isolate:   false,
				full:      3,
				container: &mockContainer{},
			},
			affinities: map[string]int32{},
			expected:   "numa node #0",
		},
		{
			path: path.Join(dir, "sysfs", "server", "sys"),
			name: "affinity to NUMA node #1",
			req: &request{
				memReq:    10000,
				memLim:    10000,
				memType:   memoryUnspec,
				isolate:   false,
				full:      3,
				container: &mockContainer{},
			},
			affinities: map[string]int32{
				"numa node #1": 1,
			},
			expected: "numa node #1",
		},
		{
			path: path.Join(dir, "sysfs", "server", "sys"),
			name: "affinity to socket #1",
			req: &request{
				memReq:    10000,
				memLim:    10000,
				memType:   memoryUnspec,
				isolate:   false,
				full:      3,
				container: &mockContainer{},
			},
			affinities: map[string]int32{
				"socket #1": 1,
			},
			expected: "socket #1",
		},
		{
			path: path.Join(dir, "sysfs", "server", "sys"),
			name: "equal affinities to numa node #1, socket #1",
			req: &request{
				memReq:    10000,
				memLim:    10000,
				memType:   memoryUnspec,
				isolate:   false,
				full:      3,
				container: &mockContainer{},
			},
			affinities: map[string]int32{
				"socket #1":    1,
				"numa node #1": 1,
			},
			expected: "numa node #1",
		},
		{
			path: path.Join(dir, "sysfs", "server", "sys"),
			name: "equal affinities to numa node #1, numa node #3",
			req: &request{
				memReq:    10000,
				memLim:    10000,
				memType:   memoryUnspec,
				isolate:   false,
				full:      3,
				container: &mockContainer{},
			},
			affinities: map[string]int32{
				"numa node #1": 1,
				"numa node #3": 1,
			},
			expected: "socket #1",
		},
		{
			path: path.Join(dir, "sysfs", "server", "sys"),
			name: "double affinity to numa node #1 vs. #3",
			req: &request{
				memReq:    10000,
				memLim:    10000,
				memType:   memoryUnspec,
				isolate:   false,
				full:      3,
				container: &mockContainer{},
			},
			affinities: map[string]int32{
				"numa node #1": 2,
				"numa node #3": 1,
			},
			expected: "socket #1",
		},
		{
			path: path.Join(dir, "sysfs", "server", "sys"),
			name: "triple affinity to numa node #1 vs. #3",
			req: &request{
				memReq:    10000,
				memLim:    10000,
				memType:   memoryUnspec,
				isolate:   false,
				full:      3,
				container: &mockContainer{},
			},
			affinities: map[string]int32{
				"numa node #1": 3,
				"numa node #3": 1,
			},
			expected: "numa node #1",
		},
		{
			path: path.Join(dir, "sysfs", "server", "sys"),
			name: "double affinity to numa node #0,#3 vs. socket #1",
			req: &request{
				memReq:    10000,
				memLim:    10000,
				memType:   memoryUnspec,
				isolate:   false,
				full:      3,
				container: &mockContainer{},
			},
			affinities: map[string]int32{
				"numa node #0": 2,
				"numa node #3": 2,
				"socket #1":    1,
			},
			expected: "root",
		},
		{
			path: path.Join(dir, "sysfs", "server", "sys"),
			name: "equal affinity to numa node #0,#3 vs. socket #1",
			req: &request{
				memReq:    10000,
				memLim:    10000,
				memType:   memoryUnspec,
				isolate:   false,
				full:      3,
				container: &mockContainer{},
			},
			affinities: map[string]int32{
				"numa node #0": 1,
				"numa node #3": 1,
				"socket #1":    1,
			},
			expected: "root",
		},
		{
			path: path.Join(dir, "sysfs", "server", "sys"),
			name: "half the affinity to numa node #0,#3 vs. socket #1",
			req: &request{
				memReq:    10000,
				memLim:    10000,
				memType:   memoryUnspec,
				isolate:   false,
				full:      3,
				container: &mockContainer{},
			},
			affinities: map[string]int32{
				"numa node #0": 1,
				"numa node #3": 1,
				"socket #1":    2,
			},
			expected: "socket #1",
		},
	}

	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			sys, err := system.DiscoverSystemAt(tc.path)
			if err != nil {
				panic(err)
			}

			reserved, _ := resapi.ParseQuantity("750m")
			policyOptions := &policyapi.BackendOptions{
				Cache:  &mockCache{},
				System: sys,
				Reserved: policyapi.ConstraintSet{
					policyapi.DomainCPU: reserved,
				},
			}

			log.EnableDebug(true)
			policy := CreateMemtierPolicy(policyOptions).(*policy)
			log.EnableDebug(false)

			affinities := map[int]int32{}
			for name, weight := range tc.affinities {
				affinities[findNodeWithName(name, policy.pools).NodeID()] = weight
			}

			log.EnableDebug(true)
			scores, filteredPools := policy.sortPoolsByScore(tc.req, affinities)
			fmt.Printf("scores: %v, remaining pools: %v\n", scores, filteredPools)
			log.EnableDebug(false)

			if len(filteredPools) < 1 {
				t.Errorf("pool scoring failed to find any pools")
			}

			node := filteredPools[0]
			if node.Name() != tc.expected {
				t.Errorf("expected best pool %s, got %s", tc.expected, node.Name())
			}
		})
	}
}
