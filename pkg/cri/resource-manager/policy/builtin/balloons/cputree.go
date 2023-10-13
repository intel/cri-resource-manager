// Copyright 2022 Intel Corporation. All Rights Reserved.
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

package balloons

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	system "github.com/intel/cri-resource-manager/pkg/sysfs"
	"github.com/intel/cri-resource-manager/pkg/utils/cpuset"
)

type CPUTopologyLevel int

const (
	CPUTopologyLevelUndefined CPUTopologyLevel = iota
	CPUTopologyLevelSystem
	CPUTopologyLevelPackage
	CPUTopologyLevelDie
	CPUTopologyLevelNuma
	CPUTopologyLevelCore
	CPUTopologyLevelThread
	CPUTopologyLevelCount
)

// cpuTreeNode is a node in the CPU tree.
type cpuTreeNode struct {
	name     string
	level    CPUTopologyLevel
	parent   *cpuTreeNode
	children []*cpuTreeNode
	cpus     cpuset.CPUSet // union of CPUs of child nodes

}

// cpuTreeNodeAttributes contains various attributes of a CPU tree
// node. When allocating or releasing CPUs, all CPU tree nodes in
// which allocating/releasing could be possible are stored to the same
// slice with these attributes. The attributes contain all necessary
// information for comparing which nodes are the best choices for
// allocating/releasing, thus traversing the tree is not needed in the
// comparison phase.
type cpuTreeNodeAttributes struct {
	t                *cpuTreeNode
	depth            int
	currentCpus      cpuset.CPUSet
	freeCpus         cpuset.CPUSet
	currentCpuCount  int
	currentCpuCounts []int
	freeCpuCount     int
	freeCpuCounts    []int
}

// cpuTreeAllocator allocates CPUs from the branch of a CPU tree
// where the "root" node is the topmost CPU of the branch.
type cpuTreeAllocator struct {
	options cpuTreeAllocatorOptions
	root    *cpuTreeNode
}

// cpuTreeAllocatorOptions contains parameters for the CPU allocator
// that that selects CPUs from a CPU tree.
type cpuTreeAllocatorOptions struct {
	// topologyBalancing true prefers allocating from branches
	// with most free CPUs (spread allocations), while false is
	// the opposite (packed allocations).
	topologyBalancing           bool
	preferSpreadOnPhysicalCores bool
}

// Strings returns topology level as a string
func (ctl CPUTopologyLevel) String() string {
	s, ok := cpuTopologyLevelToString[ctl]
	if ok {
		return s
	}
	return fmt.Sprintf("CPUTopologyLevelUnknown(%d)", ctl)
}

// cpuTopologyLevelToString defines names for all CPU topology levels.
var cpuTopologyLevelToString = map[CPUTopologyLevel]string{
	CPUTopologyLevelUndefined: "",
	CPUTopologyLevelSystem:    "system",
	CPUTopologyLevelPackage:   "package",
	CPUTopologyLevelDie:       "die",
	CPUTopologyLevelNuma:      "numa",
	CPUTopologyLevelCore:      "core",
	CPUTopologyLevelThread:    "thread",
}

// MarshalJSON()
func (ctl CPUTopologyLevel) MarshalJSON() ([]byte, error) {
	return json.Marshal(ctl.String())
}

// UnmarshalJSON unmarshals a JSON string to CPUTopologyLevel
func (ctl *CPUTopologyLevel) UnmarshalJSON(data []byte) error {
	var dataString string
	if err := json.Unmarshal(data, &dataString); err != nil {
		return err
	}
	name := strings.ToLower(dataString)
	for ctlConst, ctlString := range cpuTopologyLevelToString {
		if ctlString == name {
			*ctl = ctlConst
			return nil
		}
	}
	return fmt.Errorf("invalid CPU topology level %q", name)
}

// String returns string representation of a CPU tree node.
func (t *cpuTreeNode) String() string {
	if len(t.children) == 0 {
		return t.name
	}
	return fmt.Sprintf("%s%v", t.name, t.children)
}

func (t *cpuTreeNode) PrettyPrint() string {
	origDepth := t.Depth()
	lines := []string{}
	t.DepthFirstWalk(func(tn *cpuTreeNode) error {
		lines = append(lines,
			fmt.Sprintf("%s%s: %q cpus: %s",
				strings.Repeat(" ", (tn.Depth()-origDepth)*4),
				tn.level, tn.name, tn.cpus))
		return nil
	})
	return strings.Join(lines, "\n")
}

// String returns cpuTreeNodeAttributes as a string.
func (tna cpuTreeNodeAttributes) String() string {
	return fmt.Sprintf("%s{%d,%v,%d,%d}", tna.t.name, tna.depth,
		tna.currentCpuCounts,
		tna.freeCpuCount, tna.freeCpuCounts)
}

// NewCpuTree returns a named CPU tree node.
func NewCpuTree(name string) *cpuTreeNode {
	return &cpuTreeNode{
		name: name,
		cpus: cpuset.New(),
	}
}

func (t *cpuTreeNode) CopyTree() *cpuTreeNode {
	newNode := t.CopyNode()
	newNode.children = make([]*cpuTreeNode, 0, len(t.children))
	for _, child := range t.children {
		newNode.AddChild(child.CopyTree())
	}
	return newNode
}

func (t *cpuTreeNode) CopyNode() *cpuTreeNode {
	newNode := cpuTreeNode{
		name:     t.name,
		level:    t.level,
		parent:   t.parent,
		children: t.children,
		cpus:     t.cpus,
	}
	return &newNode
}

// Depth returns the distance from the root node.
func (t *cpuTreeNode) Depth() int {
	if t.parent == nil {
		return 0
	}
	return t.parent.Depth() + 1
}

// AddChild adds new child node to a CPU tree node.
func (t *cpuTreeNode) AddChild(child *cpuTreeNode) {
	child.parent = t
	t.children = append(t.children, child)
}

// AddCpus adds CPUs to a CPU tree node and all its parents.
func (t *cpuTreeNode) AddCpus(cpus cpuset.CPUSet) {
	t.cpus = t.cpus.Union(cpus)
	if t.parent != nil {
		t.parent.AddCpus(cpus)
	}
}

// Cpus returns CPUs of a CPU tree node.
func (t *cpuTreeNode) Cpus() cpuset.CPUSet {
	return t.cpus
}

// SiblingIndex returns the index of this node among its parents
// children. Returns -1 for the root node, -2 if this node is not
// listed among the children of its parent.
func (t *cpuTreeNode) SiblingIndex() int {
	if t.parent == nil {
		return -1
	}
	for idx, child := range t.parent.children {
		if child == t {
			return idx
		}
	}
	return -2
}

func (t *cpuTreeNode) FindLeafWithCpu(cpu int) *cpuTreeNode {
	var found *cpuTreeNode
	t.DepthFirstWalk(func(tn *cpuTreeNode) error {
		if len(tn.children) > 0 {
			return nil
		}
		for _, cpuHere := range tn.cpus.List() {
			if cpu == cpuHere {
				found = tn
				return WalkStop
			}
		}
		return nil // not found here, no more children to search
	})
	return found
}

// WalkSkipChildren error returned from a DepthFirstWalk handler
// prevents walking deeper in the tree. The caller of the
// DepthFirstWalk will get no error.
var WalkSkipChildren error = errors.New("skip children")

// WalkStop error returned from a DepthFirstWalk handler stops the
// walk altogether. The caller of the DepthFirstWalk will get the
// WalkStop error.
var WalkStop error = errors.New("stop")

// DepthFirstWalk walks through nodes in a CPU tree. Every node is
// passed to the handler callback that controls next step by
// returning:
// - nil: continue walking to the next node
// - WalkSkipChildren: continue to the next node but skip children of this node
// - WalkStop: stop walking.
func (t *cpuTreeNode) DepthFirstWalk(handler func(*cpuTreeNode) error) error {
	if err := handler(t); err != nil {
		if err == WalkSkipChildren {
			return nil
		}
		return err
	}
	for _, child := range t.children {
		if err := child.DepthFirstWalk(handler); err != nil {
			return err
		}
	}
	return nil
}

// CpuLocations returns a slice where each element contains names of
// topology elements over which a set of CPUs spans. Example:
// systemNode.CpuLocations(cpuset:0,99) = [["system"],["p0", "p1"], ["p0d0", "p1d0"], ...]
func (t *cpuTreeNode) CpuLocations(cpus cpuset.CPUSet) [][]string {
	names := make([][]string, int(CPUTopologyLevelCount)-int(t.level))
	t.DepthFirstWalk(func(tn *cpuTreeNode) error {
		if tn.cpus.Intersection(cpus).Size() == 0 {
			return WalkSkipChildren
		}
		levelIndex := int(tn.level) - int(t.level)
		names[levelIndex] = append(names[levelIndex], tn.name)
		return nil
	})
	return names
}

// NewCpuTreeFromSystem returns the root node of the topology tree
// constructed from the underlying system.
func NewCpuTreeFromSystem() (*cpuTreeNode, error) {
	sys, err := system.DiscoverSystem(system.DiscoverCPUTopology)
	if err != nil {
		return nil, err
	}
	// TODO: split deep nested loops into functions
	sysTree := NewCpuTree("system")
	sysTree.level = CPUTopologyLevelSystem
	for _, packageID := range sys.PackageIDs() {
		packageTree := NewCpuTree(fmt.Sprintf("p%d", packageID))
		packageTree.level = CPUTopologyLevelPackage
		cpuPackage := sys.Package(packageID)
		sysTree.AddChild(packageTree)
		for _, dieID := range cpuPackage.DieIDs() {
			dieTree := NewCpuTree(fmt.Sprintf("p%dd%d", packageID, dieID))
			dieTree.level = CPUTopologyLevelDie
			packageTree.AddChild(dieTree)
			for _, nodeID := range cpuPackage.DieNodeIDs(dieID) {
				nodeTree := NewCpuTree(fmt.Sprintf("p%dd%dn%d", packageID, dieID, nodeID))
				nodeTree.level = CPUTopologyLevelNuma
				dieTree.AddChild(nodeTree)
				node := sys.Node(nodeID)
				threadsSeen := map[int]struct{}{}
				for _, cpuID := range node.CPUSet().List() {
					if _, alreadySeen := threadsSeen[cpuID]; alreadySeen {
						continue
					}
					cpuTree := NewCpuTree(fmt.Sprintf("p%dd%dn%dcpu%d", packageID, dieID, nodeID, cpuID))

					cpuTree.level = CPUTopologyLevelCore
					nodeTree.AddChild(cpuTree)
					cpu := sys.CPU(cpuID)
					for _, threadID := range cpu.ThreadCPUSet().List() {
						threadsSeen[threadID] = struct{}{}
						threadTree := NewCpuTree(fmt.Sprintf("p%dd%dn%dcpu%dt%d", packageID, dieID, nodeID, cpuID, threadID))
						threadTree.level = CPUTopologyLevelThread
						cpuTree.AddChild(threadTree)
						threadTree.AddCpus(cpuset.New(threadID))
					}
				}
			}
		}
	}
	return sysTree, nil
}

// ToAttributedSlice returns a CPU tree node and recursively all its
// child nodes in a slice that contains nodes with their attributes
// for allocation/releasing comparison.
// - currentCpus is the set of CPUs that can be freed in coming operation
// - freeCpus is the set of CPUs that can be allocated in coming operation
// - filter(tna) returns false if the node can be ignored
func (t *cpuTreeNode) ToAttributedSlice(
	currentCpus, freeCpus cpuset.CPUSet,
	filter func(*cpuTreeNodeAttributes) bool) []cpuTreeNodeAttributes {
	tnas := []cpuTreeNodeAttributes{}
	currentCpuCounts := []int{}
	freeCpuCounts := []int{}
	t.toAttributedSlice(currentCpus, freeCpus, filter, &tnas, 0, currentCpuCounts, freeCpuCounts)
	return tnas
}

func (t *cpuTreeNode) toAttributedSlice(
	currentCpus, freeCpus cpuset.CPUSet,
	filter func(*cpuTreeNodeAttributes) bool,
	tnas *[]cpuTreeNodeAttributes,
	depth int,
	currentCpuCounts []int,
	freeCpuCounts []int) {
	currentCpusHere := t.cpus.Intersection(currentCpus)
	freeCpusHere := t.cpus.Intersection(freeCpus)
	currentCpuCountHere := currentCpusHere.Size()
	currentCpuCountsHere := make([]int, len(currentCpuCounts)+1, len(currentCpuCounts)+1)
	copy(currentCpuCountsHere, currentCpuCounts)
	currentCpuCountsHere[depth] = currentCpuCountHere

	freeCpuCountHere := freeCpusHere.Size()
	freeCpuCountsHere := make([]int, len(freeCpuCounts)+1, len(freeCpuCounts)+1)
	copy(freeCpuCountsHere, freeCpuCounts)
	freeCpuCountsHere[depth] = freeCpuCountHere

	tna := cpuTreeNodeAttributes{
		t:                t,
		depth:            depth,
		currentCpus:      currentCpusHere,
		freeCpus:         freeCpusHere,
		currentCpuCount:  currentCpuCountHere,
		currentCpuCounts: currentCpuCountsHere,
		freeCpuCount:     freeCpuCountHere,
		freeCpuCounts:    freeCpuCountsHere,
	}

	if filter != nil && !filter(&tna) {
		return
	}

	*tnas = append(*tnas, tna)
	for _, child := range t.children {
		child.toAttributedSlice(currentCpus, freeCpus, filter,
			tnas, depth+1, currentCpuCountsHere, freeCpuCountsHere)
	}
}

// SplitLevel returns the root node of a new CPU tree where all
// branches of a topology level have been split into new classes.
func (t *cpuTreeNode) SplitLevel(splitLevel CPUTopologyLevel, cpuClassifier func(int) int) *cpuTreeNode {
	newRoot := t.CopyTree()
	newRoot.DepthFirstWalk(func(tn *cpuTreeNode) error {
		// Dive into the level that will be split.
		if tn.level != splitLevel {
			return nil
		}
		// Classify CPUs to the map: class -> list of cpus
		classCpus := map[int][]int{}
		for _, cpu := range t.cpus.List() {
			class := cpuClassifier(cpu)
			classCpus[class] = append(classCpus[class], cpu)
		}
		// Clear existing children of this node. New children
		// will be classes whose children are masked versions
		// of original children of this node.
		origChildren := tn.children
		tn.children = make([]*cpuTreeNode, 0, len(classCpus))
		// Add new child corresponding each class.
		for class, cpus := range classCpus {
			cpuMask := cpuset.New(cpus...)
			newNode := NewCpuTree(fmt.Sprintf("%sclass%d", tn.name, class))
			tn.AddChild(newNode)
			newNode.cpus = tn.cpus.Intersection(cpuMask)
			newNode.level = tn.level
			newNode.parent = tn
			for _, child := range origChildren {
				newChild := child.CopyTree()
				newChild.DepthFirstWalk(func(cn *cpuTreeNode) error {
					cn.cpus = cn.cpus.Intersection(cpuMask)
					if cn.cpus.Size() == 0 && cn.parent != nil {
						// all cpus masked
						// out: cut out this
						// branch
						newSiblings := []*cpuTreeNode{}
						for _, child := range cn.parent.children {
							if child != cn {
								newSiblings = append(newSiblings, child)
							}
						}
						cn.parent.children = newSiblings
						return WalkSkipChildren
					}
					return nil
				})
				newNode.AddChild(newChild)
			}
		}
		return WalkSkipChildren
	})
	return newRoot
}

// NewAllocator returns new CPU allocator for allocating CPUs from a
// CPU tree branch.
func (t *cpuTreeNode) NewAllocator(options cpuTreeAllocatorOptions) *cpuTreeAllocator {
	ta := &cpuTreeAllocator{
		root:    t,
		options: options,
	}
	if options.preferSpreadOnPhysicalCores {
		newTree := t.SplitLevel(CPUTopologyLevelNuma,
			// CPU classifier: class of the CPU equals to
			// the index in the child list of its parent
			// node in the tree. Expect leaf node is a
			// hyperthread, parent a physical core.
			func(cpu int) int {
				leaf := t.FindLeafWithCpu(cpu)
				if leaf == nil {
					log.Fatalf("SplitLevel CPU classifier: cpu %d not in tree:\n%s\n\n", cpu, t.PrettyPrint())
				}
				return leaf.SiblingIndex()
			})
		ta.root = newTree
	}
	return ta
}

// sorterAllocate implements an "is-less-than" callback that helps
// sorting a slice of cpuTreeNodeAttributes. The first item in the
// sorted list contains an optimal CPU tree node for allocating new
// CPUs.
func (ta *cpuTreeAllocator) sorterAllocate(tnas []cpuTreeNodeAttributes) func(int, int) bool {
	return func(i, j int) bool {
		if tnas[i].depth != tnas[j].depth {
			return tnas[i].depth > tnas[j].depth
		}
		for tdepth := 0; tdepth < len(tnas[i].currentCpuCounts); tdepth += 1 {
			// After this currentCpus will increase.
			// Maximize the maximal amount of currentCpus
			// as high level in the topology as possible.
			if tnas[i].currentCpuCounts[tdepth] != tnas[j].currentCpuCounts[tdepth] {
				return tnas[i].currentCpuCounts[tdepth] > tnas[j].currentCpuCounts[tdepth]
			}
		}
		for tdepth := 0; tdepth < len(tnas[i].freeCpuCounts); tdepth += 1 {
			// After this freeCpus will decrease.
			if tnas[i].freeCpuCounts[tdepth] != tnas[j].freeCpuCounts[tdepth] {
				if ta.options.topologyBalancing {
					// Goal: minimize maximal freeCpus in topology.
					return tnas[i].freeCpuCounts[tdepth] > tnas[j].freeCpuCounts[tdepth]
				} else {
					// Goal: maximize maximal freeCpus in topology.
					return tnas[i].freeCpuCounts[tdepth] < tnas[j].freeCpuCounts[tdepth]
				}
			}
		}
		return tnas[i].t.name < tnas[j].t.name
	}
}

// sorterRelease implements an "is-less-than" callback that helps
// sorting a slice of cpuTreeNodeAttributes. The first item in the
// list contains an optimal CPU tree node for releasing new CPUs.
func (ta *cpuTreeAllocator) sorterRelease(tnas []cpuTreeNodeAttributes) func(int, int) bool {
	return func(i, j int) bool {
		if tnas[i].depth != tnas[j].depth {
			return tnas[i].depth > tnas[j].depth
		}
		for tdepth := 0; tdepth < len(tnas[i].currentCpuCounts); tdepth += 1 {
			// After this currentCpus will decrease. Aim
			// to minimize the minimal amount of
			// currentCpus in order to decrease
			// fragmentation as high level in the topology
			// as possible.
			if tnas[i].currentCpuCounts[tdepth] != tnas[j].currentCpuCounts[tdepth] {
				return tnas[i].currentCpuCounts[tdepth] < tnas[j].currentCpuCounts[tdepth]
			}
		}
		for tdepth := 0; tdepth < len(tnas[i].freeCpuCounts); tdepth += 1 {
			// After this freeCpus will increase. Try to
			// maximize minimal free CPUs for better
			// isolation as high level in the topology as
			// possible.
			if tnas[i].freeCpuCounts[tdepth] != tnas[j].freeCpuCounts[tdepth] {
				if ta.options.topologyBalancing {
					return tnas[i].freeCpuCounts[tdepth] < tnas[j].freeCpuCounts[tdepth]
				} else {
					return tnas[i].freeCpuCounts[tdepth] < tnas[j].freeCpuCounts[tdepth]
				}
			}
		}
		return tnas[i].t.name > tnas[j].t.name
	}
}

// ResizeCpus implements topology awareness to both adding CPUs to and
// removing them from a set of CPUs. It returns CPUs from which actual
// allocation or releasing of CPUs can be done. ResizeCpus does not
// allocate or release CPUs.
//
// Parameters:
//   - currentCpus: a set of CPUs to/from which CPUs would be added/removed.
//   - freeCpus: a set of CPUs available CPUs.
//   - delta: number of CPUs to add (if positive) or remove (if negative).
//
// Return values:
//   - addFromCpus contains free CPUs from which delta CPUs can be
//     allocated. Note that the size of the set may be larger than
//     delta: there is room for other allocation logic to select from
//     these CPUs.
//   - removeFromCpus contains CPUs in currentCpus set from which
//     abs(delta) CPUs can be freed.
func (ta *cpuTreeAllocator) ResizeCpus(currentCpus, freeCpus cpuset.CPUSet, delta int) (cpuset.CPUSet, cpuset.CPUSet, error) {
	if delta > 0 {
		addFromSuperset, removeFromSuperset, err := ta.resizeCpus(currentCpus, freeCpus, delta)
		if !ta.options.preferSpreadOnPhysicalCores || addFromSuperset.Size() == delta {
			return addFromSuperset, removeFromSuperset, err
		}
		// addFromSuperset contains more CPUs (equally good
		// choices) than actually needed. In case of
		// preferSpreadOnPhysicalCores, however, selecting any
		// of these does not result in equally good
		// result. Therefore, in this case, construct addFrom
		// set by adding one CPU at a time.
		addFrom := cpuset.New()
		for n := 0; n < delta; n++ {
			addSingleFrom, _, err := ta.resizeCpus(currentCpus, freeCpus, 1)
			if err != nil {
				return addFromSuperset, removeFromSuperset, err
			}
			if addSingleFrom.Size() != 1 {
				return addFromSuperset, removeFromSuperset, fmt.Errorf("internal error: failed to find single CPU to allocate, "+
					"currentCpus=%s freeCpus=%s expectedSingle=%s",
					currentCpus, freeCpus, addSingleFrom)
			}
			addFrom = addFrom.Union(addSingleFrom)
			if addFrom.Size() != n+1 {
				return addFromSuperset, removeFromSuperset, fmt.Errorf("internal error: double add the same CPU (%s) to cpuset %s on round %d",
					addSingleFrom, addFrom, n+1)
			}
			currentCpus = currentCpus.Union(addSingleFrom)
			freeCpus = freeCpus.Difference(addSingleFrom)
		}
		return addFrom, removeFromSuperset, nil
	}
	// In multi-CPU removal, remove CPUs one by one instead of
	// trying to find a single topology element from which all of
	// them could be removed.
	removeFrom := cpuset.New()
	addFrom := cpuset.New()
	for n := 0; n < -delta; n++ {
		_, removeSingleFrom, err := ta.resizeCpus(currentCpus, freeCpus, -1)
		if err != nil {
			return addFrom, removeFrom, err
		}
		// Make cheap internal error checks in order to capture
		// issues in alternative algorithms.
		if removeSingleFrom.Size() != 1 {
			return addFrom, removeFrom, fmt.Errorf("internal error: failed to find single cpu to free, "+
				"currentCpus=%s freeCpus=%s expectedSingle=%s",
				currentCpus, freeCpus, removeSingleFrom)
		}
		if removeFrom.Union(removeSingleFrom).Size() != n+1 {
			return addFrom, removeFrom, fmt.Errorf("internal error: double release of a cpu, "+
				"currentCpus=%s freeCpus=%s alreadyRemoved=%s removedNow=%s",
				currentCpus, freeCpus, removeFrom, removeSingleFrom)
		}
		removeFrom = removeFrom.Union(removeSingleFrom)
		currentCpus = currentCpus.Difference(removeSingleFrom)
		freeCpus = freeCpus.Union(removeSingleFrom)
	}
	return addFrom, removeFrom, nil
}

func (ta *cpuTreeAllocator) resizeCpus(currentCpus, freeCpus cpuset.CPUSet, delta int) (cpuset.CPUSet, cpuset.CPUSet, error) {
	tnas := ta.root.ToAttributedSlice(currentCpus, freeCpus,
		func(tna *cpuTreeNodeAttributes) bool {
			// filter out branches with insufficient cpus
			if delta > 0 && tna.freeCpuCount-delta < 0 {
				// cannot allocate delta cpus
				return false
			}
			if delta < 0 && tna.currentCpuCount+delta < 0 {
				// cannot release delta cpus
				return false
			}
			return true
		})

	// Sort based on attributes
	if delta > 0 {
		sort.Slice(tnas, ta.sorterAllocate(tnas))
	} else {
		sort.Slice(tnas, ta.sorterRelease(tnas))
	}
	if len(tnas) == 0 {
		return freeCpus, currentCpus, fmt.Errorf("not enough free CPUs")
	}
	return tnas[0].freeCpus, tnas[0].currentCpus, nil
}
