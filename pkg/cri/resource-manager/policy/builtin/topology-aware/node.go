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
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
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

const (
	// OverfitPenalty is the per layer penalty for overfitting in the node tree.
	OverfitPenalty = 0.9
)

// nodeKind represents a unique node type.
type nodeKind int

const (
	// virtualKind represents a virtual node, currently the root of multi-socket setups.
	virtualKind nodeKind = iota
	// socketKind represents a physical CPU package/socket in the system.
	socketKind
	// numaKind represents a NUMA node in the system.
	numaKind
)

// String implements Stringer interface for nodeKind
func (k nodeKind) String() string {
	return []string{"virtual", "socket", "numa"}[k]
}

// FIXME: Since we don't expose this interface we can drop it and use closures instead.
//        Like in the case with attachedCPUSet().
type hintScorer interface {
	hintScore(hint system.TopologyHint) float64
}

// node represents data common to all node types.
type node struct {
	parent   *node
	children []*node

	name    string       // node name
	id      int          // enumerated node id (or rather pool id: the value is not just identity, but comparable priority)
	kind    nodeKind     // node type
	policy  *policy      // policy back pointer
	nodecpu CPUSupply    // CPU available at this node
	freecpu CPUSupply    // CPU allocatable at this node
	mem     system.IDSet // memory attached to this node
	data    hintScorer   // kind specific data

	// attachedCPUSet returns CPUSet physically attached to the node
	attachedCPUSet func() cpuset.CPUSet
}

func newNode(parent *node, name string, kind nodeKind, p *policy) *node {
	n := &node{
		parent:   parent,
		name:     name,
		kind:     kind,
		policy:   p,
		children: make([]*node, 0),
	}
	if parent != nil {
		parent.children = append(parent.children, n)
	}
	return n
}

// String implements Stringer interface for node
func (n *node) String() string {
	if n.kind == virtualKind {
		return fmt.Sprintf("<%s node %s>", n.kind, n.name)
	}

	return fmt.Sprintf("<%s>", n.name)
}

// hintScore calculates the (CPU) score of the node for the given topology hint.
func (n *node) hintScore(hint system.TopologyHint) float64 {
	return n.data.hintScore(hint)
}

// Do a depth-first traversal starting at node calling the given function at each node.
func (n *node) depthFirst(fn func(*node) error) error {
	for _, c := range n.children {
		if err := c.depthFirst(fn); err != nil {
			return err
		}
	}

	return fn(n)
}

// grantedCPU returns the amount of granted shared CPU capacity of this node.
func (n *node) grantedCPU() int {
	granted := 0
	n.depthFirst(func(tn *node) error {
		granted += tn.freecpu.Granted()
		return nil
	})

	return granted
}

// rootDistance returns the distance of this node from the root node.
func (n *node) rootDistance() int {
	distance := 0
	for parent := n.parent; parent != nil; parent = parent.parent {
		distance++
	}
	return distance
}

func (n *node) isSameNode(other *node) bool {
	return n.id == other.id
}

func (n *node) dump(prefix string, level int) {
	if !log.DebugEnabled() {
		return
	}

	depth := level * IndentDepth
	idt := fmt.Sprintf("%s%*.*s", prefix, depth, depth, "")
	log.Debug("%s%s", idt, n)
	log.Debug("%s  - node CPU: %v", idt, n.nodecpu)
	log.Debug("%s  - free CPU: %v", idt, n.freecpu)
	log.Debug("%s  - memory: %v", idt, n.mem)
	for _, grant := range n.policy.allocations.CPU {
		if grant.GetNode().isSameNode(n) {
			log.Debug("%s    + %s", idt, grant)
		}
	}
	if n.parent != nil {
		log.Debug("%s  - parent: <%s>", idt, n.parent.name)
	}
	if len(n.children) > 0 {
		log.Debug("%s  - children:", idt)
		for _, c := range n.children {
			c.dump(prefix, level+1)
		}
	}
}

func (n *node) getMemset() system.IDSet {
	if n.mem != nil {
		return n.mem.Clone()
	}

	if n.kind == numaKind {
		panic("numa nodes have their mem field set up at construction time")
	}

	n.mem = system.NewIDSet()
	for _, c := range n.children {
		n.mem.Add(c.getMemset().Members()...)
	}

	return n.mem.Clone()
}

func (n *node) discoverCPU() CPUSupply {
	log.Debug("discovering CPU available at node %s...", n.name)

	if len(n.children) > 0 {
		n.nodecpu = newCPUSupply(n, cpuset.NewCPUSet(), cpuset.NewCPUSet(), 0)
		for _, c := range n.children {
			n.nodecpu.Cumulate(c.discoverCPU())
		}
	} else {
		cpus := n.attachedCPUSet()
		isolated := cpus.Intersection(n.policy.isolated)
		sharable := cpus.Difference(isolated)
		n.nodecpu = newCPUSupply(n, isolated, sharable, 0)
	}

	n.freecpu = n.nodecpu.Clone()
	return n.nodecpu.Clone()
}

// Data specific to virtual nodes
type virtualData struct {
	owner *node // back reference to the owner of this data
}

// Data specific to socket nodes
type socketData struct {
	sysID  system.ID       // NUMA node system id
	syspkg *system.Package // corresponding system.Package
}

// Data specific to numa nodes
type numaData struct {
	owner   *node        // back reference to the owner of this data
	sysID   system.ID    // NUMA node system id
	sysnode *system.Node // corresponding system.Node
}

func newNumaNode(id system.ID, p *policy, parent *node) *node {
	n := newNode(parent, fmt.Sprintf("%s node %v", numaKind, id), numaKind, p)

	numadata := &numaData{
		sysID:   id,
		sysnode: p.sys.Node(id),
		owner:   n,
	}
	n.data = numadata
	n.mem = system.NewIDSet(id)

	n.attachedCPUSet = func() cpuset.CPUSet {
		return numadata.sysnode.CPUSet()
	}

	return n
}

func newSocketNode(id system.ID, p *policy, parent *node) *node {
	n := newNode(parent, fmt.Sprintf("%s node %v", socketKind, id), socketKind, p)

	socketdata := &socketData{
		sysID:  id,
		syspkg: p.sys.Package(id),
	}
	n.data = socketdata

	n.attachedCPUSet = func() cpuset.CPUSet {
		return socketdata.syspkg.CPUSet()
	}

	return n
}

func newVirtualNode(name string, p *policy, parent *node) *node {
	n := newNode(parent, name, virtualKind, p)

	n.data = &virtualData{
		owner: n,
	}

	n.attachedCPUSet = func() cpuset.CPUSet {
		return cpuset.NewCPUSet()
	}

	return n
}

// hintScore implements hintScorer interface for virtualData type
func (d *virtualData) hintScore(hint system.TopologyHint) float64 {
	// don't bother calculating any scores, the root should always score 1.0
	switch {
	case hint.CPUs != "":
		return cpuHintScore(hint, d.owner.policy.sys.CPUSet())

	case hint.NUMAs != "":
		return OverfitPenalty * OverfitPenalty

	case hint.Sockets != "":
		return OverfitPenalty
	}

	return 0.0
}

// hintScore implements hintScorer interface for socketData type
func (d *socketData) hintScore(hint system.TopologyHint) float64 {
	switch {
	case hint.CPUs != "":
		return cpuHintScore(hint, d.syspkg.CPUSet())

	case hint.NUMAs != "":
		return OverfitPenalty * numaHintScore(hint, d.syspkg.NodeIDs()...)

	case hint.Sockets != "":
		return socketHintScore(hint, d.sysID)
	}

	return 0.0
}

// hintScore implements hintScorer interface for numaData type
func (d *numaData) hintScore(hint system.TopologyHint) float64 {
	switch {
	case hint.CPUs != "":
		return cpuHintScore(hint, d.sysnode.CPUSet())

	case hint.NUMAs != "":
		return numaHintScore(hint, d.sysID)

	case hint.Sockets != "":
		pkgID := d.sysnode.PackageID()
		score := socketHintScore(hint, pkgID)
		if score > 0.0 {
			// penalize underfit reciprocally (inverse-proportionally) to the socket size
			score /= float64(len(d.owner.policy.sys.PackageNodeIDs(pkgID)))
		}
		return score
	}

	return 0.0
}
