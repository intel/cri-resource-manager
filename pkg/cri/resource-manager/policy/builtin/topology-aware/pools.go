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
	"sort"

	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/kubernetes"
	system "github.com/intel/cri-resource-manager/pkg/sysfs"
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
	var pool Node

	request := newCPURequest(container)

	if container.GetNamespace() == kubernetes.NamespaceSystem {
		pool = p.root
	} else {
		affinity := p.calculatePoolAffinities(request.GetContainer())
		scores, pools := p.sortPoolsByScore(request, affinity)

		if log.DebugEnabled() {
			log.Debug("* node fitting for %s", request)
			for idx, n := range pools {
				log.Debug("    - #%d: node %s, score %s, affinity: %d",
					idx, n.Name(), scores[n.NodeID()], affinity[n.NodeID()])
			}
		}

		pool = pools[0]
	}

	cpus := pool.FreeCPU()
	grant, err := cpus.Allocate(request)
	if err != nil {
		return nil, policyError("failed to allocate %s from %s: %v", request, cpus, err)
	}

	p.allocations.CPU[container.GetCacheID()] = grant
	p.saveAllocations()

	return grant, nil
}

// Apply the result of allocation to the requesting container.
func (p *policy) applyGrant(grant CPUGrant) error {
	log.Debug("* applying grant %s", grant)

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
	if !node.IsRootNode() && opt.PinMemory {
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
		container.SetCpusetMems(mems)
	} else {
		log.Debug("  => not pinning memory, memory set is empty...")
	}

	return nil
}

// Release resources allocated by this grant.
func (p *policy) releasePool(container cache.Container) (CPUGrant, bool, error) {
	log.Debug("* releasing resources allocated to %s", container.PrettyName())

	grant, ok := p.allocations.CPU[container.GetCacheID()]
	if !ok {
		log.Debug("  => no grant found, nothing to do...")
		return nil, false, nil
	}

	log.Debug("  => releasing grant %s...", grant)

	pool := grant.GetNode()
	cpus := pool.FreeCPU()

	cpus.Release(grant)
	delete(p.allocations.CPU, container.GetCacheID())
	p.saveAllocations()

	return grant, true, nil
}

// Update shared allocations effected by agrant.
func (p *policy) updateSharedAllocations(grant CPUGrant) error {
	log.Debug("* updating shared allocations affected by %s", grant)

	for _, other := range p.allocations.CPU {
		if other.SharedPortion() == 0 {
			log.Debug("  => %s not affected (no shared portion)...", other)
			continue
		}

		if opt.PinCPU {
			shared := other.GetNode().FreeCPU().SharableCPUs().String()
			log.Debug("  => updating %s with shared CPUs of %s: %s...",
				other, other.GetNode().Name(), shared)
			other.GetContainer().SetCpusetCpus(shared)
		}
	}

	return nil
}

// Calculate pool affinities for the given container.
func (p *policy) calculatePoolAffinities(container cache.Container) map[int]int32 {
	log.Debug("=> calculating pool affinities...")

	result := make(map[int]int32, len(p.nodes))
	for id, w := range p.calculateContainerAffinity(container) {
		grant, ok := p.allocations.CPU[id]
		if !ok {
			continue
		}
		node := grant.GetNode()
		result[node.NodeID()] += w
	}

	return result
}

// Caculate affinity of this container (against all other containers).
func (p *policy) calculateContainerAffinity(container cache.Container) map[string]int32 {
	log.Debug("* calculating affinity for container %s...", container.PrettyName())

	ca := container.GetAffinity()

	result := make(map[string]int32)
	for _, a := range ca {
		for id, w := range p.cache.EvaluateAffinity(a) {
			result[id] += w
		}
	}

	log.Debug("  => affinity: %v", result)

	return result
}

// Score pools against the request and sort them by score.
func (p *policy) sortPoolsByScore(req CPURequest, aff map[int]int32) (map[int]CPUScore, []Node) {
	scores := make(map[int]CPUScore, p.nodeCnt)

	p.root.DepthFirst(func(n Node) error {
		scores[n.NodeID()] = n.GetScore(req)
		return nil
	})

	sort.Slice(p.pools, func(i, j int) bool {
		return p.compareScores(req, scores, aff, i, j)
	})

	return scores, p.pools
}

// Compare two pools by scores for allocation preference.
func (p *policy) compareScores(request CPURequest, scores map[int]CPUScore,
	affinity map[int]int32, i int, j int) bool {
	node1, node2 := p.pools[i], p.pools[j]
	depth1, depth2 := node1.RootDistance(), node2.RootDistance()
	id1, id2 := node1.NodeID(), node2.NodeID()
	score1, score2 := scores[id1], scores[id2]
	isolated1, shared1 := score1.IsolatedCapacity(), score1.SharedCapacity()
	isolated2, shared2 := score2.IsolatedCapacity(), score2.SharedCapacity()
	affinity1, affinity2 := affinity[id1], affinity[id2]

	//
	// Notes:
	//
	// Our scoring/score sorting algorithm is:
	//
	// 1) - insufficient isolated or shared capacity loses
	// 2) - if we have affinity, the higher affinity wins
	// 3) - if we have topology hints
	//       * better hint score wins
	//       * for a tie, prefer the lower node then the smaller id
	// 4) - if a node is lower in the tree it wins
	// 5) - for isolated allocations
	//       * more isolated capacity wins
	//       * for a tie, prefer the smaller id
	// 6) - for exclusive allocations
	//       * more slicable (shared) capacity wins
	//       * for a tie, prefer the smaller id
	// 7) - for shared-only allocations
	//       * fewer colocated containers win
	//       * for a tie prefer more shared capacity then the smaller id
	//

	// 1) a node with insufficient isolated or shared capacity loses
	switch {
	case isolated2 < 0 || shared2 < 0:
		return true
	case isolated1 < 0 || shared1 < 0:
		return false
	}

	// 2) higher affinity wins
	if affinity1 > affinity2 {
		return true
	}
	if affinity2 > affinity1 {
		return false
	}

	// 3) better topology hint score wins
	hScores1 := score1.HintScores()
	if len(hScores1) > 0 {
		hScores2 := score2.HintScores()
		hs1, nz1 := combineHintScores(hScores1)
		hs2, nz2 := combineHintScores(hScores2)

		if hs1 > hs2 {
			return true
		}
		if hs2 > hs1 {
			return false
		}

		if hs1 == 0 {
			if nz1 > nz2 {
				return true
			}
			if nz2 > nz1 {
				return false
			}
		}

		// for a tie, prefer lower nodes and smaller ids
		if hs1 == hs2 && nz1 == nz2 && (hs1 != 0 || nz1 != 0) {
			if depth1 > depth2 {
				return true
			}
			if depth1 < depth2 {
				return false
			}
			return id1 < id2
		}
	}

	// 4) a lower node wins
	if depth1 > depth2 {
		return true
	}
	if depth1 < depth2 {
		return false
	}

	// 5) more isolated capacity wins
	if request.Isolate() {
		if isolated1 > isolated2 {
			return true
		}
		if isolated2 > isolated1 {
			return false
		}
		return id1 < id2
	}

	// 6) more slicable shared capacity wins
	if request.FullCPUs() > 0 {
		if shared1 > shared2 {
			return true
		}
		if shared2 > shared1 {
			return false
		}

		return id1 < id2
	}

	// 7) fewer colocated containers win
	if score1.Colocated() < score2.Colocated() {
		return true
	}
	if score2.Colocated() < score1.Colocated() {
		return false
	}

	// more shared capacity wins
	if shared1 > shared2 {
		return true
	}
	if shared2 > shared1 {
		return false
	}

	// lower id wins
	return id1 < id2
}

// hintScores calculates combined full and zero-filtered hint scores.
func combineHintScores(scores map[string]float64) (float64, float64) {
	if len(scores) == 0 {
		return 0.0, 0.0
	}

	combined, filtered := 1.0, 0.0
	for _, score := range scores {
		combined *= score
		if score != 0.0 {
			if filtered == 0.0 {
				filtered = score
			} else {
				filtered *= score
			}
		}
	}
	return combined, filtered
}
