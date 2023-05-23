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
	"strconv"
	"time"

	v1 "k8s.io/api/core/v1"

	"github.com/intel/cri-resource-manager/pkg/cpuallocator"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	"github.com/intel/cri-resource-manager/pkg/topology"
	"github.com/intel/cri-resource-manager/pkg/utils/cpuset"
	idset "github.com/intel/goresctrl/pkg/utils"
)

// Supply represents avaialbe CPU and memory capacity of a node.
type Supply interface {
	// GetNode returns the node supplying this capacity.
	GetNode() Node
	// Clone creates a copy of this supply.
	Clone() Supply
	// IsolatedCPUs returns the isolated cpuset in this supply.
	IsolatedCPUs() cpuset.CPUSet
	// ReservedCPUs returns the reserved cpuset in this supply.
	ReservedCPUs() cpuset.CPUSet
	// SharableCPUs returns the sharable cpuset in this supply.
	SharableCPUs() cpuset.CPUSet
	// GrantedReserved returns the locally granted reserved CPU capacity in this supply.
	GrantedReserved() int
	// GrantedShared returns the locally granted shared CPU capacity in this supply.
	GrantedShared() int
	// GrantedMemory returns the locally granted memory capacity in this supply.
	GrantedMemory(memoryType) uint64
	// Cumulate cumulates the given supply into this one.
	Cumulate(Supply)
	// AssignMemory adds extra memory to this supply (for extra NUMA nodes assigned to a pool).
	AssignMemory(mem memoryMap)
	// AccountAllocateCPU accounts for (removes) allocated exclusive capacity from the supply.
	AccountAllocateCPU(Grant)
	// AccountReleaseCPU accounts for (reinserts) released exclusive capacity into the supply.
	AccountReleaseCPU(Grant)
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
	// DumpMemoryState dumps the state of the available and allocated memory.
	DumpMemoryState(string)
}

// Request represents CPU and memory resources requested by a container.
type Request interface {
	// GetContainer returns the container requesting CPU capacity.
	GetContainer() cache.Container
	// String returns a printable representation of this request.
	String() string
	// CPUType returns the type of requested CPU.
	CPUType() cpuClass
	// SetCPUType sets the type of requested CPU.
	SetCPUType(cpuType cpuClass)
	// FullCPUs return the number of full CPUs requested.
	FullCPUs() int
	// CPUFraction returns the amount of fractional milli-CPU requested.
	CPUFraction() int
	// Isolate returns whether isolated CPUs are preferred for this request.
	Isolate() bool
	// MemoryType returns the type(s) of requested memory.
	MemoryType() memoryType
	// MemAmountToAllocate retuns how much memory we need to reserve for a request.
	MemAmountToAllocate() uint64
	// ColdStart returns the cold start timeout.
	ColdStart() time.Duration
}

// Grant represents CPU and memory capacity allocated to a container from a node.
type Grant interface {
	// SetCPUPortion sets the fraction CPU portion for the grant.
	SetCPUPortion(fraction int)
	// SetMemoryAllocation sets the memory allocation for the grant.
	SetMemoryAllocation(memoryType, memoryMap, time.Duration)
	// Clone creates a copy of this grant.
	Clone() Grant
	// RefetchNodes updates the stored cpu and memory nodes of this grant by name.
	RefetchNodes() error
	// GetContainer returns the container CPU capacity is granted to.
	GetContainer() cache.Container
	// GetCPUNode returns the node that granted CPU capacity to the container.
	GetCPUNode() Node
	// GetMemoryNode returns the node which granted memory capacity to
	// the container.
	GetMemoryNode() Node
	// CPUType returns the type of granted CPUs
	CPUType() cpuClass
	// CPUPortion returns granted milli-CPUs of non-full CPUs of CPUType().
	// CPUPortion() == ReservedPortion() + SharedPortion().
	CPUPortion() int
	// ExclusiveCPUs returns the exclusively granted non-isolated cpuset.
	ExclusiveCPUs() cpuset.CPUSet
	// ReservedCPUs returns the reserved granted cpuset.
	ReservedCPUs() cpuset.CPUSet
	// ReservedPortion() returns the amount of CPUs in milli-CPU granted.
	ReservedPortion() int
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
	Memset() idset.IDSet
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
	// AccountAllocateCPU accounts for (removes) allocated exclusive capacity for this grant.
	AccountAllocateCPU()
	// AccountReleaseCPU accounts for (reinserts) released exclusive capacity for this grant.
	AccountReleaseCPU()
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
	ReservedCapacity() int
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
	reserved             cpuset.CPUSet       // reserved CPUs at this node
	sharable             cpuset.CPUSet       // sharable CPUs at this node
	grantedReserved      int                 // amount of reserved CPUs allocated
	grantedShared        int                 // amount of shareable CPUs allocated
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
	cpuType   cpuClass        // preferred CPU type (normal, reserved)

	memReq  uint64     // memory request
	memLim  uint64     // memory limit
	memType memoryType // requested types of memory

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
	cpuType        cpuClass        // type of CPUs (normal, reserved, ...)
	cpuPortion     int             // milliCPUs granted from CPUs of cpuType
	memType        memoryType      // requested types of memory
	memset         idset.IDSet     // assigned memory nodes
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
	reserved  int                // remaining reserved CPUs
	shared    int                // remaining shared capacity
	colocated int                // number of colocated containers
	hints     map[string]float64 // hint scores
}

var _ Score = &score{}

// newSupply creates CPU supply for the given node, cpusets and existing grant.

func newSupply(n Node, isolated, reserved, sharable cpuset.CPUSet, grantedReserved int, grantedShared int, mem, grantedMem memoryMap) Supply {
	if mem == nil {
		mem = createMemoryMap(0, 0, 0)
	}
	if grantedMem == nil {
		grantedMem = createMemoryMap(0, 0, 0)
	}
	return &supply{
		node:                 n,
		isolated:             isolated.Clone(),
		reserved:             reserved.Clone(),
		sharable:             sharable.Clone(),
		grantedReserved:      grantedReserved,
		grantedShared:        grantedShared,
		mem:                  mem,
		grantedMem:           grantedMem,
		extraMemReservations: make(map[Grant]memoryMap),
	}
}

func createMemoryMap(dram, pmem, hbm uint64) memoryMap {
	return memoryMap{
		memoryDRAM:   dram,
		memoryPMEM:   pmem,
		memoryHBM:    hbm,
		memoryAll:    dram + pmem + hbm,
		memoryUnspec: 0,
	}
}

func (m memoryMap) Add(dram, pmem, hbm uint64) {
	m[memoryDRAM] += dram
	m[memoryPMEM] += pmem
	m[memoryPMEM] += hbm
	m[memoryAll] += dram + pmem + hbm
}

func (m memoryMap) AddDRAM(dram uint64) {
	m[memoryDRAM] += dram
	m[memoryAll] += dram
}

func (m memoryMap) AddPMEM(pmem uint64) {
	m[memoryPMEM] += pmem
	m[memoryAll] += pmem
}

func (m memoryMap) AddHBM(hbm uint64) {
	m[memoryHBM] += hbm
	m[memoryAll] += hbm
}

func (m memoryMap) String() string {
	mem, sep := "", ""

	dram, pmem, hbm, types := m[memoryDRAM], m[memoryPMEM], m[memoryHBM], 0
	if dram > 0 || pmem > 0 || hbm > 0 {
		if dram > 0 {
			mem += "DRAM " + prettyMem(dram)
			sep = ", "
			types++
		}
		if pmem > 0 {
			mem += sep + "PMEM " + prettyMem(pmem)
			sep = ", "
			types++
		}
		if hbm > 0 {
			mem += sep + "HBM " + prettyMem(hbm)
			types++
		}
		if types > 1 {
			mem += sep + "total " + prettyMem(pmem+dram+hbm)
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
	return newSupply(cs.node, cs.isolated, cs.reserved, cs.sharable, cs.grantedReserved, cs.grantedShared, mem, grantedMem)
}

// IsolatedCpus returns the isolated CPUSet of this supply.
func (cs *supply) IsolatedCPUs() cpuset.CPUSet {
	return cs.isolated.Clone()
}

// ReservedCpus returns the reserved CPUSet of this supply.
func (cs *supply) ReservedCPUs() cpuset.CPUSet {
	return cs.reserved.Clone()
}

// SharableCpus returns the sharable CPUSet of this supply.
func (cs *supply) SharableCPUs() cpuset.CPUSet {
	return cs.sharable.Clone()
}

// GrantedReserved returns the locally granted reserved CPU capacity.
func (cs *supply) GrantedReserved() int {
	return cs.grantedReserved
}

// GrantedShared returns the locally granted sharable CPU capacity.
func (cs *supply) GrantedShared() int {
	return cs.grantedShared
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
	cs.reserved = cs.reserved.Union(mcs.reserved)
	cs.sharable = cs.sharable.Union(mcs.sharable)
	cs.grantedReserved += mcs.grantedReserved
	cs.grantedShared += mcs.grantedShared

	for key, value := range mcs.mem {
		cs.mem[key] += value
	}
	for key, value := range mcs.grantedMem {
		cs.grantedMem[key] += value
	}
}

// AssignMemory adds memory (for extra NUMA nodes assigned to a pool node).
func (cs *supply) AssignMemory(mem memoryMap) {
	for key, value := range mem {
		cs.mem[key] += value
	}
}

// AccountAllocateCPU accounts for (removes) allocated exclusive capacity from the supply.
func (cs *supply) AccountAllocateCPU(g Grant) {
	if cs.node.IsSameNode(g.GetCPUNode()) {
		return
	}
	exclusive := g.ExclusiveCPUs()
	cs.isolated = cs.isolated.Difference(exclusive)
	cs.sharable = cs.sharable.Difference(exclusive)
}

// AccountReleaseCPU accounts for (reinserts) released exclusive capacity into the supply.
func (cs *supply) AccountReleaseCPU(g Grant) {
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
}

// allocateMemory tries to fulfill the memory allocation part of a request.
func (cs *supply) allocateMemory(r Request) (memoryMap, error) {
	reqType := r.MemoryType()
	if reqType == memoryUnspec {
		reqType = memoryAll
	}

	allocated := createMemoryMap(0, 0, 0)
	requested := r.MemAmountToAllocate()
	remaining := requested

	//
	// Notes:
	//   We try to allocate PMEM, then DRAM, and finally HBM, honoring
	//   the types allowed by the request. We don't need to care about
	//   extra memory reservations for this node as all the nodes with
	//   insufficient memory have been filtered out before allocation.
	//
	//   However, for cold started containers we do check if there is
	//   enough PMEM free to accomodate the full request and bail out
	//   if that check fails.
	//

	for _, memType := range []memoryType{memoryPMEM, memoryDRAM, memoryHBM} {
		if remaining > 0 && (reqType&memType) != 0 {
			available := cs.mem[memType]

			log.Debug("%s: trying %s %s of %s available",
				r.GetContainer().PrettyName(),
				prettyMem(remaining), memType.String(), prettyMem(available))

			if remaining <= available {
				allocated[memType] = remaining
			} else {
				allocated[memType] = available
			}

			cs.grantedMem[memType] += allocated[memType]
			cs.mem[memType] -= allocated[memType]
			remaining -= allocated[memType]
		}

		if remaining > 0 {
			if r.ColdStart() > 0 && memType == memoryPMEM {
				return nil, policyError("internal error: "+
					"not enough PMEM for cold start at %s", cs.GetNode().Name())
			}
		} else {
			break
		}
	}

	if remaining > 0 {
		log.Debug("%s: %s allocation from %s fell short %s",
			r.GetContainer().PrettyName(),
			reqType.String(), cs.GetNode().Name(), prettyMem(remaining))

		for memType, amount := range allocated {
			if amount > 0 {
				cs.grantedMem[memType] -= amount
				cs.mem[memType] += amount
			}
		}

		return nil, policyError("internal error: "+
			"not enough memory at %s", cs.node.Name())
	}

	cs.grantedMem[memoryAll] += requested
	cs.mem[memoryAll] -= requested

	return allocated, nil
}

// Allocate allocates a grant from the supply.
func (cs *supply) Allocate(r Request) (Grant, error) {
	grant, err := cs.AllocateCPU(r)
	if err != nil {
		return nil, err
	}

	memory, err := cs.allocateMemory(r)
	if err != nil {
		cs.ReleaseCPU(grant)
		return nil, err
	}

	grant.SetMemoryAllocation(r.MemoryType(), memory, r.ColdStart())

	return grant, nil
}

// AllocateCPU allocates CPU for a grant from the supply.
func (cs *supply) AllocateCPU(r Request) (Grant, error) {
	var exclusive cpuset.CPUSet
	var err error

	cr := r.(*request)

	full := cr.full
	fraction := cr.fraction

	cpuType := cr.cpuType

	if cpuType == cpuReserved && full > 0 {
		log.Warn("exclusive reserved CPUs not supported, allocating %d full CPUs as fractions", full)
		fraction += full * 1000
		full = 0
	}

	if cpuType == cpuReserved && fraction > 0 && cs.AllocatableReservedCPU() < fraction {
		log.Warn("possible misconfiguration of reserved resources:")
		log.Warn("  %s: allocatable %s", cs.GetNode().Name(), cs.DumpAllocatable())
		log.Warn("  %s: needs %d reserved, only %d available",
			cr.GetContainer().PrettyName(), fraction, cs.AllocatableReservedCPU())
		log.Warn("  falling back to using normal unreserved CPUs instead...")
		cpuType = cpuNormal
	}

	// allocate isolated exclusive CPUs or slice them off the sharable set
	switch {
	case full > 0 && cs.isolated.Size() >= full && cr.isolate:
		exclusive, err = cs.takeCPUs(&cs.isolated, nil, full)
		if err != nil {
			return nil, policyError("internal error: "+
				"%s: can't take %d exclusive isolated CPUs from %s: %v",
				cs.node.Name(), full, cs.isolated, err)
		}

	case full > 0 && cs.AllocatableSharedCPU() > 1000*full:
		exclusive, err = cs.takeCPUs(&cs.sharable, nil, full)
		if err != nil {
			return nil, policyError("internal error: "+
				"%s: can't take %d exclusive CPUs from %s: %v",
				cs.node.Name(), full, cs.sharable, err)
		}

	case full > 0:
		return nil, policyError("internal error: "+
			"%s: can't slice %d exclusive CPUs from %s, %dm available",
			cs.node.Name(), full, cs.sharable, cs.AllocatableSharedCPU())
	}

	grant := newGrant(cs.node, cr.GetContainer(), cpuType, exclusive, 0, 0, nil, 0)
	grant.AccountAllocateCPU()

	if fraction > 0 {
		if cpuType == cpuNormal {
			// allocate requested portion of shared CPUs
			if cs.AllocatableSharedCPU() < fraction {
				cs.ReleaseCPU(grant)
				return nil, policyError("internal error: "+
					"%s: not enough %dm sharable CPU for %dm, %dm available",
					cs.node.Name(), fraction, cs.sharable, cs.AllocatableSharedCPU())
			}
			cs.grantedShared += fraction
		} else if cpuType == cpuReserved {
			// allocate requested portion of reserved CPUs
			if cs.AllocatableReservedCPU() < fraction {
				cs.ReleaseCPU(grant)
				return nil, policyError("internal error: "+
					"%s: not enough reserved CPU: %dm requested, %dm available",
					cs.node.Name(), fraction, cs.AllocatableReservedCPU())
			}
			cs.grantedReserved += fraction
		}
		grant.SetCPUPortion(fraction)
	}

	return grant, nil
}

func (cs *supply) ReallocateMemory(g Grant) error {
	log.Debug("%s: reallocating memory (%s) from %s to %s",
		g.GetContainer().PrettyName(),
		g.MemLimit().String(),
		g.GetMemoryNode().Name(),
		cs.GetNode().Name())

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
	cs.grantedReserved -= g.ReservedPortion()
	cs.grantedShared -= g.SharedPortion()

	g.AccountReleaseCPU()
}

// ReleaseMemory returns memory from the given grant to the supply.
func (cs *supply) ReleaseMemory(g Grant) {
	releasedMemory := uint64(0)

	log.Debug("%s: releasing granted memory (%s) from %s",
		g.GetContainer().PrettyName(),
		g.MemLimit().String(), cs.GetNode().Name())

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
	if mems, ok := cs.extraMemReservations[g]; ok {
		log.Debug("%s: releasing extra memory reservation (%s) from %s",
			g.GetContainer().PrettyName(), mems.String(),
			cs.GetNode().Name())
		delete(cs.extraMemReservations, g)
	}
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
	if g.CPUType() == cpuNormal {
		isolated := g.IsolatedCPUs()
		exclusive := g.ExclusiveCPUs().Difference(isolated)
		sharedPortion := g.SharedPortion()
		if !cs.isolated.Intersection(isolated).Equals(isolated) {
			return policyError("can't reserve isolated CPUs (%s) of %s from %s",
				isolated.String(), g.String(), cs.DumpAllocatable())
		}
		if !cs.sharable.Intersection(exclusive).Equals(exclusive) {
			return policyError("can't reserve exclusive CPUs (%s) of %s from %s",
				exclusive.String(), g.String(), cs.DumpAllocatable())
		}
		if cs.AllocatableSharedCPU() < 1000*exclusive.Size()+sharedPortion {
			return policyError("can't reserve %d shared CPUs of %s from %s",
				sharedPortion, g.String(), cs.DumpAllocatable())
		}
		cs.isolated = cs.isolated.Difference(isolated)
		cs.sharable = cs.sharable.Difference(exclusive)
		cs.grantedShared += sharedPortion
	} else if g.CPUType() == cpuReserved {
		sharedPortion := 1000*g.ExclusiveCPUs().Size() + g.SharedPortion()
		if sharedPortion > 0 && cs.AllocatableReservedCPU() < sharedPortion {
			return policyError("can't reserve %d reserved CPUs of %s from %s",
				sharedPortion, g.String(), cs.DumpAllocatable())
		}
		cs.grantedReserved += sharedPortion
	}

	g.AccountAllocateCPU()

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
	cset, err := cs.node.Policy().cpuAllocator.AllocateCpus(from, cnt, cpuallocator.PriorityHigh)
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
		cpu = fmt.Sprintf("isolated:%s", cpuset.ShortCPUSet(cs.isolated))
		sep = ", "
	}
	if !cs.reserved.IsEmpty() {
		cpu += sep + fmt.Sprintf("reserved:%s (%dm)", cpuset.ShortCPUSet(cs.reserved),
			1000*cs.reserved.Size())
		sep = ", "
	}
	if !cs.sharable.IsEmpty() {
		cpu += sep + fmt.Sprintf("sharable:%s (%dm)", cpuset.ShortCPUSet(cs.sharable),
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
		cpu = fmt.Sprintf("isolated:%s", cpuset.ShortCPUSet(cs.isolated))
		sep = ", "
	}
	if !cs.reserved.IsEmpty() {
		cpu += sep + fmt.Sprintf("reserved:%s (allocatable: %dm)", cpuset.ShortCPUSet(cs.reserved), cs.AllocatableReservedCPU())
		sep = ", "
		if cs.grantedReserved > 0 {
			cpu += sep + fmt.Sprintf("grantedReserved:%dm", cs.grantedReserved)
		}
	}
	local_grantedShared := cs.grantedShared
	total_grantedShared := cs.node.GrantedSharedCPU()
	if !cs.sharable.IsEmpty() {
		cpu += sep + fmt.Sprintf("sharable:%s (", cpuset.ShortCPUSet(cs.sharable))
		sep = ""
		if local_grantedShared > 0 || total_grantedShared > 0 {
			cpu += fmt.Sprintf("grantedShared:")
			kind := ""
			if local_grantedShared > 0 {
				cpu += fmt.Sprintf("%dm", local_grantedShared)
				kind = "local"
				sep = "/"
			}
			if total_grantedShared > 0 {
				cpu += sep + fmt.Sprintf("%dm", total_grantedShared)
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

// prettyMem formats the given amount as k, M, G, or T units.
func prettyMem(value uint64) string {
	units := []string{"k", "M", "G", "T"}
	coeffs := []uint64{1 << 10, 1 << 20, 1 << 30, 1 << 40}

	c, u := uint64(1), ""
	for i := 0; i < len(units); i++ {
		if coeffs[i] > value {
			break
		}
		c, u = coeffs[i], units[i]
	}
	v := float64(value) / float64(c)

	return strconv.FormatFloat(v, 'f', 2, 64) + u
}

// DumpMemoryState dumps the state of the available and allocated memory.
func (cs *supply) DumpMemoryState(prefix string) {
	memTypes := []memoryType{memoryDRAM, memoryPMEM, memoryHBM}
	totalFree := uint64(0)
	totalGranted := uint64(0)
	for _, kind := range memTypes {
		free := cs.mem[kind]
		granted := cs.grantedMem[kind]
		if free != 0 || granted != 0 {
			log.Debug(prefix+"- %s: free: %s, granted %s",
				kind, prettyMem(free), prettyMem(granted))
		}
		totalFree += free
		totalGranted += granted
	}
	log.Debug(prefix+"- total free: %s, total granted %s",
		prettyMem(totalFree), prettyMem(totalGranted))

	printHdr := true
	if len(cs.extraMemReservations) > 0 {
		for g, memMap := range cs.extraMemReservations {
			split := ""
			sep := ""
			total := uint64(0)
			if mem := memMap[memoryDRAM]; mem > 0 {
				split = "DRAM " + prettyMem(mem)
				sep = ", "
				total += mem
			}
			if mem := memMap[memoryPMEM]; mem > 0 {
				split += sep + "PMEM " + prettyMem(mem)
				sep = ", "
				total += mem
			}
			if mem := memMap[memoryHBM]; mem > 0 {
				split += sep + "HBMEM " + prettyMem(mem)
				sep = ", "
				total += mem
			}
			if total > 0 {
				if printHdr {
					log.Debug(prefix + "- extra reservations:")
					printHdr = false
				}
				log.Debug(prefix+"  - %s: %s (%s)",
					g.GetContainer().PrettyName(), prettyMem(total), split)
			}
		}
	}
}

// newRequest creates a new request for the given container.
func newRequest(container cache.Container) Request {
	pod, _ := container.GetPod()
	full, fraction, isolate, cpuType := cpuAllocationPreferences(pod, container)
	req, lim, mtype := memoryAllocationPreference(pod, container)
	coldStart := time.Duration(0)

	log.Debug("%s: CPU preferences: cpuType=%s, full=%v, fraction=%v, isolate=%v",
		container.PrettyName(), cpuType, full, fraction, isolate)

	if mtype == memoryUnspec {
		mtype = defaultMemoryType
	}

	if mtype&memoryPMEM != 0 && mtype&memoryDRAM != 0 {
		parsedColdStart, err := coldStartPreference(pod, container)
		if err != nil {
			log.Error("Failed to parse cold start preference")
		} else {
			if parsedColdStart.Duration > 0 {
				if coldStartOff {
					log.Error("coldstart disabled (movable non-DRAM memory zones present)")
				} else {
					coldStart = time.Duration(parsedColdStart.Duration)
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
		cpuType:   cpuType,
		memReq:    req,
		memLim:    lim,
		memType:   mtype,
		coldStart: coldStart,
	}
}

// GetContainer returns the container requesting CPU.
func (cr *request) GetContainer() cache.Container {
	return cr.container
}

// String returns aprintable representation of the CPU request.
func (cr *request) String() string {
	mem := "<Memory request: limit:" + prettyMem(cr.memLim) + ", req:" + prettyMem(cr.memReq) + ">"
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

// CPUType returns the requested type of CPU for the grant.
func (cr *request) CPUType() cpuClass {
	return cr.cpuType
}

// SetCPUType sets the requested type of CPU for the grant.
func (cr *request) SetCPUType(cpuType cpuClass) {
	cr.cpuType = cpuType
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

	score.reserved = cs.AllocatableReservedCPU()
	score.shared = cs.AllocatableSharedCPU()

	if cr.CPUType() == cpuReserved {
		// calculate free reserved capacity
		score.reserved -= part
	} else {
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
	}

	// calculate colocation score
	for _, grant := range cs.node.Policy().allocations.grants {
		if cr.CPUType() == grant.CPUType() && grant.GetCPUNode().NodeID() == cs.node.NodeID() {
			score.colocated++
		}
	}

	// calculate real hint scores
	hints := cr.container.GetTopologyHints()
	score.hints = make(map[string]float64, len(hints))

	for provider, hint := range cr.container.GetTopologyHints() {
		if provider == topology.ProviderKubelet {
			log.Warn(" - ignoring topology pseudo-hint from kubelet allocation %s", hint)
			continue
		}
		log.Debug(" - evaluating topology hint %s", hint)
		score.hints[provider] = cs.node.HintScore(hint)
	}

	return score
}

// AllocatableReservedCPU calculates the allocatable amount of reserved CPU of this supply.
func (cs *supply) AllocatableReservedCPU() int {
	if cs.reserved.Size() == 0 {
		// This supply has no room for reserved (not even of zero-sized)
		return -1
	}
	reserved := 1000*cs.reserved.Size() - cs.node.GrantedReservedCPU()
	for node := cs.node.Parent(); !node.IsNil(); node = node.Parent() {
		pSupply := node.FreeSupply()
		pReserved := 1000*pSupply.ReservedCPUs().Size() - pSupply.GetNode().GrantedReservedCPU()
		if pReserved < reserved {
			reserved = pReserved
		}
	}
	return reserved
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

func (score *score) ReservedCapacity() int {
	return score.reserved
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
	return fmt.Sprintf("<CPU score: node %s, isolated:%d, reserved:%d, shared:%d, colocated:%d, hints: %v>",
		score.supply.GetNode().Name(), score.isolated, score.reserved, score.shared, score.colocated, score.hints)
}

// newGrant creates a CPU grant from the given node for the container.
func newGrant(n Node, c cache.Container, cpuType cpuClass, exclusive cpuset.CPUSet, cpuPortion int, mt memoryType, allocated memoryMap, coldstart time.Duration) Grant {
	grant := &grant{
		node:       n,
		memoryNode: n,
		container:  c,
		cpuType:    cpuType,
		exclusive:  exclusive,
		cpuPortion: cpuPortion,
	}
	if allocated != nil {
		grant.SetMemoryAllocation(mt, allocated, coldstart)
	}
	return grant
}

// SetCPUPortion sets the fractional CPU portion for the grant.
func (cg *grant) SetCPUPortion(fraction int) {
	cg.cpuPortion = fraction
}

// SetMemoryAllocation sets the memory allocation for the grant.
func (cg *grant) SetMemoryAllocation(mt memoryType, allocated memoryMap, coldstart time.Duration) {
	initial := memoryPMEM
	if coldstart <= 0 {
		initial = mt
	}
	mems := cg.node.GetMemset(initial)
	if mems.Size() == 0 {
		mems = cg.node.GetMemset(memoryDRAM)
		if mems.Size() == 0 {
			mems = cg.node.GetMemset(memoryAll)
		}
	}
	mems = mems.Clone()

	cg.memType = mt
	cg.memset = mems
	cg.allocatedMem = allocated
	cg.coldStart = coldstart
}

// Clone creates a copy of this grant.
func (cg *grant) Clone() Grant {
	return &grant{
		node:         cg.GetCPUNode(),
		memoryNode:   cg.GetMemoryNode(),
		container:    cg.GetContainer(),
		exclusive:    cg.ExclusiveCPUs(),
		cpuType:      cg.CPUType(),
		cpuPortion:   cg.SharedPortion(),
		memType:      cg.MemoryType(),
		memset:       cg.Memset().Clone(),
		allocatedMem: cg.MemLimit(),
		coldStart:    cg.ColdStart(),
	}
}

// RefetchNodes updates the stored cpu and memory nodes of this grant by name.
func (cg *grant) RefetchNodes() error {
	node, ok := cg.node.Policy().nodes[cg.node.Name()]
	if !ok {
		return policyError("failed to refetch grant cpu node %s", cg.node.Name())
	}
	memoryNode, ok := cg.memoryNode.Policy().nodes[cg.memoryNode.Name()]
	if !ok {
		return policyError("failed to refetch grant memory node %s", cg.memoryNode.Name())
	}
	cg.node = node
	cg.memoryNode = memoryNode
	return nil
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

// CPUType returns the requested type of CPU for the grant.
func (cg *grant) CPUType() cpuClass {
	return cg.cpuType
}

// CPUPortion returns granted milli-CPUs of non-full CPUs of CPUType().
func (cg *grant) CPUPortion() int {
	return cg.cpuPortion
}

// ExclusiveCPUs returns the non-isolated exclusive CPUSet in this grant.
func (cg *grant) ExclusiveCPUs() cpuset.CPUSet {
	return cg.exclusive
}

// ReservedCPUs returns the reserved CPUSet in the supply of this grant.
func (cg *grant) ReservedCPUs() cpuset.CPUSet {
	return cg.node.GetSupply().ReservedCPUs()
}

// ReservedPortion returns the milli-CPU allocation for the reserved CPUSet in this grant.
func (cg *grant) ReservedPortion() int {
	if cg.cpuType == cpuReserved {
		return cg.cpuPortion
	}
	return 0
}

// SharedCPUs returns the shared CPUSet in the supply of this grant.
func (cg *grant) SharedCPUs() cpuset.CPUSet {
	return cg.node.FreeSupply().SharableCPUs()
}

// SharedPortion returns the milli-CPU allocation for the shared CPUSet in this grant.
func (cg *grant) SharedPortion() int {
	if cg.cpuType == cpuNormal {
		return cg.cpuPortion
	}
	return 0
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
func (cg *grant) Memset() idset.IDSet {
	return cg.memset
}

// MemLimit returns the granted memory.
func (cg *grant) MemLimit() memoryMap {
	return cg.allocatedMem
}

// String returns a printable representation of the CPU grant.
func (cg *grant) String() string {
	var cpuType, isolated, exclusive, reserved, shared string
	cpuType = fmt.Sprintf("cputype: %s", cg.cpuType)
	isol := cg.IsolatedCPUs()
	if !isol.IsEmpty() {
		isolated = fmt.Sprintf(", isolated: %s", isol)
	}
	if !cg.exclusive.IsEmpty() {
		exclusive = fmt.Sprintf(", exclusive: %s", cg.exclusive)
	}
	if cg.ReservedPortion() > 0 {
		reserved = fmt.Sprintf(", reserved: %s (%dm)",
			cg.node.FreeSupply().ReservedCPUs(), cg.ReservedPortion())
	}
	if cg.SharedPortion() > 0 {
		shared = fmt.Sprintf(", shared: %s (%dm)",
			cg.node.FreeSupply().SharableCPUs(), cg.SharedPortion())
	}

	mem := cg.allocatedMem.String()
	if mem != "" {
		mem = ", MemLimit: " + mem
	}

	return fmt.Sprintf("<grant for %s from %s: %s%s%s%s%s%s>",
		cg.container.PrettyName(), cg.node.Name(), cpuType, isolated, exclusive, reserved, shared, mem)
}

func (cg *grant) AccountAllocateCPU() {
	cg.node.DepthFirst(func(n Node) error {
		n.FreeSupply().AccountAllocateCPU(cg)
		return nil
	})
	for node := cg.node.Parent(); !node.IsNil(); node = node.Parent() {
		node.FreeSupply().AccountAllocateCPU(cg)
	}
}

func (cg *grant) Release() {
	cg.GetCPUNode().FreeSupply().ReleaseCPU(cg)
	cg.GetMemoryNode().FreeSupply().ReleaseMemory(cg)
	cg.StopTimer()
}

func (cg *grant) AccountReleaseCPU() {
	cg.node.DepthFirst(func(n Node) error {
		n.FreeSupply().AccountReleaseCPU(cg)
		return nil
	})
	for node := cg.node.Parent(); !node.IsNil(); node = node.Parent() {
		node.FreeSupply().AccountReleaseCPU(cg)
	}
}

func (cg *grant) RestoreMemset() {
	mems := cg.GetMemoryNode().GetMemset(cg.memType)
	cg.memset = mems
	cg.GetMemoryNode().Policy().applyGrant(cg)
}

func (cg *grant) ExpandMemset() (bool, error) {
	supply := cg.GetMemoryNode().FreeSupply()
	node := cg.GetMemoryNode()
	parent := node.Parent()

	// We have to assume that the memory has been allocated how we granted it (if PMEM ran out
	// the allocations have been made from DRAM and so on).

	// Figure out if there is enough memory now to have grant as-is.
	extra := supply.ExtraMemoryReservation(memoryAll)
	free := supply.MemoryLimit()[memoryAll]
	if extra <= free {
		// The grant fits in the node even with extra reservations
		return false, nil
	}
	// Else it doesn't fit, so move the grant up in the memory tree.
	required := uint64(0)
	for _, memType := range []memoryType{memoryPMEM, memoryDRAM, memoryHBM} {
		required += cg.MemLimit()[memType]
	}
	log.Debug("out-of-memory risk in %s: extra reservations %s > free %s -> moving up %s total memory grant from %s",
		cg, prettyMem(extra), prettyMem(free), prettyMem(required), node.Name())

	// Find an ancestor where the grant fits. As reservations in
	// child nodes do not show up in free + extra in parent nodes,
	// releasing the grant is not necessary before searching.
	for ; !parent.IsNil(); parent = parent.Parent() {
		pSupply := parent.FreeSupply()
		parentFree := pSupply.MemoryLimit()[memoryAll]
		parentExtra := pSupply.ExtraMemoryReservation(memoryAll)
		if parentExtra+required <= parentFree {
			required = 0
			break
		}
		log.Debug("- %s has %s free but %s extra reservations, moving further up",
			parent.Name(), prettyMem(parentFree), prettyMem(parentExtra))
	}
	if required > 0 {
		return false, fmt.Errorf("internal error: cannot find enough memory (%s) for %s from ancestors of %s", prettyMem(required), cg, node.Name())
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
