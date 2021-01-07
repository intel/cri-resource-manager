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

package memtier

import (
	"math"
	"sort"

	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"

	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/kubernetes"
	system "github.com/intel/cri-resource-manager/pkg/sysfs"
)

// buildPoolsByTopology builds a hierarchical tree of pools based on HW topology.
func (p *policy) buildPoolsByTopology() error {
	if err := p.checkHWTopology(); err != nil {
		return err
	}

	// Notes:
	//   we never create pool nodes for PMEM-only NUMA nodes (as these
	//   are always without any close/local set of CPUs). We instead
	//   assign the PMEM memory of such a node to one of the closest
	//   normal (DRAM) pool NUMA nodes.
	//
	//   Akin to omitting lone dies from their parent, we omit from the
	//   pool tree each NUMA node that would end up being the only child
	//   of its parent (a die or a socket pool node). Resources for each
	//   such node will get discovered by and assigned to the would be
	//   parent which is now a leaf (die or socket) node in the tree.
	//
	//   The PMEM memory of (omitted) PMEM-only nodes is assigned
	//   to one of the closest normal (DRAM) NUMA nodes. This right
	//   assignment has already been calculated by assignPMEMNodes().
	//   However, making the corresponding assignment in the pool
	//   tree is a bit more involved as the DRAM node where a PMEM
	//   node has been assigned to might have gotten omitted from the
	//   tree if it ended up being a lone child. We use the recorded
	//   per NUMA node surrogates to find both if and where resources
	//   of omitted DRAM NUMA nodes need to get assigned to, and also
	//   where PMEM NUMA node resources need to get assigned to.

	log.Debug("building topology pool tree...")

	p.nodes = make(map[string]Node)

	// create a virtual root node, if we have a multi-socket system
	if p.sys.SocketCount() > 1 {
		p.root = p.NewVirtualNode("root", nilnode)
		p.nodes[p.root.Name()] = p.root
		log.Debug("  + created pool (root) %q", p.root.Name())
	} else {
		log.Debug("  - omitted pool virtual root (single-socket system)")
	}

	// create socket nodes, for a single-socket system set the only socket as the root
	sockets := map[system.ID]Node{}
	for _, socketID := range p.sys.PackageIDs() {
		var socket Node

		if p.root != nil {
			socket = p.NewSocketNode(socketID, p.root)
			log.Debug("    + created pool %q", socket.Parent().Name()+"/"+socket.Name())
		} else {
			socket = p.NewSocketNode(socketID, nilnode)
			p.root = socket
			log.Debug("    + created pool %q (as root)", socket.Name())
		}

		p.nodes[socket.Name()] = socket
		sockets[socketID] = socket
	}

	// create dies for every socket, but only if we have more than one die in the socket
	numaDies := map[system.ID]Node{} // created die Nodes per NUMA node id
	for socketID, socket := range sockets {
		dieIDs := p.sys.Package(socketID).DieIDs()
		if len(dieIDs) < 2 {
			log.Debug("      - omitted pool %q (die count: %d)", socket.Name()+"/die #0",
				len(dieIDs))
			continue
		}
		for _, dieID := range dieIDs {
			die := p.NewDieNode(dieID, socket)
			p.nodes[die.Name()] = die
			for _, numaNodeID := range p.sys.Package(socketID).DieNodeIDs(dieID) {
				numaDies[numaNodeID] = die
			}
			log.Debug("      + created pool %q", die.Parent().Name()+"/"+die.Name())
		}
	}

	// create pool nodes for NUMA nodes
	pmemNodes := map[system.ID]system.Node{} // collected PMEM-only nodes
	dramNodes := map[system.ID]system.Node{} // collected DRAM-only nodes
	numaSurrogates := map[system.ID]Node{}   // surrogate leaf nodes for omitted NUMA nodes
	for _, numaNodeID := range p.sys.NodeIDs() {
		var numaNode Node

		numaSysNode := p.sys.Node(numaNodeID)
		switch numaSysNode.GetMemoryType() {
		case system.MemoryTypeDRAM:
			dramNodes[numaNodeID] = numaSysNode
		case system.MemoryTypePMEM:
			pmemNodes[numaNodeID] = numaSysNode
			log.Debug("        - omitted pool \"NUMA node #%d\": PMEM node", numaNodeID)
			continue // don't create pool, will assign to a closest DRAM node
		default:
			log.Warn("        - ignored pool \"NUMA node #%d\": unhandled memory type %v",
				numaNodeID, numaSysNode.GetMemoryType())
			continue
		}

		//
		// Notes:
		//   We omit inserting NUMA nodes (as leaf nodes) in the tree, if that NUMA node
		//   would be the only child of its parent. In this case, we record the would-be
		//   parent as the surrogate for the NUMA node. This surrogate will get assigned
		//   any closest PMEM-only NUMA node that the original one would have received.
		//

		if die, ok := numaDies[numaNodeID]; ok {
			if p.parentNumaNodeCountWithCPUs(numaSysNode) < 2 {
				numaSurrogates[numaNodeID] = die
				log.Debug("        - omitted pool \"NUMA node #%d\": using surrogate %q",
					numaNodeID, numaSurrogates[numaNodeID].Name())
				continue
			}
			numaNode = p.NewNumaNode(numaNodeID, die)
		} else {
			socket := sockets[p.sys.Node(numaNodeID).PackageID()]
			if p.parentNumaNodeCountWithCPUs(numaSysNode) < 2 {
				numaSurrogates[numaNodeID] = socket
				log.Debug("        - omitted pool \"NUMA node #%d\": using surrogate %q",
					numaNodeID, numaSurrogates[numaNodeID].Name())
				continue
			}
			numaNode = p.NewNumaNode(numaNodeID, socket)
		}

		p.nodes[numaNode.Name()] = numaNode
		numaSurrogates[numaNodeID] = numaNode
		log.Debug("        + created pool %q", numaNode.Parent().Name()+"/"+numaNode.Name())
	}

	// set up assignment of PMEM and DRAM node resources to pool nodes and surrogates
	assigned := p.assignNUMANodes(numaSurrogates, pmemNodes, dramNodes)
	log.Debug("NUMA node to pool assignment:")
	for n, numaNodeIDs := range assigned {
		log.Debug("  pool %q: NUMA nodes #%s", n.Name(), system.NewIDSet(numaNodeIDs...))
	}

	// enumerate pools, calculate depth, discover resource capacity, assign NUMA nodes
	p.pools = make([]Node, 0)
	p.root.DepthFirst(func(n Node) error {
		p.pools = append(p.pools, n)
		n.(*node).id = p.nodeCnt
		p.nodeCnt++

		if p.depth < n.(*node).depth {
			p.depth = n.(*node).depth
		}

		n.DiscoverSupply(assigned[n.(*node).self.node])
		delete(assigned, n.(*node).self.node)

		return nil
	})

	// make sure all PMEM nodes got assigned
	if len(assigned) > 0 {
		for node, pmem := range assigned {
			log.Error("failed to assign PMEM NUMA nodes #%s (to NUMA node/surrogate %s %v)",
				system.NewIDSet(pmem...), node.Name(), node)
		}
		log.Fatal("internal error: unassigned PMEM NUMA nodes remaining")
	}

	p.root.Dump("<pool-setup>")

	return nil
}

// parentNumaNodeCountWithCPUs returns the number of CPU-ful NUMA nodes in the parent die/socket.
func (p *policy) parentNumaNodeCountWithCPUs(numaNode system.Node) int {
	socketID := numaNode.PackageID()
	socket := p.sys.Package(socketID)
	count := 0
	for _, nodeID := range socket.DieNodeIDs(numaNode.DieID()) {
		node := p.sys.Node(nodeID)
		if !node.CPUSet().IsEmpty() {
			count++
		}
	}
	return count
}

// assignNUMANodes assigns each PMEM node to one of the closest DRAM nodes
func (p *policy) assignNUMANodes(surrogates map[system.ID]Node, pmem, dram map[system.ID]system.Node) map[Node][]system.ID {
	// collect the closest DRAM NUMA nodes (sorted by system.ID) for each PMEM NUMA node.
	closest := map[system.ID][]system.ID{}
	for pmemID := range pmem {
		var min []system.ID
		for dramID := range dram {
			if len(min) < 1 {
				min = []system.ID{dramID}
			} else {
				minDist := p.sys.NodeDistance(pmemID, min[0])
				newDist := p.sys.NodeDistance(pmemID, dramID)
				switch {
				case newDist == minDist:
					min = append(min, dramID)
				case newDist < minDist:
					min = []system.ID{dramID}
				}
			}
		}
		sort.Slice(min, func(i, j int) bool { return min[i] < min[j] })
		closest[pmemID] = min
	}

	assigned := map[Node][]system.ID{}

	// assign each PMEM node to the closest DRAM surrogate with the least PMEM assigned
	for pmemID, min := range closest {
		var taker Node
		var takerID system.ID

		for _, dramID := range min {
			if taker == nil {
				taker = surrogates[dramID]
				takerID = dramID
			} else {
				if len(assigned[taker]) > len(assigned[surrogates[dramID]]) {
					taker = surrogates[dramID]
					takerID = dramID
				}
			}
		}
		if taker == nil {
			log.Panic("failed to assign CPU-less PMEM node #%d to any surrogate", pmemID)
		}

		assigned[taker] = append(assigned[taker], pmemID)
		log.Debug("        + PMEM node #%d assigned to %s with distance %v", pmemID, taker.Name(),
			p.sys.NodeDistance(pmemID, takerID))
	}

	// assign each DRAM node to its own surrogate (can be the DRAM node itself)
	for dramID := range dram {
		taker := surrogates[dramID]
		assigned[taker] = append([]system.ID{dramID}, assigned[taker]...)
		log.Debug("        + DRAM node #%d assigned to %s", dramID, taker.Name())
	}

	return assigned
}

// checkHWTopology verifies our otherwise implicit assumptions about the HW.
func (p *policy) checkHWTopology() error {
	// NUMA nodes (memory controllers) should not be shared by multiple sockets.
	socketNodes := map[system.ID]cpuset.CPUSet{}
	for _, socketID := range p.sys.PackageIDs() {
		pkg := p.sys.Package(socketID)
		socketNodes[socketID] = system.NewIDSet(pkg.NodeIDs()...).CPUSet()
	}
	for id1, nodes1 := range socketNodes {
		for id2, nodes2 := range socketNodes {
			if id1 == id2 {
				continue
			}
			if shared := nodes1.Intersection(nodes2); !shared.IsEmpty() {
				log.Error("can't handle HW topology: sockets #%v, #%v share NUMA node(s) #%s",
					id1, id2, shared.String())
				return policyError("unhandled HW topology: sockets #%v, #%v share NUMA node(s) #%s",
					id1, id2, shared.String())
			}
		}
	}

	// NUMA nodes (memory controllers) should not be shared by multiple dies.
	for _, socketID := range p.sys.PackageIDs() {
		pkg := p.sys.Package(socketID)
		for _, id1 := range pkg.DieIDs() {
			nodes1 := system.NewIDSet(pkg.DieNodeIDs(id1)...)
			for _, id2 := range pkg.DieIDs() {
				if id1 == id2 {
					continue
				}
				nodes2 := system.NewIDSet(pkg.DieNodeIDs(id2)...)
				if shared := nodes1.CPUSet().Intersection(nodes2.CPUSet()); !shared.IsEmpty() {
					log.Error("can't handle HW topology: "+
						"socket #%v, dies #%v,%v share NUMA node(s) #%s",
						socketID, id1, id2, shared.String())
					return policyError("unhandled HW topology: "+
						"socket #%v, dies #%v,#%v share NUMA node(s) #%s",
						socketID, id1, id2, shared.String())
				}
			}
		}
	}

	// NUMA distance matrix should be symmetric.
	for _, from := range p.sys.NodeIDs() {
		for _, to := range p.sys.NodeIDs() {
			d1 := p.sys.NodeDistance(from, to)
			d2 := p.sys.NodeDistance(to, from)
			if d1 != d2 {
				log.Error("asymmetric NUMA distance (#%d, #%d): %d != %d",
					from, to, d1, d2)
				return policyError("asymmetric NUMA distance (#%d, #%d): %d != %d",
					from, to, d1, d2)
			}
		}
	}

	return nil
}

// Pick a pool and allocate resource from it to the container.
func (p *policy) allocatePool(container cache.Container, poolHint string) (Grant, error) {
	var pool Node

	request := newRequest(container)

	if p.root.FreeSupply().ReservedCPUs().IsEmpty() && request.CPUType() == cpuReserved {
		// Fallback to allocating reserved CPUs from the shared pool
		// if there are no reserved CPUs.
		request.SetCPUType(cpuNormal)
	}

	// Assumption: in the beginning the CPUs and memory will be allocated from
	// the same pool. This assumption can be relaxed later, requires separate
	// (but connected) scoring of memory and CPU.

	if request.CPUType() == cpuNormal && container.GetNamespace() == kubernetes.NamespaceSystem {
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

		if len(pools) == 0 {
			return nil, policyError("no suitable pool found for container %s",
				container.PrettyName())
		}

		if poolHint != "" {
			for idx, p := range pools {
				if p.Name() == poolHint {
					log.Debug("* using hinted pool %q (#%d best fit)", poolHint, idx+1)
					pool = p
					break
				}
			}
			if pool == nil {
				log.Debug("* cannot use hinted pool %q", poolHint)
			}
		}

		if pool == nil {
			pool = pools[0]
		}
	}

	supply := pool.FreeSupply()
	grant, err := supply.Allocate(request)
	if err != nil {
		return nil, policyError("failed to allocate %s from %s: %v",
			request, supply.DumpAllocatable(), err)
	}

	log.Debug("allocated req '%s' to memory node '%s' (memset %s,%s)", container.GetCacheID(), grant.GetMemoryNode().Name(), grant.GetMemoryNode().GetMemset(memoryDRAM), grant.GetMemoryNode().GetMemset(memoryPMEM))

	// In case the workload is assigned to a memory node with multiple
	// child nodes, there is no guarantee that the workload will
	// allocate memory "nicely". Instead we'll have to make the
	// conservative assumption that the memory will all be allocated
	// from one single node, and that node can be any of the child
	// nodes in the system. Thus, we'll need to reserve the memory
	// from all child nodes, and move the containers already
	// assigned to the child nodes upwards in the topology tree, if
	// they no longer fit to the child node that they are in. In
	// other words, they'll need to have a wider range of memory
	// node options in order to fit to memory.
	//
	//
	// Example:
	//
	// Workload 1 and Workload 2 are running on the leaf nodes:
	//
	//                    +----------------+
	//                    |Total mem: 4G   |
	//                    |Total CPUs: 4   |            Workload 1:
	//                    |Reserved:       |
	//                    |  1.5G          |             1G mem
	//                    |                |
	//                    |                |            Workload 2:
	//                    |                |
	//                    +----------------+             0.5G mem
	//                       /          \
	//                      /            \
	//                     /              \
	//                    /                \
	//                   /                  \
	//                  /                    \
	//                 /                      \
	//                /                        \
	//  +----------------+                  +----------------+
	//  |Total mem: 2G   |                  |Total mem: 2G   |
	//  |Total CPUs: 2   |                  |Total CPUs: 2   |
	//  |Reserved:       |                  |Reserved:       |
	//  |  1G            |                  |  0.5G          |
	//  |                |                  |                |
	//  |                |                  |                |
	//  |     * WL 1     |                  |     * WL 2     |
	//  +----------------+                  +----------------+
	//
	//
	// Then Workload 3 comes in and is assigned to the root node. Memory
	// reservations are done on the leaf nodes:
	//
	//                    +----------------+
	//                    |Total mem: 4G   |
	//                    |Total CPUs: 4   |            Workload 1:
	//                    |Reserved:       |
	//                    |  3G            |             1G mem
	//                    |                |
	//                    |                |            Workload 2:
	//                    |  * WL 3        |
	//                    +----------------+             0.5G mem
	//                       /          \
	//                      /            \              Workload 3:
	//                     /              \
	//                    /                \             1.5G mem
	//                   /                  \
	//                  /                    \
	//                 /                      \
	//                /                        \
	//  +----------------+                  +----------------+
	//  |Total mem: 2G   |                  |Total mem: 2G   |
	//  |Total CPUs: 2   |                  |Total CPUs: 2   |
	//  |Reserved:       |                  |Reserved:       |
	//  |  2.5G          |                  |  2G            |
	//  |                |                  |                |
	//  |                |                  |                |
	//  |     * WL 1     |                  |     * WL 2     |
	//  +----------------+                  +----------------+
	//
	//
	// Workload 1 no longer fits to the leaf node, because the total
	// reservation from the leaf node is over the memory maximum.
	// Thus, it's moved upwards in the tree to the root node. Memory
	// resevations are again updated accordingly:
	//
	//                    +----------------+
	//                    |Total mem: 4G   |
	//                    |Total CPUs: 4   |            Workload 1:
	//                    |Reserved:       |
	//                    |  3G            |             1G mem
	//                    |                |
	//                    |  * WL 1        |            Workload 2:
	//                    |  * WL 3        |
	//                    +----------------+             0.5G mem
	//                       /          \
	//                      /            \              Workload 3:
	//                     /              \
	//                    /                \             1.5G mem
	//                   /                  \
	//                  /                    \
	//                 /                      \
	//                /                        \
	//  +----------------+                  +----------------+
	//  |Total mem: 2G   |                  |Total mem: 2G   |
	//  |Total CPUs: 2   |                  |Total CPUs: 2   |
	//  |Reserved:       |                  |Reserved:       |
	//  |  2.5G          |                  |  3G            |
	//  |                |                  |                |
	//  |                |                  |                |
	//  |                |                  |     * WL 2     |
	//  +----------------+                  +----------------+
	//
	//
	// Now Workload 2 doesn't fit to the leaf node either. It's also moved
	// to the root node:
	//
	//                    +----------------+
	//                    |Total mem: 4G   |
	//                    |Total CPUs: 4   |            Workload 1:
	//                    |Reserved:       |
	//                    |  3G            |             1G mem
	//                    |  * WL 2        |
	//                    |  * WL 1        |            Workload 2:
	//                    |  * WL 3        |
	//                    +----------------+             0.5G mem
	//                       /          \
	//                      /            \              Workload 3:
	//                     /              \
	//                    /                \             1.5G mem
	//                   /                  \
	//                  /                    \
	//                 /                      \
	//                /                        \
	//  +----------------+                  +----------------+
	//  |Total mem: 2G   |                  |Total mem: 2G   |
	//  |Total CPUs: 2   |                  |Total CPUs: 2   |
	//  |Reserved:       |                  |Reserved:       |
	//  |  3G            |                  |  3G            |
	//  |                |                  |                |
	//  |                |                  |                |
	//  |                |                  |                |
	//  +----------------+                  +----------------+
	//

	// We need to analyze all existing containers which are a subset of current grant.
	memset := grant.GetMemoryNode().GetMemset(grant.MemoryType())

	// Add an extra memory reservation to all subnodes.
	// TODO: no need to do any of this if no memory request
	grant.UpdateExtraMemoryReservation()

	// See how much memory reservations the workloads on the
	// nodes up from this one cause to the node. We only need to
	// analyze the workloads up until this node, because it's
	// guaranteed that the subtree can hold the workloads.

	// If it turns out that the current workloads no longer fit
	// to the node with the reservations from nodes from above
	// in the tree, move all nodes upward. Note that this
	// creates a reservation of the same size to the node, so in
	// effect the node has to be empty of its "own" workloads.
	// In this case move all the workloads one level up in the tree.

	changed := true
	for changed {
		changed = false
		for _, oldGrant := range p.allocations.grants {
			oldMemset := oldGrant.GetMemoryNode().GetMemset(grant.MemoryType())
			if oldMemset.Size() < memset.Size() && memset.Has(oldMemset.Members()...) {
				changed, err = oldGrant.ExpandMemset()
				if err != nil {
					return nil, err
				}
				if changed {
					log.Debug("* moved container %s upward to node %s to guarantee memory", oldGrant.GetContainer().GetCacheID(), oldGrant.GetMemoryNode().Name())
					break
				}
			}
		}
	}

	p.allocations.grants[container.GetCacheID()] = grant

	p.saveAllocations()

	return grant, nil
}

// Apply the result of allocation to the requesting container.
func (p *policy) applyGrant(grant Grant) {
	log.Debug("* applying grant %s", grant)

	container := grant.GetContainer()
	cpuType := grant.CPUType()
	exclusive := grant.ExclusiveCPUs()
	reserved := grant.ReservedCPUs()
	shared := grant.SharedCPUs()
	cpuPortion := grant.SharedPortion()

	cpus := ""
	kind := ""
	if cpuType == cpuNormal {
		if exclusive.IsEmpty() {
			cpus = shared.String()
			kind = "shared"
		} else {
			kind = "exclusive"
			if cpuPortion > 0 {
				kind += "+shared"
				cpus = exclusive.Union(shared).String()
			} else {
				cpus = exclusive.String()
			}
		}
	} else if cpuType == cpuReserved {
		kind = "reserved"
		cpus = reserved.String()
		cpuPortion = grant.ReservedPortion()
	} else {
		log.Debug("unsupported granted cpuType %s", cpuType)
		return
	}

	mems := ""
	node := grant.GetMemoryNode()
	if !node.IsRootNode() && opt.PinMemory {
		mems = grant.Memset().String()
	}

	if opt.PinCPU {
		if cpus != "" {
			log.Debug("  => pinning to (%s) cpuset %s", kind, cpus)
		} else {
			log.Debug("  => not pinning CPUs, allocated cpuset is empty...")
		}
		container.SetCpusetCpus(cpus)
		if exclusive.IsEmpty() {
			container.SetCPUShares(int64(cache.MilliCPUToShares(cpuPortion)))
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
			container.SetCPUShares(int64(cache.MilliCPUToShares(cpuPortion)))
		}
	}

	if mems != "" {
		log.Debug("  => pinning to memory %s", mems)
		container.SetCpusetMems(mems)
		p.setDemotionPreferences(container, grant)
	} else {
		log.Debug("  => not pinning memory, memory set is empty...")
	}
}

// Release resources allocated by this grant.
func (p *policy) releasePool(container cache.Container) (Grant, bool) {
	log.Debug("* releasing resources allocated to %s", container.PrettyName())

	grant, ok := p.allocations.grants[container.GetCacheID()]
	if !ok {
		log.Debug("  => no grant found, nothing to do...")
		return nil, false
	}

	log.Debug("  => releasing grant %s...", grant)

	// Remove the grant from all supplys it uses.
	grant.Release()

	delete(p.allocations.grants, container.GetCacheID())
	p.saveAllocations()

	return grant, true
}

// Update shared allocations effected by agrant.
func (p *policy) updateSharedAllocations(grant *Grant) {
	if grant != nil {
		log.Debug("* updating shared allocations affected by %s", (*grant).String())
	} else {
		log.Debug("* updating shared allocations")
	}

	if grant.CPUType() == cpuReserved {
		log.Debug("  this grant uses reserved CPUs, does not affect shared allocations")
		return
	}

	for _, other := range p.allocations.grants {
		if grant != nil {
			if other.GetContainer().GetCacheID() == (*grant).GetContainer().GetCacheID() {
				continue
			}
		}

		if other.CPUType() == cpuReserved {
			log.Debug("  => %s not affected (only reserved CPUs)...", other)
			continue
		}

		if other.SharedPortion() == 0 && !other.ExclusiveCPUs().IsEmpty() {
			log.Debug("  => %s not affected (only exclusive CPUs)...", other)
			continue
		}

		if opt.PinCPU {
			shared := other.GetCPUNode().FreeSupply().SharableCPUs()
			exclusive := other.ExclusiveCPUs()
			if exclusive.IsEmpty() {
				log.Debug("  => updating %s with shared CPUs of %s: %s...",
					other, other.GetCPUNode().Name(), shared.String())
				other.GetContainer().SetCpusetCpus(shared.String())
			} else {
				log.Debug("  => updating %s with exclusive+shared CPUs of %s: %s+%s...",
					other, other.GetCPUNode().Name(), exclusive.String(), shared.String())
				other.GetContainer().SetCpusetCpus(exclusive.Union(shared).String())
			}
		}
	}
}

// setDemotionPreferences sets the dynamic demotion preferences a container.
func (p *policy) setDemotionPreferences(c cache.Container, g Grant) {
	log.Debug("%s: setting demotion preferences...", c.PrettyName())

	// System containers should not be demoted.
	if c.GetNamespace() == kubernetes.NamespaceSystem {
		c.SetPageMigration(nil)
		return
	}

	memType := g.GetMemoryNode().GetMemoryType()
	if memType&memoryDRAM == 0 || memType&memoryPMEM == 0 {
		c.SetPageMigration(nil)
		return
	}

	dram := g.GetMemoryNode().GetMemset(memoryDRAM)
	pmem := g.GetMemoryNode().GetMemset(memoryPMEM)

	log.Debug("%s: eligible for demotion from %s to %s NUMA node(s)",
		c.PrettyName(), dram, pmem)

	c.SetPageMigration(&cache.PageMigrate{
		SourceNodes: dram,
		TargetNodes: pmem,
	})
}

// addImplicitAffinities adds our set of policy-specific implicit affinities.
func (p *policy) addImplicitAffinities() error {
	return p.cache.AddImplicitAffinities(map[string]*cache.ImplicitAffinity{
		PolicyName + ":AVX512-pull": {
			Eligible: func(c cache.Container) bool {
				_, ok := c.GetTag(cache.TagAVX512)
				return ok
			},
			Affinity: cache.GlobalAffinity("tags/"+cache.TagAVX512, 5),
		},
		PolicyName + ":AVX512-push": {
			Eligible: func(c cache.Container) bool {
				_, ok := c.GetTag(cache.TagAVX512)
				return !ok
			},
			Affinity: cache.GlobalAntiAffinity("tags/"+cache.TagAVX512, 5),
		},
	})
}

// Calculate pool affinities for the given container.
func (p *policy) calculatePoolAffinities(container cache.Container) map[int]int32 {
	log.Debug("=> calculating pool affinities...")

	result := make(map[int]int32, len(p.nodes))
	for id, w := range p.calculateContainerAffinity(container) {
		grant, ok := p.allocations.grants[id]
		if !ok {
			continue
		}
		node := grant.GetCPUNode()
		result[node.NodeID()] += w

		// TODO: calculate affinity for memory here too?
	}

	return result
}

// Calculate affinity of this container (against all other containers).
func (p *policy) calculateContainerAffinity(container cache.Container) map[string]int32 {
	log.Debug("* calculating affinity for container %s...", container.PrettyName())

	ca := container.GetAffinity()

	result := make(map[string]int32)
	for _, a := range ca {
		for id, w := range p.cache.EvaluateAffinity(a) {
			result[id] += w
		}
	}

	// self-affinity does not make sense, so remove any
	delete(result, container.GetCacheID())

	log.Debug("  => affinity: %v", result)

	return result
}

func (p *policy) filterInsufficientResources(req Request, originals []Node) []Node {
	filtered := make([]Node, 0)

	for _, node := range originals {
		// TODO: Need to filter based on the memory demotion scheme here. For example, if the request is
		// of memory type memoryAll, the memory used might be PMEM until it's full and after that DRAM. If
		// it's DRAM, amount of PMEM should not be considered and so on. How to find this out in a live
		// system?

		supply := node.FreeSupply()
		memType := req.MemoryType()

		if memType == memoryUnspec {
			// The algorithm for handling unspecified memory allocations is the same as for handling a request
			// with memory type all.
			memType = memoryAll
		}
		bitsToFit := req.MemAmountToAllocate()

		if memType&memoryPMEM != 0 {
			if supply.MemoryLimit()[memoryPMEM]-supply.ExtraMemoryReservation(memoryPMEM) >= bitsToFit {
				filtered = append(filtered, node)
				continue
			} else {
				// Can't go negative
				bitsToFit -= supply.MemoryLimit()[memoryPMEM] - supply.ExtraMemoryReservation(memoryPMEM)
			}
		}
		if req.ColdStart() > 0 {
			// For a "cold start" request, the memory request must fit completely in the PMEM. So reject the node.
			continue
		}
		if memType&memoryDRAM != 0 {
			if supply.MemoryLimit()[memoryDRAM]-supply.ExtraMemoryReservation(memoryDRAM) >= bitsToFit {
				filtered = append(filtered, node)
				continue
			} else {
				bitsToFit -= supply.MemoryLimit()[memoryDRAM] - supply.ExtraMemoryReservation(memoryDRAM)
			}
		}
		if memType&memoryHBM != 0 {
			if supply.MemoryLimit()[memoryHBM]-supply.ExtraMemoryReservation(memoryHBM) >= bitsToFit {
				filtered = append(filtered, node)
			}
		}
	}

	return filtered
}

// Score pools against the request and sort them by score.
func (p *policy) sortPoolsByScore(req Request, aff map[int]int32) (map[int]Score, []Node) {
	scores := make(map[int]Score, p.nodeCnt)

	p.root.DepthFirst(func(n Node) error {
		scores[n.NodeID()] = n.GetScore(req)
		return nil
	})

	// Filter out pools which don't have enough uncompressible resources
	// (memory) to satisfy the request.
	filteredPools := p.filterInsufficientResources(req, p.pools)

	sort.Slice(filteredPools, func(i, j int) bool {
		return p.compareScores(req, filteredPools, scores, aff, i, j)
	})

	return scores, filteredPools
}

// Compare two pools by scores for allocation preference.
func (p *policy) compareScores(request Request, pools []Node, scores map[int]Score,
	affinity map[int]int32, i int, j int) bool {
	node1, node2 := pools[i], pools[j]
	depth1, depth2 := node1.RootDistance(), node2.RootDistance()
	id1, id2 := node1.NodeID(), node2.NodeID()
	score1, score2 := scores[id1], scores[id2]
	cpuType := request.CPUType()
	isolated1, reserved1, shared1 := score1.IsolatedCapacity(), score1.ReservedCapacity(), score1.SharedCapacity()
	isolated2, reserved2, shared2 := score2.IsolatedCapacity(), score2.ReservedCapacity(), score2.SharedCapacity()
	a1 := affinityScore(affinity, node1)
	a2 := affinityScore(affinity, node2)

	log.Debug("comparing scores for %s and %s", node1.Name(), node2.Name())
	log.Debug("  %s: %s, affinity score %f", node1.Name(), score1.String(), a1)
	log.Debug("  %s: %s, affinity score %f", node2.Name(), score2.String(), a2)

	//
	// Notes:
	//
	// Our scoring/score sorting algorithm is:
	//
	// 1) - insufficient isolated, reserved or shared capacity loses
	// 2) - if we have affinity, the higher affinity score wins
	// 3) - if only one node matches the memory type request, it wins
	// 4) - if we have topology hints
	//       * better hint score wins
	//       * for a tie, prefer the lower node then the smaller id
	// 5) - if a node is lower in the tree it wins
	// 6) - for reserved allocations
	//       * more unallocated reserved capacity per colocated container wins
	// 7) - for (non-reserved) isolated allocations
	//       * more isolated capacity wins
	//       * for a tie, prefer the smaller id
	// 8) - for (non-reserved) exclusive allocations
	//       * more slicable (shared) capacity wins
	//       * for a tie, prefer the smaller id
	// 9) - for (non-reserved) shared-only allocations
	//       * fewer colocated containers win
	//       * for a tie prefer more shared capacity
	// 10) - lower id wins
	//
	// Before this comparison is reached, nodes with insufficient uncompressible resources
	// (memory) have been filtered out.

	// 1) a node with insufficient isolated or shared capacity loses
	switch {
	case cpuType == cpuNormal && ((isolated2 < 0 && isolated1 >= 0) || (shared2 <= 0 && shared1 > 0)):
		log.Debug("  => %s loses, insufficent isolated or shared", node2.Name())
		return true
	case cpuType == cpuNormal && ((isolated1 < 0 && isolated2 >= 0) || (shared1 <= 0 && shared2 > 0)):
		log.Debug("  => %s loses, insufficent isolated or shared", node1.Name())
		return false
	case cpuType == cpuReserved && reserved2 < 0 && reserved1 >= 0:
		log.Debug("  => %s loses, insufficent reserved", node2.Name())
		return true
	case cpuType == cpuReserved && reserved1 < 0 && reserved2 >= 0:
		log.Debug("  => %s loses, insufficent reserved", node1.Name())
		return false
	}

	log.Debug("  - isolated/reserved/shared insufficiency is a TIE")

	// 2) higher affinity score wins
	if a1 > a2 {
		log.Debug("  => %s loses on affinity", node2.Name())
		return true
	}
	if a2 > a1 {
		log.Debug("  => %s loses on affinity", node1.Name())
		return false
	}

	log.Debug("  - affinity is a TIE")

	// 3) matching memory type wins
	if reqType := request.MemoryType(); reqType != memoryUnspec {
		if node1.HasMemoryType(reqType) && !node2.HasMemoryType(reqType) {
			log.Debug("  => %s WINS on memory type", node1.Name())
			return true
		}
		if !node1.HasMemoryType(reqType) && node2.HasMemoryType(reqType) {
			log.Debug("  => %s WINS on memory type", node2.Name())
			return false
		}

		log.Debug("  - memory type is a TIE")
	}

	// 4) better topology hint score wins
	hScores1 := score1.HintScores()
	if len(hScores1) > 0 {
		hScores2 := score2.HintScores()
		hs1, nz1 := combineHintScores(hScores1)
		hs2, nz2 := combineHintScores(hScores2)

		if hs1 > hs2 {
			log.Debug("  => %s WINS on hints", node1.Name())
			return true
		}
		if hs2 > hs1 {
			log.Debug("  => %s WINS on hints", node2.Name())
			return false
		}

		log.Debug("  - hints are a TIE")

		if hs1 == 0 {
			if nz1 > nz2 {
				log.Debug("  => %s WINS on non-zero hints", node1.Name())
				return true
			}
			if nz2 > nz1 {
				log.Debug("  => %s WINS on non-zero hints", node2.Name())
				return false
			}

			log.Debug("  - non-zero hints are a TIE")
		}

		// for a tie, prefer lower nodes and smaller ids
		if hs1 == hs2 && nz1 == nz2 && (hs1 != 0 || nz1 != 0) {
			if depth1 > depth2 {
				log.Debug("  => %s WINS as it is lower", node1.Name())
				return true
			}
			if depth1 < depth2 {
				log.Debug("  => %s WINS as it is lower", node2.Name())
				return false
			}

			log.Debug("  => %s WINS based on equal hint socres, lower id",
				map[bool]string{true: node1.Name(), false: node2.Name()}[id1 < id2])

			return id1 < id2
		}
	}

	// 5) a lower node wins
	if depth1 > depth2 {
		log.Debug("  => %s WINS on depth", node1.Name())
		return true
	}
	if depth1 < depth2 {
		log.Debug("  => %s WINS on depth", node2.Name())
		return false
	}

	log.Debug("  - depth is a TIE")

	if request.CPUType() == cpuReserved {
		// 6) if requesting reserved CPUs, more reserved
		//    capacity per colocated container wins. Reserved
		//    CPUs cannot be precisely accounted as they run
		//    also BestEffort containers that do not carry
		//    information on their CPU needs.
		if reserved1/(score1.Colocated()+1) > reserved2/(score2.Colocated()+1) {
			return true
		}
		if reserved2/(score2.Colocated()+1) > reserved1/(score1.Colocated()+1) {
			return false
		}
		log.Debug("  - reserved capacity is a TIE")
	} else if request.CPUType() == cpuNormal {
		// 7) more isolated capacity wins
		if request.Isolate() && (isolated1 > 0 || isolated2 > 0) {
			if isolated1 > isolated2 {
				return true
			}
			if isolated2 > isolated1 {
				return false
			}

			log.Debug("  => %s WINS based on equal isolated capacity, lower id",
				map[bool]string{true: node1.Name(), false: node2.Name()}[id1 < id2])

			return id1 < id2
		}

		// 8) more slicable shared capacity wins
		if request.FullCPUs() > 0 && (shared1 > 0 || shared2 > 0) {
			if shared1 > shared2 {
				log.Debug("  => %s WINS on more slicable capacity", node1.Name())
				return true
			}
			if shared2 > shared1 {
				log.Debug("  => %s WINS on more slicable capacity", node2.Name())
				return false
			}

			log.Debug("  => %s WINS based on equal slicable capacity, lower id",
				map[bool]string{true: node1.Name(), false: node2.Name()}[id1 < id2])

			return id1 < id2
		}

		// 9) fewer colocated containers win
		if score1.Colocated() < score2.Colocated() {
			log.Debug("  => %s WINS on colocation score", node1.Name())
			return true
		}
		if score2.Colocated() < score1.Colocated() {
			log.Debug("  => %s WINS on colocation score", node2.Name())
			return false
		}

		log.Debug("  - colocation score is a TIE")

		// more shared capacity wins
		if shared1 > shared2 {
			log.Debug("  => %s WINS on more shared capacity", node1.Name())
			return true
		}
		if shared2 > shared1 {
			log.Debug("  => %s WINS on more shared capacity", node2.Name())
			return false
		}
	}

	// 10) lower id wins
	log.Debug("  => %s WINS based on lower id",
		map[bool]string{true: node1.Name(), false: node2.Name()}[id1 < id2])

	return id1 < id2
}

// affinityScore calculate the 'goodness' of the affinity for a node.
func affinityScore(affinities map[int]int32, node Node) float64 {
	Q := 0.75

	// Calculate affinity for every node as a combination of
	// affinities of the nodes on the path from the node to
	// the root and the nodes in the subtree under the node.
	//
	// The combined affinity for node n is Sum_x(A_x*D_x),
	// where for every node x, A_x is the affinity for x and
	// D_x is Q ** (number of links from node to x). IOW, the
	// effective affinity is the sum of the affinity of n and
	// the affinity of each node x of the above mentioned set
	// diluted proprotionally to the distance of x to n, with
	// Q being 0.75.

	var score float64
	for n, q := node.Parent(), Q; !n.IsNil(); n, q = n.Parent(), q*Q {
		a := affinities[n.NodeID()]
		score += q * float64(a)
	}
	node.BreadthFirst(func(n Node) error {
		diff := float64(n.RootDistance() - node.RootDistance())
		q := math.Pow(Q, diff)
		a := affinities[n.NodeID()]
		score += q * float64(a)
		return nil
	})
	return score
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
