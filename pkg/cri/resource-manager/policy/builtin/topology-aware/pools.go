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
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	system "github.com/intel/cri-resource-manager/pkg/sysfs"
	"sort"
)

// buildPoolsByTopology builds a hierarchical tree of pools based on HW topology.
func (p *policy) buildPoolsByTopology() error {
	var n Node

	socketCnt := p.sys.SocketCount()
	nodeCnt := p.sys.NUMANodeCount()
	if nodeCnt < 2 {
		nodeCnt = 0
	}
	poolCnt := socketCnt + nodeCnt + map[bool]int{false: 0, true: 1}[socketCnt > 1]

	p.nodes = make(map[string]Node, poolCnt)
	p.pools = make([]Node, poolCnt)

	// create virtual root if necessary
	if socketCnt > 1 {
		p.root = p.NewVirtualNode("root", nilnode)
		p.nodes[p.root.Name()] = p.root
	}

	// create nodes for sockets
	sockets := make(map[system.ID]Node, socketCnt)
	for _, id := range p.sys.PackageIDs() {
		if socketCnt > 1 {
			n = p.NewSocketNode(id, p.root)
		} else {
			n = p.NewSocketNode(id, nilnode)
			p.root = n
		}
		p.nodes[n.Name()] = n
		sockets[id] = n
	}

	// create nodes for NUMA nodes
	if nodeCnt > 0 {
		for _, id := range p.sys.NodeIDs() {
			n = p.NewNumaNode(id, sockets[p.sys.Node(id).PackageID()])
			p.nodes[n.Name()] = n
		}
	}

	// enumerate nodes, calculate tree depth, discover node resource capacity
	p.root.DepthFirst(func(n Node) error {
		p.pools[p.nodeCnt] = n
		n.(*node).id = p.nodeCnt
		p.nodeCnt++

		if p.depth < n.(*node).depth {
			p.depth = n.(*node).depth
		}

		n.DiscoverCPU()
		n.DiscoverMemset()

		return nil
	})

	return nil
}

// Pick a pool and allocate resource from it to the container.
func (p *policy) allocatePool(container cache.Container) (CPUGrant, error) {
	log.Debug("* allocating resources for %s", container.GetCacheID())

	request := newCPURequest(container)
	scores, pools := p.sortPoolsByScore(request)

	if log.DebugEnabled() {
		for idx, n := range pools {
			log.Debug("* fitting %s: #%d: node %s, score %f",
				request.String(), idx, n.Name(), scores[n.NodeID()])
		}
	}

	pool := pools[0]
	cpus := pool.FreeCPU()

	grant, err := cpus.Allocate(request)
	if err != nil {
		return nil, policyError("failed to allocate %s from %s: %v",
			request.String(), cpus.String(), err)
	}

	p.allocations.CPU[container.GetCacheID()] = grant
	p.saveAllocations()

	return grant, nil
}

// Apply the result of allocation to the requesting container.
func (p *policy) applyGrant(grant CPUGrant) error {
	log.Debug("* applying grant %s", grant.String())

	container := grant.GetContainer()
	exclusive := grant.ExclusiveCPUs()
	shared := grant.SharedCPUs()
	portion := grant.SharedPortion()

	cpus := ""
	kind := ""
	if exclusive.IsEmpty() {
		cpus = shared.String()
		kind = "shared"
	} else {
		cpus = exclusive.Union(shared).String()
		kind = "exclusive"
		if portion > 0 {
			kind += "+shared"
		}
	}

	mems := ""
	node := grant.GetNode()
	if !node.IsRootNode() && opt.PinMem {
		mems = node.GetMemset().String()
	}

	if opt.PinCPU {
		if cpus != "" {
			log.Debug("  => pinning to (%s) cpuset %s", kind, cpus)
		} else {
			log.Debug("  => not pinning CPUs, allocated cpuset is empty...")
		}
		container.SetCpusetCpus(cpus)
		if exclusive.IsEmpty() {
			container.SetCPUShares(int64(cache.MilliCPUToShares(portion)))
		} else {
			// Notes:
			//   Hmm... I think setting CPU shares according to the normal formula
			//   can be dangerous when we do mixed allocations (both exclusive and
			//   shared CPUs assigned). If the exclusive cpuset is not isolated and
			//   there are other processes (unbeknown to us) running on some of the
			//   same exclusive CPU(s) with CPU shares not set by us, those processes
			//   can starve our containers with supposedly exclusive CPUs...
			//   There's not much we can do though... if we don't set the CPU shares
			//   then any process/thread in the container that might sched_setaffinity
			//   itself to the shared subset will not get properly weighted wrt. other
			//   processes sharing the same CPUs.
			//
			container.SetCPUShares(int64(cache.MilliCPUToShares(portion)))
		}
	}

	if mems != "" {
		log.Debug("  => pinning to memory %s", mems)
	} else {
		log.Debug("  => not pinning memory, memory set is empty...")
	}

	return nil
}

// Release resources allocated by this grant.
func (p *policy) releasePool(container cache.Container) (CPUGrant, bool, error) {
	log.Debug("* releasing resources allocated to %s", container.GetCacheID())

	grant, ok := p.allocations.CPU[container.GetCacheID()]
	if !ok {
		log.Debug("  => no grant found, nothing to do...")
		return nil, false, nil
	}

	log.Debug("  => releasing grant %s...", grant.String())

	pool := grant.GetNode()
	cpus := pool.FreeCPU()

	cpus.Release(grant)
	delete(p.allocations.CPU, container.GetCacheID())
	p.saveAllocations()

	return grant, true, nil
}

// Update shared allocations effected by agrant.
func (p *policy) updateSharedAllocations(grant CPUGrant) error {
	log.Debug("* updating shared allocations affected by %s", grant.String())

	for _, other := range p.allocations.CPU {
		if other.SharedPortion() == 0 {
			log.Debug("  => %s not affected (no shared portion)...", other.String())
			continue
		}

		if opt.PinCPU {
			shared := other.GetNode().FreeCPU().SharableCPUs().String()
			log.Debug("  => updating %s with shared CPUs of %s: %s...",
				other.String(), other.GetNode().Name(), shared)
			other.GetContainer().SetCpusetCpus(shared)
		}
	}

	return nil
}

// Score pools against the request and sort them by score.
func (p *policy) sortPoolsByScore(request CPURequest) (map[int]float64, []Node) {
	scores := make(map[int]float64, p.nodeCnt)

	p.root.DepthFirst(func(n Node) error {
		scores[n.NodeID()] = n.Score(request)
		return nil
	})

	sort.Slice(p.pools, func(i, j int) bool {
		return p.comparePools(request, scores, i, j)
	})

	return scores, p.pools
}

// Compare two pools by scores for allocation preference.
func (p *policy) comparePools(request CPURequest, scores map[int]float64, i int, j int) bool {
	n1, n2 := p.pools[i], p.pools[j]
	d1, d2 := n1.RootDistance(), n2.RootDistance()
	id1, id2 := n1.NodeID(), n2.NodeID()
	s1, s2 := scores[id1], scores[id2]

	switch {
	case s1 > s2:
		return true
	case s1 < s2:
		return false

	case d1 == d2:
		return id1 < id2

	case len(request.GetContainer().GetTopologyHints()) > 0:
		return d1 > d2
	default:
		return d1 < d2
	}
}
