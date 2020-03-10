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

// CPUAllocator encapsulates state for allocating CPUs.
type CPUAllocator struct {
	logger.Logger               // allocator logger instance
	sys           sysfs.System  // sysfs CPU and topology information
	flags         AllocFlag     // allocation preferences
	from          cpuset.CPUSet // set of CPUs to allocate from
	preferred     cpuset.CPUSet // set of preferred CPUs
	cnt           int           // number of CPUs to allocate
	result        cpuset.CPUSet // set of CPUs allocated

	pkgs []sysfs.CPUPackage // physical CPU packages, sorted by preference
	cpus []sysfs.CPU        // CPU cores, sorted by preference
}

// A singleton wrapper around sysfs.System.
type sysfsSingleton struct {
	sys     sysfs.System // wrapped sysfs.System instance
	err     error        // error during recovery
	cpusets struct {     // cached cpusets per
		pkg  map[sysfs.ID]cpuset.CPUSet // package,
		node map[sysfs.ID]cpuset.CPUSet // node, and
		core map[sysfs.ID]cpuset.CPUSet // CPU core
	}
	priorityCpus cpuset.CPUSet // set of CPUs having higher priority
}

var system sysfsSingleton

func init() {
	if err := system.init(); err != nil {
		log.Warn("sysfs system discovery failed: %v", err)
	}
}

// IDFilter helps filtering Ids.
type IDFilter func(sysfs.ID) bool

// IDSorter helps sorting Ids.
type IDSorter func(int, int) bool

// our logger instance
var log = logger.NewLogger(logSource)

// init does system discovery
func (s *sysfsSingleton) init() error {
	sys, err := sysfs.DiscoverSystem(sysfs.DiscoverCPUTopology)
	if err != nil {
		return err
	}
	s.sys = sys
	s.cpusets.pkg = make(map[sysfs.ID]cpuset.CPUSet)
	s.cpusets.node = make(map[sysfs.ID]cpuset.CPUSet)
	s.cpusets.core = make(map[sysfs.ID]cpuset.CPUSet)

	s.discoverPriorityCpus()

	return err
}

// PackageCPUSet gets the CPUSet for the given package.
func (s *sysfsSingleton) PackageCPUSet(id sysfs.ID) cpuset.CPUSet {
	if cset, ok := s.cpusets.pkg[id]; ok {
		return cset
	}

	cset := s.sys.Package(id).CPUSet()
	s.cpusets.pkg[id] = cset

	return cset
}

// NodeCPUSet gets the CPUSet for the given node.
func (s *sysfsSingleton) NodeCPUSet(id sysfs.ID) cpuset.CPUSet {
	if cset, ok := s.cpusets.node[id]; ok {
		return cset
	}

	cset := s.sys.Node(id).CPUSet()
	s.cpusets.node[id] = cset

	return cset
}

// CoreCPUSet gets the CPUSet for the given core.
func (s *sysfsSingleton) CoreCPUSet(id sysfs.ID) cpuset.CPUSet {
	if cset, ok := s.cpusets.core[id]; ok {
		return cset
	}

	cset := s.sys.CPU(id).ThreadCPUSet()
	for _, cid := range cset.ToSlice() {
		s.cpusets.core[sysfs.ID(cid)] = cset
	}

	return cset
}

// Pick packages, nodes or CPUs by filtering according to a function.
func (s *sysfsSingleton) pick(idSlice []sysfs.ID, f IDFilter) []sysfs.ID {
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

func (s *sysfsSingleton) discoverPriorityCpus() {
	s.priorityCpus = cpuset.NewCPUSet()
	if s.sys == nil {
		return
	}

	// Group cpus by base frequency
	freqs := map[uint64][]sysfs.ID{}
	for _, id := range s.sys.CPUIDs() {
		bf := s.sys.CPU(id).BaseFrequency()
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
		s.priorityCpus = sysfs.NewIDSet(priorityCpus...).CPUSet()
	}
	if s.priorityCpus.Size() > 0 {
		log.Debug("discovered high priority cpus: %v", s.priorityCpus)
	}
}

// NewCPUAllocator creates a new CPU allocator.
func NewCPUAllocator(sys sysfs.System) *CPUAllocator {
	if sys == nil {
		sys = system.sys
	}

	a := &CPUAllocator{
		Logger: log,
		sys:    sys,
		flags:  AllocDefault,
	}

	return a
}

// Allocate full idle CPU packages.
func (a *CPUAllocator) takeIdlePackages() {
	a.Debug("* takeIdlePackages()...")

	offline := a.sys.Offlined()

	// pick idle packages
	pkgs := system.pick(a.sys.PackageIDs(),
		func(id sysfs.ID) bool {
			cset := system.PackageCPUSet(id).Difference(offline)
			return cset.Intersection(a.from).Equals(cset)
		})

	// sorted by number of preferred cpus and then by cpu id
	sort.Slice(pkgs,
		func(i, j int) bool {
			iPref := system.PackageCPUSet(pkgs[i]).Intersection(a.preferred).Size()
			jPref := system.PackageCPUSet(pkgs[j]).Intersection(a.preferred).Size()
			if iPref != jPref {
				return iPref > jPref
			}
			return pkgs[i] < pkgs[j]
		})

	a.Debug(" => idle packages sorted by preference: %v", pkgs)

	// take as many idle packages as we need/can
	for _, id := range pkgs {
		cset := system.PackageCPUSet(id).Difference(offline)
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
func (a *CPUAllocator) takeIdleCores() {
	a.Debug("* takeIdleCores()...")

	offline := a.sys.Offlined()

	// pick (first id for all) idle cores
	cores := system.pick(a.sys.CPUIDs(),
		func(id sysfs.ID) bool {
			cset := system.CoreCPUSet(id).Difference(offline)
			return cset.Intersection(a.from).Equals(cset) && cset.ToSlice()[0] == int(id)
		})

	// sorted by id
	sort.Slice(cores,
		func(i, j int) bool {
			iPref := system.CoreCPUSet(cores[i]).Intersection(a.preferred).Size()
			jPref := system.CoreCPUSet(cores[j]).Intersection(a.preferred).Size()
			if iPref != jPref {
				return iPref > jPref
			}
			return cores[i] < cores[j]
		})

	a.Debug(" => idle cores sorted by preference: %v", cores)

	// take as many idle cores as we can
	for _, id := range cores {
		cset := system.CoreCPUSet(id).Difference(offline)
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
func (a *CPUAllocator) takeIdleThreads() {
	offline := a.sys.Offlined()

	// pick all threads with free capacity
	cores := system.pick(a.sys.CPUIDs(),
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

			iCoreSet := system.CoreCPUSet(iCore)
			jCoreSet := system.CoreCPUSet(jCore)
			iPkgSet := system.PackageCPUSet(iPkg)
			jPkgSet := system.PackageCPUSet(jPkg)

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
		cset := system.CoreCPUSet(id).Difference(offline)
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
func (a *CPUAllocator) takeAny() {
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
func (a *CPUAllocator) allocate() cpuset.CPUSet {
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

func allocateCpus(from *cpuset.CPUSet, cnt int, preferred cpuset.CPUSet) (cpuset.CPUSet, error) {
	var result cpuset.CPUSet
	var err error

	switch {
	case from.Size() < cnt:
		result, err = cpuset.NewCPUSet(), fmt.Errorf("cpuset %s does not have %d CPUs", from, cnt)
	case from.Size() == cnt:
		result, err, *from = from.Clone(), nil, cpuset.NewCPUSet()
	default:
		a := NewCPUAllocator(nil)
		a.from = from.Clone()
		a.cnt = cnt
		a.preferred = preferred

		result, err, *from = a.allocate(), nil, a.from.Clone()

		a.Debug("%d cpus from #%v (preferring #%v) => #%v", cnt, from.Union(result), preferred, result)
	}

	return result, err
}

// AllocateCpus allocates a number of CPUs from the given set.
func AllocateCpus(from *cpuset.CPUSet, cnt int, preferHighPrio bool) (cpuset.CPUSet, error) {
	preferred := system.priorityCpus
	if !preferHighPrio {
		// Try to avoid high priority cpus
		preferred = from.Difference(system.priorityCpus)
	}

	result, err := allocateCpus(from, cnt, preferred)
	return result, err
}

// ReleaseCpus releases a number of CPUs from the given set.
func ReleaseCpus(from *cpuset.CPUSet, cnt int, preferHighPrio bool) (cpuset.CPUSet, error) {
	oset := from.Clone()

	preferred := system.priorityCpus
	if !preferHighPrio {
		// Try to avoid high priority cpus
		preferred = from.Difference(system.priorityCpus)
	}

	result, err := allocateCpus(from, from.Size()-cnt, preferred)

	log.Debug("ReleaseCpus(#%s, %d) => kept: #%s, released: #%s", oset, cnt, from, result)

	return result, err
}
