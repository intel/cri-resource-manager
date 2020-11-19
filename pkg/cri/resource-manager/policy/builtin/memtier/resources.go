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
	"fmt"
	"strconv"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"

	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/kubernetes"
	system "github.com/intel/cri-resource-manager/pkg/sysfs"
)

// Supply represents avaialbe CPU and memory capacity of a node.
type Supply interface {
	// GetNode returns the node supplying this capacity.
	GetNode() Node
	// Clone creates a copy of this supply.
	Clone() Supply
	// IsolatedCPUs returns the isolated cpuset in this supply.
	IsolatedCPUs() cpuset.CPUSet
	// SharableCPUs returns the sharable cpuset in this supply.
	SharableCPUs() cpuset.CPUSet
	// Granted returns the locally granted CPU capacity in this supply.
	Granted() int
	// GrantedMemory returns the locally granted memory capacity in this supply.
	GrantedMemory(memoryType) uint64
	// Cumulate cumulates the given supply into this one.
	Cumulate(Supply)
	// AccountAllocate accounts for (removes) allocated exclusive capacity from the supply.
	AccountAllocate(Grant)
	// AccountRelease accounts for (reinserts) released exclusive capacity into the supply.
	AccountRelease(Grant)
	// GetScore calculates how well this supply fits/fulfills the given request.
	GetScore(Request) Score
	// AllocatableSharedCPU calculates the allocatable amount of shared CPU of this supply.
	AllocatableSharedCPU(...bool) int
	// Allocate allocates CPU capacity from this supply and returns it as a grant.
	Allocate(Request) (Grant, error)
	// ReleaseCPU releases a previously allocated CPU grant from this supply.
	ReleaseCPU(Grant)
	// ReleaseMemory releases a previously allocated memory grant from this supply.
	ReleaseMemory(Grant)
	// ReallocateMemory updates the Grant to allocate memory from this supply.
	ReallocateMemory(Grant) error
	// ExtraMemoryReservation returns the memory reservation.
	ExtraMemoryReservation(memoryType) uint64
	// SetExtraMemroyReservation sets the extra memory reservation based on the granted memory.
	SetExtraMemoryReservation(Grant)
	// ReleaseExtraMemoryReservation removes the extra memory reservations based on the granted memory.
	ReleaseExtraMemoryReservation(Grant)
	// MemoryLimit returns the amount of various memory types belonging to this grant.
	MemoryLimit() memoryMap

	// Reserve accounts for CPU grants after reloading cached allocations.
	Reserve(Grant) error
	// ReserveMemory accounts for memory grants after reloading cached allocations.
	ReserveMemory(Grant) error
	// DumpCapacity returns a printable representation of the supply's resource capacity.
	DumpCapacity() string
	// DumpAllocatable returns a printable representation of the supply's alloctable resources.
	DumpAllocatable() string
}

// Request represents CPU and memory resources requested by a container.
type Request interface {
	// GetContainer returns the container requesting CPU capacity.
	GetContainer() cache.Container
	// String returns a printable representation of this request.
	String() string

	// FullCPUs return the number of full CPUs requested.
	FullCPUs() int
	// CPUFraction returns the amount of fractional milli-CPU requested.
	CPUFraction() int
	// Isolate returns whether isolated CPUs are preferred for this request.
	Isolate() bool
	// Elevate returns the requested elevation/allocation displacement for this request.
	Elevate() int
	// MemoryType returns the type(s) of requested memory.
	MemoryType() memoryType
	// MemAmountToAllocate retuns how much memory we need to reserve for a request.
	MemAmountToAllocate() uint64
	// ColdStart returns the cold start timeout.
	ColdStart() time.Duration
}

// Grant represents CPU and memory capacity allocated to a container from a node.
type Grant interface {
	// GetContainer returns the container CPU capacity is granted to.
	GetContainer() cache.Container
	// GetCPUNode returns the node that granted CPU capacity to the container.
	GetCPUNode() Node
	// GetMemoryNode returns the node which granted memory capacity to
	// the container.
	GetMemoryNode() Node
	// ExclusiveCPUs returns the exclusively granted non-isolated cpuset.
	ExclusiveCPUs() cpuset.CPUSet
	// SharedCPUs returns the shared granted cpuset.
	SharedCPUs() cpuset.CPUSet
	// SharedPortion returns the amount of CPUs in milli-CPU granted.
	SharedPortion() int
	// IsolatedCpus returns the exclusively granted isolated cpuset.
	IsolatedCPUs() cpuset.CPUSet
	// MemoryType returns the type(s) of granted memory.
	MemoryType() memoryType
	// SetMemoryNode updates the grant memory controllers.
	SetMemoryNode(Node)
	// Memset returns the granted memory controllers as a string.
	Memset() system.IDSet
	// ExpandMemset() makes the memory controller set larger as the grant
	// is moved up in the node hierarchy.
	ExpandMemset() (bool, error)
	// MemLimit returns the amount of memory that the container is
	// allowed to use.
	MemLimit() memoryMap
	// String returns a printable representation of this grant.
	String() string
	// Release releases the grant from all the Supplys it uses.
	Release()
	// AccountAllocate accounts for (removes) allocated exclusive capacity for this grant.
	AccountAllocate()
	// AccountRelease accounts for (reinserts) released exclusive capacity for this grant.
	AccountRelease()
	// UpdateExtraMemoryReservation() updates the reservations in the subtree
	// of nodes under the node from which the memory was granted.
	UpdateExtraMemoryReservation()
	// RestoreMemset restores the granted memory set to node maximum
	// and reapplies the grant.
	RestoreMemset()
	// ColdStart returns the cold start timeout.
	ColdStart() time.Duration
	// AddTimer adds a cold start timer.
	AddTimer(*time.Timer)
	// StopTimer stops a cold start timer.
	StopTimer()
	// ClearTimer clears the cold start timer pointer.
	ClearTimer()
}

// Score represents how well a supply can satisfy a request.
type Score interface {
	// Calculate the actual score from the collected parameters.
	Eval() float64
	// Supply returns the supply associated with this score.
	Supply() Supply
	// Request returns the request associated with this score.
	Request() Request

	IsolatedCapacity() int
	SharedCapacity() int
	Colocated() int
	HintScores() map[string]float64

	String() string
}

type memoryMap map[memoryType]uint64

// supply implements our Supply interface.
type supply struct {
	node                 Node                // node supplying CPUs and memory
	isolated             cpuset.CPUSet       // isolated CPUs at this node
	sharable             cpuset.CPUSet       // sharable CPUs at this node
	granted              int                 // amount of shareable allocated
	mem                  memoryMap           // available memory for this node
	grantedMem           memoryMap           // total memory granted
	extraMemReservations map[Grant]memoryMap // how much memory each workload above has requested
}

var _ Supply = &supply{}

// request implements our Request interface.
type request struct {
	container cache.Container // container for this request
	full      int             // number of full CPUs requested
	fraction  int             // amount of fractional CPU requested
	isolate   bool            // prefer isolated exclusive CPUs

	memReq  uint64     // memory request
	memLim  uint64     // memory limit
	memType memoryType // requested types of memory

	// elevate indicates how much to elevate the actual allocation of the
	// container in the tree of pools. Or in other words how many levels to
	// go up in the tree starting at the best fitting pool, before assigning
	// the container to an actual pool. Currently ignored.
	elevate int

	// coldStart tells the timeout (in milliseconds) how long to wait until
	// a DRAM memory controller should be added to a container asking for a
	// mixed DRAM/PMEM memory allocation. This allows for a "cold start" where
	// initial memory requests are made to the PMEM memory. A value of 0
	// indicates that cold start is not explicitly requested.
	coldStart time.Duration
}

var _ Request = &request{}

// grant implements our Grant interface.
type grant struct {
	container      cache.Container // container CPU is granted to
	node           Node            // node CPU is supplied from
	memoryNode     Node            // node memory is supplied from
	exclusive      cpuset.CPUSet   // exclusive CPUs
	portion        int             // milliCPUs granted from shared set
	memType        memoryType      // requested types of memory
	memset         system.IDSet    // assigned memory nodes
	allocatedMem   memoryMap       // memory limit
	coldStart      time.Duration   // how long until cold start is done
	coldStartTimer *time.Timer     // timer to trigger cold start timeout
}

var _ Grant = &grant{}

// score implements our Score interface.
type score struct {
	supply    Supply             // CPU supply (node)
	req       Request            // CPU request (container)
	isolated  int                // remaining isolated CPUs
	shared    int                // remaining shared capacity
	colocated int                // number of colocated containers
	hints     map[string]float64 // hint scores
}

var _ Score = &score{}

// newSupply creates CPU supply for the given node, cpusets and existing grant.
func newSupply(n Node, isolated, sharable cpuset.CPUSet, granted int, mem, grantedMem memoryMap) Supply {
	return &supply{
		node:     n,
		isolated: isolated.Clone(),
		sharable: sharable.Clone(),
		granted:  granted,
		// TODO: why are the CPU amounts cloned? Should we do the same for memory to be predictable?
		mem:                  mem,
		grantedMem:           grantedMem,
		extraMemReservations: make(map[Grant]memoryMap),
	}
}

func createMemoryMap(normalMemory, persistentMemory, hbMemory uint64) memoryMap {
	return memoryMap{
		memoryDRAM:   normalMemory,
		memoryPMEM:   persistentMemory,
		memoryHBM:    hbMemory,
		memoryAll:    normalMemory + persistentMemory + hbMemory,
		memoryUnspec: 0,
	}
}

func (m memoryMap) String() string {
	mem, sep := "", ""

	dram, pmem, hbm := m[memoryDRAM], m[memoryPMEM], m[memoryHBM]
	if dram > 0 || pmem > 0 || hbm > 0 {
		if dram > 0 {
			mem += "dram:" + strconv.FormatUint(dram, 10)
			sep = ", "
		}
		if pmem > 0 {
			mem += sep + "pmem:" + strconv.FormatUint(pmem, 10)
			sep = ", "
		}
		if hbm > 0 {
			mem += sep + "hbm:" + strconv.FormatUint(hbm, 10)
		}
	}

	return mem
}

// GetNode returns the node supplying CPU and memory.
func (cs *supply) GetNode() Node {
	return cs.node
}

// Clone clones the given CPU supply.
func (cs *supply) Clone() Supply {
	// Copy the maps.
	mem := make(memoryMap)
	for key, value := range cs.mem {
		mem[key] = value
	}
	grantedMem := make(memoryMap)
	for key, value := range cs.grantedMem {
		grantedMem[key] = value
	}
	return newSupply(cs.node, cs.isolated, cs.sharable, cs.granted, mem, grantedMem)
}

// IsolatedCpus returns the isolated CPUSet of this supply.
func (cs *supply) IsolatedCPUs() cpuset.CPUSet {
	return cs.isolated.Clone()
}

// SharableCpus returns the sharable CPUSet of this supply.
func (cs *supply) SharableCPUs() cpuset.CPUSet {
	return cs.sharable.Clone()
}

// Granted returns the locally granted sharable CPU capacity.
func (cs *supply) Granted() int {
	return cs.granted
}

func (cs *supply) GrantedMemory(memType memoryType) uint64 {
	// Return only granted memory of correct type
	return cs.grantedMem[memType]
}

func (cs *supply) MemoryLimit() memoryMap {
	return cs.mem
}

// Cumulate more CPU to supply.
func (cs *supply) Cumulate(more Supply) {
	mcs := more.(*supply)

	cs.isolated = cs.isolated.Union(mcs.isolated)
	cs.sharable = cs.sharable.Union(mcs.sharable)
	cs.granted += mcs.granted

	for key, value := range mcs.mem {
		cs.mem[key] += value
	}
	for key, value := range mcs.grantedMem {
		cs.grantedMem[key] += value
	}
}

// AccountAllocate accounts for (removes) allocated exclusive capacity from the supply.
func (cs *supply) AccountAllocate(g Grant) {
	if cs.node.IsSameNode(g.GetCPUNode()) {
		return
	}
	exclusive := g.ExclusiveCPUs()
	cs.isolated = cs.isolated.Difference(exclusive)
	cs.sharable = cs.sharable.Difference(exclusive)

	// TODO: same for memory
}

// AccountRelease accounts for (reinserts) released exclusive capacity into the supply.
func (cs *supply) AccountRelease(g Grant) {
	if cs.node.IsSameNode(g.GetCPUNode()) {
		return
	}

	ncs := cs.node.GetSupply()
	nodecpus := ncs.IsolatedCPUs().Union(ncs.SharableCPUs())
	grantcpus := g.ExclusiveCPUs().Intersection(nodecpus)

	isolated := grantcpus.Intersection(ncs.IsolatedCPUs())
	sharable := grantcpus.Intersection(ncs.SharableCPUs())
	cs.isolated = cs.isolated.Union(isolated)
	cs.sharable = cs.sharable.Union(sharable)

	// For memory the extra allocations be released elsewhere.
}

func (cs *supply) allocateMemory(cr *request) (memoryMap, error) {
	memType := cr.MemoryType()
	allocatedMem := createMemoryMap(0, 0, 0)

	if memType == memoryUnspec {
		memType = memoryAll
	}

	amount := cr.MemAmountToAllocate()
	remaining := amount

	// First allocate from PMEM, then DRAM, finally HBM. No need to care about
	// extra memory reservations since the nodes into which the request won't
	// fit have already been filtered out.

	if remaining > 0 && memType&memoryPMEM != 0 {
		available := cs.mem[memoryPMEM] - cs.grantedMem[memoryPMEM]
		if remaining < available {
			cs.grantedMem[memoryPMEM] += remaining
			cs.mem[memoryPMEM] -= remaining
			allocatedMem[memoryPMEM] = remaining
			remaining = 0
		} else {
			cs.grantedMem[memoryPMEM] += available
			cs.mem[memoryPMEM] = 0
			allocatedMem[memoryPMEM] = available
			remaining -= available
		}
	}

	if remaining > 0 && cr.ColdStart() > 0 {
		cs.mem[memoryPMEM] += amount - remaining
		cs.grantedMem[memoryPMEM] = amount - remaining
		return nil, policyError("internal error: not enough memory at %s, short circuit due to cold start", cs.node.Name())
	}

	if remaining > 0 && memType&memoryDRAM != 0 {
		available := cs.mem[memoryDRAM] - cs.grantedMem[memoryDRAM]
		if remaining < available {
			cs.grantedMem[memoryDRAM] += remaining
			cs.mem[memoryDRAM] -= remaining
			allocatedMem[memoryDRAM] = remaining
			remaining = 0
		} else {
			cs.grantedMem[memoryDRAM] += available
			cs.mem[memoryDRAM] = 0
			allocatedMem[memoryDRAM] = available
			remaining -= available
		}
	}

	if remaining > 0 && memType&memoryHBM != 0 {
		available := cs.mem[memoryHBM] - cs.grantedMem[memoryHBM]
		if remaining < available {
			cs.grantedMem[memoryHBM] += remaining
			cs.mem[memoryHBM] -= remaining
			allocatedMem[memoryHBM] = remaining
			remaining = 0
		} else {
			cs.grantedMem[memoryHBM] += available
			cs.mem[memoryHBM] = 0
			allocatedMem[memoryHBM] = available
			remaining -= available
		}
	}

	if remaining > 0 {
		// FIXME: restore the already allocated memory to the supply
		return nil, policyError("internal error: not enough memory at %s", cs.node.Name())
	}

	// TODO: do we need to track the overall memory use or would the individual types be enough?
	cs.mem[memoryAll] -= amount
	cs.grantedMem[memoryAll] += amount

	return allocatedMem, nil
}

// Allocate allocates a grant from the supply.
func (cs *supply) Allocate(r Request) (Grant, error) {
	var exclusive cpuset.CPUSet
	var err error

	cr := r.(*request)

	// allocate isolated exclusive CPUs or slice them off the sharable set
	switch {
	case cr.full > 0 && cs.isolated.Size() >= cr.full && cr.isolate:
		exclusive, err = cs.takeCPUs(&cs.isolated, nil, cr.full)
		if err != nil {
			return nil, policyError("internal error: "+
				"%s: can't take %d exclusive isolated CPUs from %s: %v",
				cs.node.Name(), cr.full, cs.isolated, err)
		}

	case cr.full > 0 && cs.AllocatableSharedCPU() > 1000*cr.full:
		exclusive, err = cs.takeCPUs(&cs.sharable, nil, cr.full)
		if err != nil {
			return nil, policyError("internal error: "+
				"%s: can't take %d exclusive CPUs from %s: %v",
				cs.node.Name(), cr.full, cs.sharable, err)
		}

	case cr.full > 0:
		return nil, policyError("internal error: "+
			"%s: can't slice %d exclusive CPUs from %s, %dm available",
			cs.node.Name(), cr.full, cs.sharable, cs.AllocatableSharedCPU())
	}

	// allocate requested portion of the sharable set
	if cr.fraction > 0 {
		if cs.AllocatableSharedCPU() < cr.fraction {
			return nil, policyError("internal error: "+
				"%s: not enough sharable CPU for %dm, %dm available",
				cs.node.Name(), cr.fraction, cs.sharable, cs.AllocatableSharedCPU())
		}
		cs.granted += cr.fraction
	}

	allocatedMem, err := cs.allocateMemory(cr)
	if err != nil {
		return nil, err
	}

	// allocate only limited memory set due to cold start
	memType := memoryPMEM
	coldStart := cr.ColdStart()
	if coldStart <= 0 {
		memType = cr.memType
	}

	grant := newGrant(cs.node, cr.GetContainer(), exclusive, cr.fraction, memType, cr.memType, allocatedMem, coldStart)

	grant.AccountAllocate()

	return grant, nil
}

func (cs *supply) ReallocateMemory(g Grant) error {
	// The grant has been previously allocated from another supply. Reallocate it here.
	g.GetMemoryNode().FreeSupply().ReleaseMemory(g)

	mem := uint64(0)
	allocatedMemory := g.MemLimit()
	for key, value := range allocatedMemory {
		if cs.mem[key] < value {
			return policyError("internal error: not enough memory for reallocation at %s (released from %s)", cs.GetNode().Name(), g.GetMemoryNode().Name())
		}
		cs.mem[key] -= value
		cs.grantedMem[key] += value
		mem += value
	}
	cs.grantedMem[memoryAll] += mem
	cs.mem[memoryAll] -= mem
	return nil
}

func (cs *supply) ReleaseCPU(g Grant) {
	isolated := g.ExclusiveCPUs().Intersection(cs.node.GetSupply().IsolatedCPUs())
	sharable := g.ExclusiveCPUs().Difference(isolated)

	cs.isolated = cs.isolated.Union(isolated)
	cs.sharable = cs.sharable.Union(sharable)
	cs.granted -= g.SharedPortion()

	g.AccountRelease()
}

// ReleaseMemory returns memory from the given grant to the supply.
func (cs *supply) ReleaseMemory(g Grant) {
	releasedMemory := uint64(0)
	for key, value := range g.MemLimit() {
		cs.grantedMem[key] -= value
		cs.mem[key] += value
		releasedMemory += value
	}
	cs.grantedMem[memoryAll] -= releasedMemory
	cs.mem[memoryAll] += releasedMemory

	cs.node.DepthFirst(func(n Node) error {
		n.FreeSupply().ReleaseExtraMemoryReservation(g)
		return nil
	})
}

func (cs *supply) ExtraMemoryReservation(memType memoryType) uint64 {
	extra := uint64(0)
	for _, res := range cs.extraMemReservations {
		extra += res[memType]
	}
	return extra
}

func (cs *supply) ReleaseExtraMemoryReservation(g Grant) {
	delete(cs.extraMemReservations, g)
}

func (cs *supply) SetExtraMemoryReservation(g Grant) {
	res := make(memoryMap)
	extraMemory := uint64(0)
	for key, value := range g.MemLimit() {
		res[key] = value
		extraMemory += value
	}
	res[memoryAll] = extraMemory
	cs.extraMemReservations[g] = res
}

func (cs *supply) Reserve(g Grant) error {
	isolated := g.IsolatedCPUs()
	exclusive := g.ExclusiveCPUs().Difference(isolated)
	fraction := g.SharedPortion()

	if !cs.isolated.Intersection(isolated).Equals(isolated) {
		return policyError("can't reserve isolated CPUs (%s) of %s from %s",
			isolated.String(), g.String(), cs.DumpAllocatable())
	}
	if !cs.sharable.Intersection(exclusive).Equals(exclusive) {
		return policyError("can't reserve exclusive CPUs (%s) of %s from %s",
			exclusive.String(), g.String(), cs.DumpAllocatable())
	}

	if cs.AllocatableSharedCPU() < 1000*exclusive.Size()+fraction {
		return policyError("can't reserve %d fractional CPUs of %s from %s",
			fraction, g.String(), cs.DumpAllocatable())
	}

	cs.isolated = cs.isolated.Difference(isolated)
	cs.sharable = cs.sharable.Difference(exclusive)
	cs.granted += fraction

	g.AccountAllocate()

	// TODO: do the same for memory

	return nil
}

func (cs *supply) ReserveMemory(g Grant) error {
	mem := uint64(0)
	allocatedMemory := g.MemLimit()
	for key, value := range allocatedMemory {
		if cs.mem[key] < value {
			return policyError("internal error: not enough memory for allocation at %s", g.GetMemoryNode().Name())
		}
		cs.mem[key] -= value
		cs.grantedMem[key] += value
		mem += value
	}
	cs.grantedMem[memoryAll] += mem
	cs.mem[memoryAll] -= mem
	g.UpdateExtraMemoryReservation()
	return nil
}

// takeCPUs takes up to cnt CPUs from a given CPU set to another.
func (cs *supply) takeCPUs(from, to *cpuset.CPUSet, cnt int) (cpuset.CPUSet, error) {
	cset, err := cs.node.Policy().cpuAllocator.AllocateCpus(from, cnt, true)
	if err != nil {
		return cset, err
	}

	if to != nil {
		*to = to.Union(cset)
	}

	return cset, err
}

// DumpCapacity returns a printable representation of the supply's resource capacity.
func (cs *supply) DumpCapacity() string {
	cpu, mem, sep := "", cs.mem.String(), ""

	if !cs.isolated.IsEmpty() {
		cpu = fmt.Sprintf("isolated:%s", kubernetes.ShortCPUSet(cs.isolated))
		sep = ", "
	}
	if !cs.sharable.IsEmpty() {
		cpu += sep + fmt.Sprintf("sharable:%s (%dm)", kubernetes.ShortCPUSet(cs.sharable),
			1000*cs.sharable.Size())
	}

	capacity := "<" + cs.node.Name() + " capacity: "

	if cpu == "" && mem == "" {
		capacity += "-"
	} else {
		sep = ""
		if cpu != "" {
			capacity += "CPU: " + cpu
			sep = ", "
		}
		if mem != "" {
			capacity += sep + "MemLimit: " + mem
		}
	}
	capacity += ">"

	return capacity
}

// DumpAllocatable returns a printable representation of the supply's resource capacity.
func (cs *supply) DumpAllocatable() string {
	cpu, mem, sep := "", cs.mem.String(), ""

	if !cs.isolated.IsEmpty() {
		cpu = fmt.Sprintf("isolated:%s", kubernetes.ShortCPUSet(cs.isolated))
		sep = ", "
	}

	local_granted := cs.granted
	total_granted := cs.node.GrantedSharedCPU()
	if !cs.sharable.IsEmpty() {
		cpu += sep + fmt.Sprintf("sharable:%s (", kubernetes.ShortCPUSet(cs.sharable))
		sep = ""
		if local_granted > 0 || total_granted > 0 {
			cpu += fmt.Sprintf("granted:")
			kind := ""
			if local_granted > 0 {
				cpu += fmt.Sprintf("%dm", local_granted)
				kind = "local"
				sep = "/"
			}
			if total_granted > 0 {
				cpu += sep + fmt.Sprintf("%dm", total_granted)
				kind += sep + "subtree"
			}
			cpu += " " + kind
			sep = ", "
		}
		cpu += sep + fmt.Sprintf("allocatable:%dm)", cs.AllocatableSharedCPU(true))
	}

	allocatable := "<" + cs.node.Name() + " allocatable: "

	if cpu == "" && mem == "" {
		allocatable += "-"
	} else {
		sep = ""
		if cpu != "" {
			allocatable += "CPU: " + cpu
			sep = ", "
		}
		if mem != "" {
			allocatable += sep + "MemLimit: " + mem
		}
	}
	allocatable += ">"

	return allocatable
}

// newRequest creates a new request for the given container.
func newRequest(container cache.Container) Request {
	pod, _ := container.GetPod()
	full, fraction, isolate, elevate := cpuAllocationPreferences(pod, container)
	req, lim, mtype := memoryAllocationPreference(pod, container)
	coldStart := time.Duration(0)

	log.Debug("%s: CPU preferences: full=%v, fraction=%v, isolate=%v",
		container.PrettyName(), full, fraction, isolate)

	if mtype == memoryUnspec {
		mtype = defaultMemoryType
	}

	if mtype&memoryPMEM != 0 && mtype&memoryDRAM != 0 {
		parsedColdStart, err := coldStartPreference(pod, container)
		if err != nil {
			log.Error("Failed to parse cold start preference")
		} else {
			if parsedColdStart.duration > 0 {
				if coldStartOff {
					log.Error("coldstart disabled (movable non-DRAM memory zones present)")
				} else {
					coldStart = parsedColdStart.duration
				}
			}
		}
	} else if mtype == memoryPMEM {
		if coldStartOff {
			mtype = mtype | memoryDRAM
			log.Error("%s: forced also DRAM usage (movable non-DRAM memory zones present)",
				container.PrettyName())
		}
	}

	return &request{
		container: container,
		full:      full,
		fraction:  fraction,
		isolate:   isolate,
		memReq:    req,
		memLim:    lim,
		memType:   mtype,
		elevate:   elevate,
		coldStart: coldStart,
	}
}

// GetContainer returns the container requesting CPU.
func (cr *request) GetContainer() cache.Container {
	return cr.container
}

// String returns aprintable representation of the CPU request.
func (cr *request) String() string {
	mem := "<Memory request: limit:" + strconv.FormatUint(cr.memLim, 10) + ", req:" + strconv.FormatUint(cr.memReq, 10) + ">"
	isolated := map[bool]string{false: "", true: "isolated "}[cr.isolate]
	switch {
	case cr.full == 0 && cr.fraction == 0:
		return fmt.Sprintf("<CPU request "+cr.container.PrettyName()+": ->") + mem

	case cr.full > 0 && cr.fraction > 0:
		return fmt.Sprintf("<CPU request "+cr.container.PrettyName()+": "+
			"%sexclusive: %d, shared: %d>", isolated, cr.full, cr.fraction) + mem

	case cr.full > 0:
		return fmt.Sprintf("<CPU request "+
			cr.container.PrettyName()+": %sexclusive: %d>", isolated, cr.full) + mem

	default:
		return fmt.Sprintf("<CPU request "+
			cr.container.PrettyName()+": shared: %d>", cr.fraction) + mem
	}
}

// FullCPUs return the number of full CPUs requested.
func (cr *request) FullCPUs() int {
	return cr.full
}

// CPUFraction returns the amount of fractional milli-CPU requested.
func (cr *request) CPUFraction() int {
	return cr.fraction
}

// Isolate returns whether isolated CPUs are preferred for this request.
func (cr *request) Isolate() bool {
	return cr.isolate
}

// Elevate returns the requested elevation/allocation displacement for this request.
func (cr *request) Elevate() int {
	return cr.elevate
}

// MemAmountToAllocate retuns how much memory we need to reserve for a request.
func (cr *request) MemAmountToAllocate() uint64 {
	var amount uint64 = 0
	switch cr.GetContainer().GetQOSClass() {
	case v1.PodQOSBurstable:
		// May be a request and/or limit. We focus on the limit because we
		// need to prepare for the case when all containers are using all
		// the memory they are allowed to. If limit is not set then we'll
		// allocate the request (which the container will get).
		if cr.memLim > 0 {
			amount = cr.memLim
		} else {
			amount = cr.memReq
		}
	case v1.PodQOSGuaranteed:
		// Limit and request are the same.
		amount = cr.memLim
	case v1.PodQOSBestEffort:
		// No requests or limits.
		amount = 0
	}
	return amount
}

// MemoryType returns the requested type of memory for the grant.
func (cr *request) MemoryType() memoryType {
	return cr.memType
}

// ColdStart returns the cold start timeout (in milliseconds).
func (cr *request) ColdStart() time.Duration {
	return cr.coldStart
}

// Score collects data for scoring this supply wrt. the given request.
func (cs *supply) GetScore(req Request) Score {
	score := &score{
		supply: cs,
		req:    req,
	}

	cr := req.(*request)
	full, part := cr.full, cr.fraction
	if full == 0 && part == 0 {
		part = 1
	}

	// calculate free shared capacity
	score.shared = cs.AllocatableSharedCPU()

	// calculate isolated node capacity CPU
	if cr.isolate {
		score.isolated = cs.isolated.Size() - full
	}

	// if we don't want isolated or there is not enough, calculate slicable capacity
	if !cr.isolate || score.isolated < 0 {
		score.shared -= 1000 * full
	}

	// calculate fractional capacity
	score.shared -= part

	// calculate colocation score
	for _, grant := range cs.node.Policy().allocations.grants {
		if grant.GetCPUNode().NodeID() == cs.node.NodeID() {
			score.colocated++
		}
	}

	// calculate real hint scores
	hints := cr.container.GetTopologyHints()
	score.hints = make(map[string]float64, len(hints))

	for provider, hint := range cr.container.GetTopologyHints() {
		log.Debug(" - evaluating topology hint %s", hint)
		score.hints[provider] = cs.node.HintScore(hint)
	}

	// calculate any fake hint scores
	pod, _ := cr.container.GetPod()
	key := pod.GetName() + ":" + cr.container.GetName()
	if fakeHints, ok := opt.FakeHints[key]; ok {
		for provider, hint := range fakeHints {
			log.Debug(" - evaluating fake hint %s", hint)
			score.hints[provider] = cs.node.HintScore(hint)
		}
	}
	if fakeHints, ok := opt.FakeHints[cr.container.GetName()]; ok {
		for provider, hint := range fakeHints {
			log.Debug(" - evaluating fake hint %s", hint)
			score.hints[provider] = cs.node.HintScore(hint)
		}
	}

	return score
}

// AllocatableSharedCPU calculates the allocatable amount of shared CPU of this supply.
func (cs *supply) AllocatableSharedCPU(quiet ...bool) int {
	verbose := !(len(quiet) > 0 && quiet[0])

	// Notes:
	//   Take into account the supplies/grants in all ancestors, making sure
	//   none of them gets overcommitted as the result of fulfilling this request.
	shared := 1000*cs.sharable.Size() - cs.node.GrantedSharedCPU()
	if verbose {
		log.Debug("%s: unadjusted free shared CPU: %dm", cs.node.Name(), shared)
	}
	for node := cs.node.Parent(); !node.IsNil(); node = node.Parent() {
		pSupply := node.FreeSupply()
		pShared := 1000*pSupply.SharableCPUs().Size() - pSupply.GetNode().GrantedSharedCPU()
		if pShared < shared {
			if verbose {
				log.Debug("%s: capping free shared CPU (%dm -> %dm) to avoid overcommit of %s",
					cs.node.Name(), shared, pShared, node.Name())
			}
			shared = pShared
		}
	}
	if verbose {
		log.Debug("%s: ancestor-adjusted free shared CPU: %dm", cs.node.Name(), shared)
	}
	return shared
}

// Eval...
func (score *score) Eval() float64 {
	return 1.0
}

func (score *score) Supply() Supply {
	return score.supply
}

func (score *score) Request() Request {
	return score.req
}

func (score *score) IsolatedCapacity() int {
	return score.isolated
}

func (score *score) SharedCapacity() int {
	return score.shared
}

func (score *score) Colocated() int {
	return score.colocated
}

func (score *score) HintScores() map[string]float64 {
	return score.hints
}

func (score *score) String() string {
	return fmt.Sprintf("<CPU score: node %s, isolated:%d, shared:%d, colocated:%d, hints: %v>",
		score.supply.GetNode().Name(), score.isolated, score.shared, score.colocated, score.hints)
}

// newGrant creates a CPU grant from the given node for the container.
func newGrant(n Node, c cache.Container, exclusive cpuset.CPUSet, portion int, initialMt, mt memoryType, allocatedMem memoryMap, coldStart time.Duration) Grant {
	mems := n.GetMemset(initialMt)
	if mems.Size() == 0 {
		mems = n.GetMemset(memoryDRAM)
		if mems.Size() == 0 {
			mems = n.GetMemset(memoryAll)
		}
	}

	return &grant{
		node:         n,
		memoryNode:   n,
		container:    c,
		exclusive:    exclusive,
		portion:      portion,
		memType:      mt,
		memset:       mems.Clone(),
		allocatedMem: allocatedMem,
		coldStart:    coldStart,
	}
}

// GetContainer returns the container this grant is valid for.
func (cg *grant) GetContainer() cache.Container {
	return cg.container
}

// GetNode returns the Node this grant gets its CPU allocation from.
func (cg *grant) GetCPUNode() Node {
	return cg.node
}

// GetNode returns the Node this grant gets its memory allocation from.
func (cg *grant) GetMemoryNode() Node {
	return cg.memoryNode
}

func (cg *grant) SetMemoryNode(n Node) {
	cg.memoryNode = n
	cg.memset = n.GetMemset(cg.MemoryType())
}

// ExclusiveCPUs returns the non-isolated exclusive CPUSet in this grant.
func (cg *grant) ExclusiveCPUs() cpuset.CPUSet {
	return cg.exclusive
}

// SharedCPUs returns the shared CPUSet in this grant.
func (cg *grant) SharedCPUs() cpuset.CPUSet {
	return cg.node.GetSupply().SharableCPUs()
}

// SharedPortion returns the milli-CPU allocation for the shared CPUSet in this grant.
func (cg *grant) SharedPortion() int {
	return cg.portion
}

// ExclusiveCPUs returns the isolated exclusive CPUSet in this grant.
func (cg *grant) IsolatedCPUs() cpuset.CPUSet {
	return cg.node.GetSupply().IsolatedCPUs().Intersection(cg.exclusive)
}

// MemoryType returns the requested type of memory for the grant.
func (cg *grant) MemoryType() memoryType {
	return cg.memType
}

// Memset returns the granted memory controllers as an IDSet.
func (cg *grant) Memset() system.IDSet {
	return cg.memset
}

// MemLimit returns the granted memory.
func (cg *grant) MemLimit() memoryMap {
	return cg.allocatedMem
}

// String returns a printable representation of the CPU grant.
func (cg *grant) String() string {
	var isolated, exclusive, shared, sep string

	isol := cg.IsolatedCPUs()
	if !isol.IsEmpty() {
		isolated = fmt.Sprintf("isolated: %s", isol)
		sep = ", "
	}
	if !cg.exclusive.IsEmpty() {
		exclusive = fmt.Sprintf("%sexclusive: %s", sep, cg.exclusive)
		sep = ", "
	}
	if cg.portion > 0 {
		shared = fmt.Sprintf("%sshared: %s (%dm)", sep,
			cg.node.FreeSupply().SharableCPUs(), cg.portion)
		sep = ", "
	}

	mem := cg.allocatedMem.String()
	if mem != "" {
		mem = sep + "MemLimit: " + mem
	}

	return fmt.Sprintf("<CPU grant for %s from %s: %s%s%s%s>",
		cg.container.PrettyName(), cg.node.Name(), isolated, exclusive, shared, mem)
}

func (cg *grant) AccountAllocate() {
	cg.node.DepthFirst(func(n Node) error {
		n.FreeSupply().AccountAllocate(cg)
		return nil
	})
	for node := cg.node.Parent(); !node.IsNil(); node = node.Parent() {
		node.FreeSupply().AccountAllocate(cg)
	}
}

func (cg *grant) Release() {
	cg.GetCPUNode().FreeSupply().ReleaseCPU(cg)
	cg.GetMemoryNode().FreeSupply().ReleaseMemory(cg)
	cg.StopTimer()
}

func (cg *grant) AccountRelease() {
	cg.node.DepthFirst(func(n Node) error {
		n.FreeSupply().AccountRelease(cg)
		return nil
	})
	for node := cg.node.Parent(); !node.IsNil(); node = node.Parent() {
		node.FreeSupply().AccountRelease(cg)
	}
}

func (cg *grant) RestoreMemset() {
	mems := cg.GetMemoryNode().GetMemset(cg.memType)
	cg.memset = mems
	cg.GetMemoryNode().Policy().applyGrant(cg)
}

func (cg *grant) ExpandMemset() (bool, error) {
	supply := cg.GetMemoryNode().FreeSupply()
	mems := cg.MemLimit()
	node := cg.GetMemoryNode()
	parent := node.Parent()

	// We have to assume that the memory has been allocated how we granted it (if PMEM ran out
	// the allocations have been made from DRAM and so on).

	// Figure out if there is enough memory now to have grant as-is.
	fits := true
	for memType, limit := range mems {
		if limit > 0 {
			// This memory type was granted.
			extra := supply.ExtraMemoryReservation(memType)
			granted := supply.GrantedMemory(memType)
			limit := supply.MemoryLimit()[memType]

			if extra+granted > limit {
				log.Debug("%s: extra():%d + granted(): %d > limit: %d -> moving from %s to %s", memType, extra, granted, limit, node.Name(), parent.Name())
				fits = false
				break
			}
		}
	}

	if fits {
		return false, nil
	}
	// Else it doesn't fit, so move the grant up in the memory tree.

	if parent.IsNil() {
		return false, fmt.Errorf("trying to move a grant up past the root of the tree")
	}

	// Release granted memory from the node and allocate it from the parent node.
	err := parent.FreeSupply().ReallocateMemory(cg)
	if err != nil {
		return false, err
	}
	cg.SetMemoryNode(parent)
	cg.UpdateExtraMemoryReservation()

	// Make the container to use the new memory set.
	// FIXME: this could be done in a second pass to avoid doing this many times
	cg.GetMemoryNode().Policy().applyGrant(cg)

	return true, nil
}

func (cg *grant) UpdateExtraMemoryReservation() {
	// For every subnode, make sure that this grant is added to the extra memory allocation.
	cg.GetMemoryNode().DepthFirst(func(n Node) error {
		// No extra allocation should be done to the node itself.
		if !n.IsSameNode(cg.GetMemoryNode()) {
			supply := n.FreeSupply()
			supply.SetExtraMemoryReservation(cg)
		}
		return nil
	})
}

func (cg *grant) ColdStart() time.Duration {
	return cg.coldStart
}

func (cg *grant) AddTimer(timer *time.Timer) {
	cg.coldStartTimer = timer
}

func (cg *grant) StopTimer() {
	if cg.coldStartTimer != nil {
		cg.coldStartTimer.Stop()
		cg.coldStartTimer = nil
	}
}

func (cg *grant) ClearTimer() {
	if cg.coldStartTimer != nil {
		cg.coldStartTimer = nil
	}
}
