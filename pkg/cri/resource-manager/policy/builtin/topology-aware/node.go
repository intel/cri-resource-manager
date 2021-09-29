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
	"fmt"

	system "github.com/intel/cri-resource-manager/pkg/sysfs"
	"github.com/intel/cri-resource-manager/pkg/topology"
	idset "github.com/intel/goresctrl/pkg/utils"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"

	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/kubernetes"
)

//
// Nodes (currently) correspond to some tangible entity in the hardware topology
// hierarchy: full machine (virtual root in multi-socket systems), an individual
// sockets a NUMA node. These nodes are linked into a tree resembling the topology
// tree, with the full machine at the top, and CPU cores at the bottom. In a single
// socket system, the virtual root is replaced with the single socket. In a single
// NUMA node case, the single node is omitted. Also, CPU cores are not modelled as
// nodes, instead they are properties of the nodes (as capacity and free CPU).
//

// NodeKind represents a unique node type.
type NodeKind string

const (
	// NilNode is the type of a nil node.
	NilNode NodeKind = ""
	// UnknownNode is the type of unknown node type.
	UnknownNode NodeKind = "unknown"
	// SocketNode represents a physical CPU package/socket in the system.
	SocketNode NodeKind = "socket"
	// DieNode represents a die within a physical CPU package/socket in the system.
	DieNode NodeKind = "die"
	// NumaNode represents a NUMA node in the system.
	NumaNode NodeKind = "numa node"
	// VirtualNode represents a virtual node, currently the root multi-socket setups.
	VirtualNode NodeKind = "virtual node"
)

const (
	// OverfitPenalty is the per layer penalty for overfitting in the node tree.
	OverfitPenalty = 0.9
)

// Node is the abstract interface our partition tree nodes implement.
type Node interface {
	// IsNil tests if this node is nil.
	IsNil() bool
	// Name returns the name of this node.
	Name() string
	// Kind returns the type of this node.
	Kind() NodeKind
	// NodeID returns the (enumerated) node id of this node.
	NodeID() int
	// Parent returns the parent node of this node.
	Parent() Node
	// Children returns the child nodes of this node.
	Children() []Node
	// LinkParent sets the given node as the parent node, and appends this node as a its child.
	LinkParent(Node)
	// AddChildren appends the nodes to the children, *WITHOUT* updating their parents.
	AddChildren([]Node)
	// IsSameNode returns true if the given node is the same as this one.
	IsSameNode(Node) bool
	// IsRootNode returns true if this node has no parent.
	IsRootNode() bool
	// IsLeafNode returns true if this node has no children.
	IsLeafNode() bool
	// Get the distance of this node from the root node.
	RootDistance() int
	// Get the height of this node (inverse of depth: tree depth - node depth).
	NodeHeight() int
	// System returns the policy sysfs instance.
	System() system.System
	// Policy returns the policy back pointer.
	Policy() *policy
	// DiscoverSupply
	DiscoverSupply(assignedNUMANodes []idset.ID) Supply
	// GetSupply returns the full CPU at this node.
	GetSupply() Supply
	// FreeSupply returns the available CPU supply of this node.
	FreeSupply() Supply
	// GrantedReservedCPU returns the amount of granted reserved CPU of this node and its children.
	GrantedReservedCPU() int
	// GrantedSharedCPU returns the amount of granted shared CPU of this node and its children.
	GrantedSharedCPU() int
	// GetMemset
	GetMemset(mtype memoryType) idset.IDSet
	// AssignNUMANodes assigns the given set of NUMA nodes to this one.
	AssignNUMANodes(ids []idset.ID)
	// DepthFirst traverse the tree@node calling the function at each node.
	DepthFirst(func(Node) error) error
	// BreadthFirst traverse the tree@node calling the function at each node.
	BreadthFirst(func(Node) error) error
	// Dump state of the node.
	Dump(string, ...int)
	// Dump type-specific state of the node.
	dump(string, ...int)

	GetMemoryType() memoryType
	HasMemoryType(memoryType) bool
	GetPhysicalNodeIDs() []idset.ID

	GetScore(Request) Score
	HintScore(topology.Hint) float64
}

// node represents data common to all node types.
type node struct {
	policy   *policy     // policy back pointer
	self     nodeself    // upcasted/type-specific interface
	name     string      // node name
	id       int         // node id
	kind     NodeKind    // node type
	depth    int         // node depth in the tree
	parent   Node        // parent node
	children []Node      // child nodes
	noderes  Supply      // CPU and memory available at this node
	freeres  Supply      // CPU and memory allocatable at this node
	mem      idset.IDSet // controllers with normal DRAM attached
	pMem     idset.IDSet // controllers with PMEM attached
	hbm      idset.IDSet // controllers with HBM attached
}

// nodeself is used to 'upcast' a generic Node interface to a type-specific one.
type nodeself struct {
	node Node
}

// socketnode represents a physical CPU package/socket in the system.
type socketnode struct {
	node                     // common node data
	id     idset.ID          // NUMA node socket id
	syspkg system.CPUPackage // corresponding system.Package
}

// dienode represents a die within a physical CPU package/socket in the system.
type dienode struct {
	node                     // common node data
	id     idset.ID          // die id within socket
	syspkg system.CPUPackage // corresponding system.Package
}

// numanode represents a NUMA node in the system.
type numanode struct {
	node                // common node data
	id      idset.ID    // NUMA node system id
	sysnode system.Node // corresponding system.Node
}

// virtualnode represents a virtual node (ATM only the root in a multi-socket system).
type virtualnode struct {
	node // common node data
}

// special node instance to represent a nonexistent node
var nilnode Node = &node{
	name:     "<nil node>",
	id:       -1,
	kind:     NilNode,
	depth:    -1,
	children: nil,
}

// Init initializes the resource with common node data.
func (n *node) init(p *policy, name string, kind NodeKind, parent Node) {
	n.policy = p
	n.name = name
	n.kind = kind
	n.parent = parent
	n.id = -1

	n.LinkParent(parent)

	n.mem = idset.NewIDSet()
	n.pMem = idset.NewIDSet()
	n.hbm = idset.NewIDSet()
}

// IsNil tests if a node
func (n *node) IsNil() bool {
	return n.kind == NilNode
}

// Name returns the name of this node.
func (n *node) Name() string {
	if n.IsNil() {
		return "<nil node>"
	}
	return n.name
}

// Kind returns the kind of this node.
func (n *node) Kind() NodeKind {
	return n.kind
}

// NodeID returns the node id of this node.
func (n *node) NodeID() int {
	if n.IsNil() {
		return -1
	}
	return n.id
}

// IsSameNode checks if the given node is that same as this one.
func (n *node) IsSameNode(other Node) bool {
	return n.NodeID() == other.NodeID()
}

// IsRootNode returns true if this node has no parent.
func (n *node) IsRootNode() bool {
	return n.parent.IsNil()
}

// IsLeafNode returns true if this node has no children.
func (n *node) IsLeafNode() bool {
	return len(n.children) == 0
}

// RootDistance returns the distance of this node from the root node.
func (n *node) RootDistance() int {
	if n.IsNil() {
		return -1
	}
	return n.depth
}

// NodeHeight returns the hight of this node (tree depth - node depth).
func (n *node) NodeHeight() int {
	if n.IsNil() {
		return -1
	}
	return n.policy.depth - n.depth
}

// Parent returns the parent of this node.
func (n *node) Parent() Node {
	if n.IsNil() {
		return nil
	}

	return n.parent
}

// Children returns the children of this node.
func (n *node) Children() []Node {
	if n.IsNil() {
		return nil
	}

	return n.children
}

// LinkParent sets the given node as the node parent and appends this node to the parents children.
func (n *node) LinkParent(parent Node) {
	n.parent = parent
	if !parent.IsNil() {
		parent.AddChildren([]Node{n})
	}

	n.depth = parent.RootDistance() + 1
}

// AddChildren appends the nodes to the childres, *WITHOUT* setting their parent.
func (n *node) AddChildren(nodes []Node) {
	n.children = append(n.children, nodes...)
}

// Dump information/state of the node.
func (n *node) Dump(prefix string, level ...int) {
	if !log.DebugEnabled() {
		return
	}

	lvl := 0
	if len(level) > 0 {
		lvl = level[0]
	}
	idt := indent(prefix, lvl)

	n.self.node.dump(prefix, lvl)
	log.Debug("%s  - %s", idt, n.noderes.DumpCapacity())
	log.Debug("%s  - %s", idt, n.freeres.DumpAllocatable())
	n.freeres.DumpMemoryState(idt + "  ")
	if n.mem.Size() > 0 {
		log.Debug("%s  - normal memory: %v", idt, n.mem)
	}
	if n.hbm.Size() > 0 {
		log.Debug("%s  - HBM memory: %v", idt, n.hbm)
	}
	if n.pMem.Size() > 0 {
		log.Debug("%s  - PMEM memory: %v", idt, n.pMem)
	}
	for _, grant := range n.policy.allocations.grants {
		cpuNodeID := grant.GetCPUNode().NodeID()
		memNodeID := grant.GetMemoryNode().NodeID()
		switch {
		case cpuNodeID == n.id && memNodeID == n.id:
			log.Debug("%s    + cpu+mem %s", idt, grant)
		case cpuNodeID == n.id:
			log.Debug("%s    + cpuonly %s", idt, grant)
		case memNodeID == n.id:
			log.Debug("%s    + memonly %s", idt, grant)
		}
	}
	if !n.Parent().IsNil() {
		log.Debug("%s  - parent: <%s>", idt, n.Parent().Name())
	}
	if len(n.children) > 0 {
		log.Debug("%s  - children:", idt)
		for _, c := range n.children {
			c.Dump(prefix, lvl+1)
		}
	}
}

// Dump type-specific information about the node.
func (n *node) dump(prefix string, level ...int) {
	n.self.node.dump(prefix, level...)
}

// Do a depth-first traversal starting at node calling the given function at each node.
func (n *node) DepthFirst(fn func(Node) error) error {
	for _, c := range n.children {
		if err := c.DepthFirst(fn); err != nil {
			return err
		}
	}

	return fn(n)
}

// Do a breadth-first traversal starting at node calling the given function at each node.
func (n *node) BreadthFirst(fn func(Node) error) error {
	if err := fn(n); err != nil {
		return err
	}

	for _, c := range n.children {
		if err := c.BreadthFirst(fn); err != nil {
			return err
		}
	}

	return nil
}

// System returns the policy System instance.
func (n *node) System() system.System {
	return n.policy.sys
}

// Policy returns the policy back pointer.
func (n *node) Policy() *policy {
	return n.policy
}

// GetSupply returns the full CPU supply of this node.
func (n *node) GetSupply() Supply {
	return n.self.node.GetSupply()
}

// Discover CPU available at this node.
func (n *node) DiscoverSupply(assignedNUMANodes []idset.ID) Supply {
	return n.self.node.DiscoverSupply(assignedNUMANodes)
}

// discoverSupply discovers the resource supply assigned to this pool node.
func (n *node) discoverSupply(assignedNUMANodes []idset.ID) Supply {
	if n.noderes != nil {
		return n.noderes.Clone()
	}

	if !n.IsLeafNode() {
		log.Debug("%s: cumulating child resources...", n.Name())

		if len(assignedNUMANodes) > 0 {
			log.Fatal("invalid pool setup: trying to attach NUMA nodes to non-leaf node %s",
				n.Name())
		}

		n.noderes = newSupply(n, cpuset.NewCPUSet(), cpuset.NewCPUSet(), cpuset.NewCPUSet(), 0, 0, nil, nil)
		for _, c := range n.children {
			supply := c.GetSupply()
			n.noderes.Cumulate(supply)
			n.mem.Add(c.GetMemset(memoryDRAM).Members()...)
			n.hbm.Add(c.GetMemset(memoryHBM).Members()...)
			n.pMem.Add(c.GetMemset(memoryPMEM).Members()...)
			log.Debug("  + %s", supply.DumpCapacity())
		}
		log.Debug("  = %s", n.noderes.DumpCapacity())
	} else {
		log.Debug("%s: discovering attached/assigned resources...", n.Name())

		mmap := createMemoryMap(0, 0, 0)
		cpus := cpuset.NewCPUSet()

		for _, nodeID := range assignedNUMANodes {
			node := n.System().Node(nodeID)
			nodeCPUs := node.CPUSet()

			meminfo, err := node.MemoryInfo()
			if err != nil {
				log.Fatal("%s: failed to get memory info for NUMA node #%d", n.Name(), nodeID)
			}

			switch node.GetMemoryType() {
			case system.MemoryTypeDRAM:
				n.mem.Add(nodeID)
				mmap.AddDRAM(meminfo.MemTotal)
				shortCPUs := kubernetes.ShortCPUSet(nodeCPUs)
				log.Debug("  + assigned DRAM NUMA node #%d (cpuset: %s, DRAM %.2fM)",
					nodeID, shortCPUs, float64(meminfo.MemTotal)/float64(1024*1024))
			case system.MemoryTypePMEM:
				n.pMem.Add(nodeID)
				mmap.AddPMEM(meminfo.MemTotal)
				log.Debug("  + assigned PMEM NUMA node #%d (DRAM %.2fM)", nodeID,
					float64(meminfo.MemTotal)/float64(1024*1024))
			case system.MemoryTypeHBM:
				n.hbm.Add(nodeID)
				mmap.AddHBM(meminfo.MemTotal)
				log.Debug("  + assigned HBMEM NUMA node #%d (DRAM %.2fM)",
					nodeID, float64(meminfo.MemTotal)/float64(1024*1024))
			default:
				log.Fatal("NUMA node #%d with unknown memory type %v", node.GetMemoryType())
			}

			allowed := nodeCPUs.Intersection(n.policy.allowed)
			isolated := allowed.Intersection(n.policy.isolated)
			reserved := allowed.Intersection(n.policy.reserved).Difference(isolated)
			sharable := allowed.Difference(isolated).Difference(reserved)

			if !reserved.IsEmpty() {
				log.Debug("    allowed reserved CPUs: %s", kubernetes.ShortCPUSet(reserved))
			}
			if !sharable.IsEmpty() {
				log.Debug("    allowed sharable CPUs: %s", kubernetes.ShortCPUSet(sharable))
			}
			if !isolated.IsEmpty() {
				log.Debug("    allowed isolated CPUs: %s", kubernetes.ShortCPUSet(isolated))
			}

			cpus = cpus.Union(allowed)
		}

		isolated := cpus.Intersection(n.policy.isolated)
		reserved := cpus.Intersection(n.policy.reserved).Difference(isolated)
		sharable := cpus.Difference(isolated).Difference(reserved)
		n.noderes = newSupply(n, isolated, reserved, sharable, 0, 0, mmap, nil)
		log.Debug("  = %s", n.noderes.DumpCapacity())
	}

	n.freeres = n.noderes.Clone()
	return n.noderes.Clone()
}

// FreeSupply returns the available CPU supply of this node.
func (n *node) FreeSupply() Supply {
	return n.freeres
}

// Get the set of memory attached to this node.
func (n *node) GetMemset(mtype memoryType) idset.IDSet {
	if n.self.node == nil { // protect against &node{}-abuse by test cases...
		return idset.NewIDSet()
	}
	return n.self.node.GetMemset(mtype)
}

// AssignNUMANodes assigns the given set of NUMA nodes to this one.
func (n *node) AssignNUMANodes(ids []idset.ID) {
	n.self.node.AssignNUMANodes(ids)
}

// assignNUMANodes assigns the given set of NUMA nodes to this one.
func (n *node) assignNUMANodes(ids []idset.ID) {
	mem := createMemoryMap(0, 0, 0)

	for _, numaNodeID := range ids {
		if n.mem.Has(numaNodeID) || n.pMem.Has(numaNodeID) || n.hbm.Has(numaNodeID) {
			log.Warn("*** NUMA node #%d already discovered by or assigned to %s",
				numaNodeID, n.Name())
			continue
		}
		numaNode := n.policy.sys.Node(numaNodeID)
		memTotal := uint64(0)
		if meminfo, err := numaNode.MemoryInfo(); err != nil {
			log.Error("%s: failed to get memory info for NUMA node #%d",
				n.Name(), numaNodeID)
		} else {
			memTotal = meminfo.MemTotal
		}
		switch numaNode.GetMemoryType() {
		case system.MemoryTypeDRAM:
			mem.Add(memTotal, 0, 0)
			n.mem.Add(numaNodeID)
			log.Info("*** DRAM NUMA node #%d assigned to pool node %q",
				numaNodeID, n.Name())
		case system.MemoryTypePMEM:
			n.pMem.Add(numaNodeID)
			mem.Add(0, memTotal, 0)
			log.Info("*** PMEM NUMA node #%d assigned to pool node %q",
				numaNodeID, n.Name())
		case system.MemoryTypeHBM:
			n.hbm.Add(numaNodeID)
			mem.Add(0, 0, memTotal)
			log.Info("*** HBM NUMA node #%d assigned to pool node %q",
				numaNodeID, n.Name())
		default:
			log.Fatal("can't assign NUMA node #%d of type %v to pool node %q",
				numaNodeID, numaNode.GetMemoryType())
		}
	}

	n.noderes.AssignMemory(mem)
	n.freeres.AssignMemory(mem)
}

// Discover the set of memory attached to this node.
func (n *node) GetPhysicalNodeIDs() []idset.ID {
	return n.self.node.GetPhysicalNodeIDs()
}

// GrantedReservedCPU returns the amount of granted reserved CPU of this node and its children.
func (n *node) GrantedReservedCPU() int {
	grantedReserved := n.freeres.GrantedReserved()
	for _, c := range n.children {
		grantedReserved += c.GrantedReservedCPU()
	}
	return grantedReserved
}

// GrantedSharedCPU returns the amount of granted shared CPU of this node and its children.
func (n *node) GrantedSharedCPU() int {
	grantedShared := n.freeres.GrantedShared()
	for _, c := range n.children {
		grantedShared += c.GrantedSharedCPU()
	}
	return grantedShared
}

// Get Score for a cpu request.
func (n *node) GetScore(req Request) Score {
	f := n.FreeSupply()
	return f.GetScore(req)
}

// HintScore calculates the (CPU) score of the node for the given topology hint.
func (n *node) HintScore(hint topology.Hint) float64 {
	return n.self.node.HintScore(hint)
}

func (n *node) GetMemoryType() memoryType {
	var memoryMask memoryType = 0x0
	if n.pMem.Size() > 0 {
		memoryMask |= memoryPMEM
	}
	if n.mem.Size() > 0 {
		memoryMask |= memoryDRAM
	}
	if n.hbm.Size() > 0 {
		memoryMask |= memoryHBM
	}
	return memoryMask
}

func (n *node) HasMemoryType(reqType memoryType) bool {
	nodeType := n.GetMemoryType()
	return (nodeType & reqType) == reqType
}

// NewNumaNode create a node for a CPU socket.
func (p *policy) NewNumaNode(id idset.ID, parent Node) Node {
	n := &numanode{}
	n.self.node = n
	n.node.init(p, fmt.Sprintf("NUMA node #%v", id), NumaNode, parent)
	n.id = id
	n.sysnode = p.sys.Node(id)

	return n
}

// Dump (the NUMA-specific parts of) this node.
func (n *numanode) dump(prefix string, level ...int) {
	log.Debug("%s<NUMA node #%v>", indent(prefix, level...), n.id)
}

// Get CPU supply available at this node.
func (n *numanode) GetSupply() Supply {
	return n.noderes.Clone()
}

func (n *numanode) GetPhysicalNodeIDs() []idset.ID {
	return []idset.ID{n.id}
}

// DiscoverSupply discovers the CPU supply available at this node.
func (n *numanode) DiscoverSupply(assignedNUMANodes []idset.ID) Supply {
	return n.node.discoverSupply(assignedNUMANodes)
}

// GetMemset returns the set of memory attached to this node.
func (n *numanode) GetMemset(mtype memoryType) idset.IDSet {
	mset := idset.NewIDSet()

	if mtype&memoryDRAM != 0 {
		mset.Add(n.mem.Members()...)
	}
	if mtype&memoryHBM != 0 {
		mset.Add(n.hbm.Members()...)
	}
	if mtype&memoryPMEM != 0 {
		mset.Add(n.pMem.Members()...)
	}

	return mset
}

// AssignNUMANodes assigns the given NUMA nodes to this one.
func (n *numanode) AssignNUMANodes(ids []idset.ID) {
	n.node.assignNUMANodes(ids)
}

// HintScore calculates the (CPU) score of the node for the given topology hint.
func (n *numanode) HintScore(hint topology.Hint) float64 {
	switch {
	case hint.CPUs != "":
		return cpuHintScore(hint, n.sysnode.CPUSet())

	case hint.NUMAs != "":
		return numaHintScore(hint, n.id)

	case hint.Sockets != "":
		pkgID := n.sysnode.PackageID()
		score := socketHintScore(hint, n.sysnode.PackageID())
		if score > 0.0 {
			// penalize underfit reciprocally (inverse-proportionally) to the socket size
			score /= float64(len(n.System().Package(pkgID).NodeIDs()))
		}
		return score
	}

	return 0.0
}

// NewDieNode create a node for a CPU die.
func (p *policy) NewDieNode(id idset.ID, parent Node) Node {
	pkg := parent.(*socketnode)
	n := &dienode{}
	n.self.node = n
	n.node.init(p, fmt.Sprintf("die #%v/%v", pkg.id, id), DieNode, parent)
	n.id = id
	n.syspkg = p.sys.Package(pkg.id)

	return n
}

// Dump (the die-specific parts of) this node.
func (n *dienode) dump(prefix string, level ...int) {
	log.Debug("%s<die #%v/%v>", indent(prefix, level...), n.syspkg.ID(), n.id)
}

// Get CPU supply available at this node.
func (n *dienode) GetSupply() Supply {
	return n.noderes.Clone()
}

func (n *dienode) GetPhysicalNodeIDs() []idset.ID {
	ids := make([]idset.ID, 0)
	ids = append(ids, n.id)
	for _, c := range n.children {
		cIds := c.GetPhysicalNodeIDs()
		ids = append(ids, cIds...)
	}
	return ids
}

// DiscoverSupply discovers the CPU supply available at this die.
func (n *dienode) DiscoverSupply(assignedNUMANodes []idset.ID) Supply {
	return n.node.discoverSupply(assignedNUMANodes)
}

// GetMemset returns the set of memory attached to this die.
func (n *dienode) GetMemset(mtype memoryType) idset.IDSet {
	mset := idset.NewIDSet()

	if mtype&memoryDRAM != 0 {
		mset.Add(n.mem.Members()...)
	}
	if mtype&memoryHBM != 0 {
		mset.Add(n.hbm.Members()...)
	}
	if mtype&memoryPMEM != 0 {
		mset.Add(n.pMem.Members()...)
	}

	return mset
}

// AssignNUMANodes assigns the given NUMA nodes to this one.
func (n *dienode) AssignNUMANodes(ids []idset.ID) {
	n.node.assignNUMANodes(ids)
}

// HintScore calculates the (CPU) score of the node for the given topology hint.
func (n *dienode) HintScore(hint topology.Hint) float64 {
	switch {
	case hint.CPUs != "":
		return cpuHintScore(hint, n.syspkg.CPUSet())

	case hint.NUMAs != "":
		return OverfitPenalty * dieHintScore(hint, n.id, n.syspkg)

	case hint.Sockets != "":
		score := socketHintScore(hint, n.syspkg.ID())
		if score > 0.0 {
			// penalize underfit reciprocally (inverse-proportionally) to the socket size in dies
			score /= float64(len(n.syspkg.DieNodeIDs(n.id)))
		}
		return score
	}

	return 0.0
}

// NewSocketNode create a node for a CPU socket.
func (p *policy) NewSocketNode(id idset.ID, parent Node) Node {
	n := &socketnode{}
	n.self.node = n
	n.node.init(p, fmt.Sprintf("socket #%v", id), SocketNode, parent)
	n.id = id
	n.syspkg = p.sys.Package(id)

	return n
}

// Dump (the socket-specific parts of) this node.
func (n *socketnode) dump(prefix string, level ...int) {
	log.Debug("%s<socket #%v>", indent(prefix, level...), n.id)
}

// Get CPU supply available at this node.
func (n *socketnode) GetSupply() Supply {
	return n.noderes.Clone()
}

func (n *socketnode) GetPhysicalNodeIDs() []idset.ID {
	ids := make([]idset.ID, 0)
	ids = append(ids, n.id)
	for _, c := range n.children {
		cIds := c.GetPhysicalNodeIDs()
		ids = append(ids, cIds...)
	}
	return ids
}

// DiscoverSupply discovers the CPU supply available at this socket.
func (n *socketnode) DiscoverSupply(assignedNUMANodes []idset.ID) Supply {
	return n.node.discoverSupply(assignedNUMANodes)
}

// GetMemset returns the set of memory attached to this socket.
func (n *socketnode) GetMemset(mtype memoryType) idset.IDSet {
	mset := idset.NewIDSet()

	if mtype&memoryDRAM != 0 {
		mset.Add(n.mem.Members()...)
	}
	if mtype&memoryHBM != 0 {
		mset.Add(n.hbm.Members()...)
	}
	if mtype&memoryPMEM != 0 {
		mset.Add(n.pMem.Members()...)
	}

	return mset
}

// AssignNUMANodes assigns the given NUMA nodes to this one.
func (n *socketnode) AssignNUMANodes(ids []idset.ID) {
	n.node.assignNUMANodes(ids)
}

// HintScore calculates the (CPU) score of the node for the given topology hint.
func (n *socketnode) HintScore(hint topology.Hint) float64 {
	switch {
	case hint.CPUs != "":
		return cpuHintScore(hint, n.syspkg.CPUSet())

	case hint.NUMAs != "":
		return OverfitPenalty * numaHintScore(hint, n.syspkg.NodeIDs()...)

	case hint.Sockets != "":
		return socketHintScore(hint, n.id)
	}

	return 0.0
}

// NewVirtualNode creates a new virtual node.
func (p *policy) NewVirtualNode(name string, parent Node) Node {
	n := &virtualnode{}
	n.self.node = n
	n.node.init(p, fmt.Sprintf("%s", name), VirtualNode, parent)

	return n
}

// Dump (the virtual-node specific parts of) this node.
func (n *virtualnode) dump(prefix string, level ...int) {
	log.Debug("%s<virtual %s>", indent(prefix, level...), n.name)
}

// Get CPU supply available at this node.
func (n *virtualnode) GetSupply() Supply {
	return n.noderes.Clone()
}

// DiscoverSupply discovers the CPU supply available at this node.
func (n *virtualnode) DiscoverSupply(assignedNUMANodes []idset.ID) Supply {
	return n.node.discoverSupply(assignedNUMANodes)
}

// GetMemset returns the set of memory attached to this socket.
func (n *virtualnode) GetMemset(mtype memoryType) idset.IDSet {
	mset := idset.NewIDSet()

	if mtype&memoryDRAM != 0 {
		mset.Add(n.mem.Members()...)
	}
	if mtype&memoryHBM != 0 {
		mset.Add(n.hbm.Members()...)
	}
	if mtype&memoryPMEM != 0 {
		mset.Add(n.pMem.Members()...)
	}

	return mset
}

// AssignNUMANodes assigns the given NUMA nodes to this one.
func (n *virtualnode) AssignNUMANodes(ids []idset.ID) {
	log.Panic("cannot assign NUMA nodes #%s to %s",
		idset.NewIDSet(ids...).String(), n.Name())
}

// HintScore calculates the (CPU) score of the node for the given topology hint.
func (n *virtualnode) HintScore(hint topology.Hint) float64 {
	// don't bother calculating any scores, the root should always score 1.0
	switch {
	case hint.CPUs != "":
		return cpuHintScore(hint, n.System().CPUSet())

	case hint.NUMAs != "":
		return OverfitPenalty * OverfitPenalty

	case hint.Sockets != "":
		return OverfitPenalty
	}

	return 0.0
}

func (n *virtualnode) GetPhysicalNodeIDs() []idset.ID {
	ids := make([]idset.ID, 0)
	for _, c := range n.children {
		cIds := c.GetPhysicalNodeIDs()
		ids = append(ids, cIds...)
	}
	return ids
}

// Finalize the setup of nilnode.
func init() {
	nilnode.(*node).self.node = nilnode
	nilnode.(*node).parent = nilnode.(*node).self.node
}
