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

package cpuallocator

import (
	"fmt"
	"sort"

	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"

	logger "github.com/intel/cri-resource-manager/pkg/log"
	"github.com/intel/cri-resource-manager/pkg/sysfs"
)

// AllocFlag represents CPU allocation preferences.
type AllocFlag uint

const (
	// AllocIdlePackages requests allocation of full idle packages.
	AllocIdlePackages AllocFlag = 1 << iota
	// AllocIdleNodes requests allocation of full idle NUMA nodes.
	AllocIdleNodes
	// AllocIdleCores requests allocation of full idle cores (all threads in core).
	AllocIdleCores
	// AllocDefault is the default allocation preferences.
	AllocDefault = AllocIdlePackages | AllocIdleCores

	logSource = "cpuallocator"
)

// allocatorHelper encapsulates state for allocating CPUs.
type allocatorHelper struct {
	logger.Logger               // allocatorHelper logger instance
	sys           sysfs.System  // sysfs CPU and topology information
	topology      topologyCache // cached topology information
	flags         AllocFlag     // allocation preferences
	from          cpuset.CPUSet // set of CPUs to allocate from
	preferred     cpuset.CPUSet // set of preferred CPUs
	cnt           int           // number of CPUs to allocate
	result        cpuset.CPUSet // set of CPUs allocated

	pkgs []sysfs.CPUPackage // physical CPU packages, sorted by preference
	cpus []sysfs.CPU        // CPU cores, sorted by preference
}

// CPUAllocator is an interface for a generic CPU allocator
type CPUAllocator interface {
	AllocateCpus(*cpuset.CPUSet, int, bool) (cpuset.CPUSet, error)
	ReleaseCpus(*cpuset.CPUSet, int, bool) (cpuset.CPUSet, error)
}

type cpuAllocator struct {
	logger.Logger
	sys           sysfs.System  // wrapped sysfs.System instance
	topologyCache topologyCache // topology lookups
	priorityCpus  cpuset.CPUSet // set of CPUs having higher priority
}

// topologyCache caches topology lookups
type topologyCache struct {
	pkg  map[sysfs.ID]cpuset.CPUSet
	node map[sysfs.ID]cpuset.CPUSet
	core map[sysfs.ID]cpuset.CPUSet
}

// IDFilter helps filtering Ids.
type IDFilter func(sysfs.ID) bool

// IDSorter helps sorting Ids.
type IDSorter func(int, int) bool

// our logger instance
var log = logger.NewLogger(logSource)

// NewCPUAllocator return a new cpuAllocator instance
func NewCPUAllocator(sys sysfs.System) CPUAllocator {
	ca := cpuAllocator{
		Logger:        log,
		sys:           sys,
		topologyCache: newTopologyCache(sys),
	}

	ca.discoverPriorityCpus()

	return &ca
}

// Pick packages, nodes or CPUs by filtering according to a function.
func pickIds(idSlice []sysfs.ID, f IDFilter) []sysfs.ID {
	ids := make([]sysfs.ID, len(idSlice))

	idx := 0
	for _, id := range idSlice {
		if f == nil || f(id) {
			ids[idx] = id
			idx++
		}
	}

	return ids[0:idx]
}

func (ca *cpuAllocator) discoverPriorityCpus() {
	ca.priorityCpus = cpuset.NewCPUSet()
	if ca.sys == nil {
		return
	}

	// Group cpus by base frequency
	freqs := map[uint64][]sysfs.ID{}
	for _, id := range ca.sys.CPUIDs() {
		bf := ca.sys.CPU(id).BaseFrequency()
		freqs[bf] = append(freqs[bf], id)
	}

	// Construct a sorted list of detected frequencies
	freqList := []uint64{}
	for freq := range freqs {
		if freq > 0 {
			freqList = append(freqList, freq)
		}
	}
	sort.Slice(freqList, func(i, j int) bool {
		return freqList[i] < freqList[j]
	})

	// All cpus NOT in the lowest base frequency bin are considered high prio
	if len(freqList) > 0 {
		priorityCpus := []sysfs.ID{}
		for _, freq := range freqList[1:] {
			priorityCpus = append(priorityCpus, freqs[freq]...)
		}
		ca.priorityCpus = sysfs.NewIDSet(priorityCpus...).CPUSet()
	}
	if ca.priorityCpus.Size() > 0 {
		log.Debug("discovered high priority cpus: %v", ca.priorityCpus)
	}
}

// newAllocatorHelper creates a new CPU allocatorHelper.
func newAllocatorHelper(sys sysfs.System, topo topologyCache) *allocatorHelper {
	a := &allocatorHelper{
		Logger:   log,
		sys:      sys,
		topology: topo,
		flags:    AllocDefault,
	}

	return a
}

// Allocate full idle CPU packages.
func (a *allocatorHelper) takeIdlePackages() {
	a.Debug("* takeIdlePackages()...")

	offline := a.sys.Offlined()

	// pick idle packages
	pkgs := pickIds(a.sys.PackageIDs(),
		func(id sysfs.ID) bool {
			cset := a.topology.pkg[id].Difference(offline)
			return cset.Intersection(a.from).Equals(cset)
		})

	// sorted by number of preferred cpus and then by cpu id
	sort.Slice(pkgs,
		func(i, j int) bool {
			iPref := a.topology.pkg[pkgs[i]].Intersection(a.preferred).Size()
			jPref := a.topology.pkg[pkgs[j]].Intersection(a.preferred).Size()
			if iPref != jPref {
				return iPref > jPref
			}
			return pkgs[i] < pkgs[j]
		})

	a.Debug(" => idle packages sorted by preference: %v", pkgs)

	// take as many idle packages as we need/can
	for _, id := range pkgs {
		cset := a.topology.pkg[id].Difference(offline)
		a.Debug(" => considering package %v (#%s)...", id, cset)
		if a.cnt >= cset.Size() {
			a.Debug(" => taking pakcage %v...", id)
			a.result = a.result.Union(cset)
			a.from = a.from.Difference(cset)
			a.cnt -= cset.Size()

			if a.cnt == 0 {
				break
			}
		}
	}
}

// Allocate full idle CPU cores.
func (a *allocatorHelper) takeIdleCores() {
	a.Debug("* takeIdleCores()...")

	offline := a.sys.Offlined()

	// pick (first id for all) idle cores
	cores := pickIds(a.sys.CPUIDs(),
		func(id sysfs.ID) bool {
			cset := a.topology.core[id].Difference(offline)
			return cset.Intersection(a.from).Equals(cset) && cset.ToSlice()[0] == int(id)
		})

	// sorted by id
	sort.Slice(cores,
		func(i, j int) bool {
			iPref := a.topology.core[cores[i]].Intersection(a.preferred).Size()
			jPref := a.topology.core[cores[j]].Intersection(a.preferred).Size()
			if iPref != jPref {
				return iPref > jPref
			}
			return cores[i] < cores[j]
		})

	a.Debug(" => idle cores sorted by preference: %v", cores)

	// take as many idle cores as we can
	for _, id := range cores {
		cset := a.topology.core[id].Difference(offline)
		a.Debug(" => considering core %v (#%s)...", id, cset)
		if a.cnt >= cset.Size() {
			a.Debug(" => taking core %v...", id)
			a.result = a.result.Union(cset)
			a.from = a.from.Difference(cset)
			a.cnt -= cset.Size()

			if a.cnt == 0 {
				break
			}
		}
	}
}

// Allocate idle CPU hyperthreads.
func (a *allocatorHelper) takeIdleThreads() {
	offline := a.sys.Offlined()

	// pick all threads with free capacity
	cores := pickIds(a.sys.CPUIDs(),
		func(id sysfs.ID) bool {
			return a.from.Difference(offline).Contains(int(id))
		})

	a.Debug(" => idle threads unsorted: %v", cores)

	// sorted for preference by id, mimicking cpus_assignment.go for now:
	//   IOW, prefer CPUs
	//     - from packages with higher number of CPUs/cores already in a.result
	//     - from the list of preferred cpus
	//     - from packages with fewer remaining free CPUs/cores in a.from
	//     - from cores with fewer remaining free CPUs/cores in a.from
	//     - from packages with lower id
	//     - with lower id
	sort.Slice(cores,
		func(i, j int) bool {
			iCore := cores[i]
			jCore := cores[j]
			iPkg := a.sys.CPU(iCore).PackageID()
			jPkg := a.sys.CPU(jCore).PackageID()

			iCoreSet := a.topology.core[iCore]
			jCoreSet := a.topology.core[jCore]
			iPkgSet := a.topology.pkg[iPkg]
			jPkgSet := a.topology.pkg[jPkg]

			iPkgColo := iPkgSet.Intersection(a.result).Size()
			jPkgColo := jPkgSet.Intersection(a.result).Size()

			iPkgFree := iPkgSet.Intersection(a.from).Size()
			jPkgFree := jPkgSet.Intersection(a.from).Size()

			iCoreFree := iCoreSet.Intersection(a.from).Size()
			jCoreFree := jCoreSet.Intersection(a.from).Size()

			iPreferred := a.preferred.Contains(int(cores[i]))
			jPreferred := a.preferred.Contains(int(cores[j]))

			switch {
			case iPkgColo != jPkgColo:
				return iPkgColo > jPkgColo
			case iPreferred != jPreferred:
				return iPreferred
			case iPkgFree != jPkgFree:
				return iPkgFree < jPkgFree
			case iCoreFree != jCoreFree:
				return iCoreFree < jCoreFree
			default:
				return iCore < jCore
			}
		})

	a.Debug(" => idle threads sorted: %v", cores)

	// take as many idle cores as we can
	for _, id := range cores {
		cset := a.topology.core[id].Difference(offline)
		a.Debug(" => considering thread %v (#%s)...", id, cset)
		cset = cpuset.NewCPUSet(int(id))
		a.result = a.result.Union(cset)
		a.from = a.from.Difference(cset)
		a.cnt -= cset.Size()

		if a.cnt == 0 {
			break
		}
	}
}

// takeAny is a dummy allocator not dependent on sysfs topology information
func (a *allocatorHelper) takeAny() {
	a.Debug("* takeAnyCores()...")

	cpus := a.from.Intersection(a.preferred).ToSlice()
	cpus = append(cpus, a.from.Difference(a.preferred).ToSlice()...)

	if len(cpus) >= a.cnt {
		cset := cpuset.NewCPUSet(cpus[0:a.cnt]...)
		a.result = a.result.Union(cset)
		a.from = a.from.Difference(cset)
		a.cnt = 0
	}
}

// Perform CPU allocation.
func (a *allocatorHelper) allocate() cpuset.CPUSet {
	if a.sys != nil {
		if (a.flags & AllocIdlePackages) != 0 {
			a.takeIdlePackages()
		}
		if a.cnt > 0 && (a.flags&AllocIdleCores) != 0 {
			a.takeIdleCores()
		}
		if a.cnt > 0 {
			a.takeIdleThreads()
		}
	} else {
		a.takeAny()
	}
	if a.cnt == 0 {
		return a.result
	}

	return cpuset.NewCPUSet()
}

func (ca *cpuAllocator) allocateCpus(from *cpuset.CPUSet, cnt int, preferHighPrio bool) (cpuset.CPUSet, error) {
	var result cpuset.CPUSet
	var err error

	switch {
	case from.Size() < cnt:
		result, err = cpuset.NewCPUSet(), fmt.Errorf("cpuset %s does not have %d CPUs", from, cnt)
	case from.Size() == cnt:
		result, err, *from = from.Clone(), nil, cpuset.NewCPUSet()
	default:
		a := newAllocatorHelper(ca.sys, ca.topologyCache)
		a.from = from.Clone()
		a.cnt = cnt

		if preferHighPrio {
			a.preferred = ca.priorityCpus
		} else {
			// Try to avoid high priority cpus
			a.preferred = from.Difference(ca.priorityCpus)
		}

		result, err, *from = a.allocate(), nil, a.from.Clone()

		a.Debug("%d cpus from #%v (preferring #%v) => #%v", cnt, from.Union(result), a.preferred, result)
	}

	return result, err
}

// AllocateCpus allocates a number of CPUs from the given set.
func (ca *cpuAllocator) AllocateCpus(from *cpuset.CPUSet, cnt int, preferHighPrio bool) (cpuset.CPUSet, error) {
	result, err := ca.allocateCpus(from, cnt, preferHighPrio)
	return result, err
}

// ReleaseCpus releases a number of CPUs from the given set.
func (ca *cpuAllocator) ReleaseCpus(from *cpuset.CPUSet, cnt int, preferHighPrio bool) (cpuset.CPUSet, error) {
	oset := from.Clone()

	result, err := ca.allocateCpus(from, from.Size()-cnt, preferHighPrio)

	ca.Debug("ReleaseCpus(#%s, %d) => kept: #%s, released: #%s", oset, cnt, from, result)

	return result, err
}

func newTopologyCache(sys sysfs.System) topologyCache {
	c := topologyCache{
		pkg:  make(map[sysfs.ID]cpuset.CPUSet),
		node: make(map[sysfs.ID]cpuset.CPUSet),
		core: make(map[sysfs.ID]cpuset.CPUSet)}
	if sys != nil {
		for _, id := range sys.PackageIDs() {
			c.pkg[id] = sys.Package(id).CPUSet()
		}
		for _, id := range sys.NodeIDs() {
			c.node[id] = sys.Node(id).CPUSet()
		}
		for _, id := range sys.CPUIDs() {
			c.core[id] = sys.CPU(id).ThreadCPUSet()
		}
	}
	return c
}
