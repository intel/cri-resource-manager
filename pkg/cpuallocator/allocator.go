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
	"flag"
	"fmt"
	"sort"
	"sync"

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
	debugFlag = "cpu-allocator-debug"
)

// CpuAllocator encapsulates state for allocating CPUs.
type CpuAllocator struct {
	logger.Logger               // allocator logger instance
	sys           *sysfs.System // sysfs CPU and topology information
	flags         AllocFlag     // allocation preferences
	from          cpuset.CPUSet // set of CPUs to allocate from
	cnt           int           // number of CPUs to allocate
	result        cpuset.CPUSet // set of CPUs allocated
	offline       cpuset.CPUSet // set of CPUs currently offline

	pkgs []sysfs.Package // physical CPU packages, sorted by preference
	cpus []sysfs.Cpu     // CPU cores, sorted by preference
}

// A singleton wrapper around sysfs.System.
type sysfsSingleton struct {
	sync.Once               // we do discovery only once
	sys       *sysfs.System // wrapped sysfs.System instance
	err       error         // error during recovery
	cpusets   struct {      // cached cpusets per
		pkg  map[sysfs.Id]cpuset.CPUSet // package,
		node map[sysfs.Id]cpuset.CPUSet // node, and
		core map[sysfs.Id]cpuset.CPUSet // CPU core
	}
}

var system sysfsSingleton
var debug bool

func init() {
	flag.BoolVar(&debug, debugFlag, false, "enable CPU allocator debug log")
}

// IdFilter helps filtering Ids.
type IdFilter func(sysfs.Id) bool

// IdSorter helps sorting Ids.
type IdSorter func(int, int) bool

// our logger instance
var log = logger.NewLogger(logSource)

// Get/discover sysfs.System.
func (s *sysfsSingleton) get() (*sysfs.System, error) {
	s.Do(func() {
		s.sys, s.err = sysfs.DiscoverSystem(sysfs.DiscoverCpuTopology)
		s.cpusets.pkg = make(map[sysfs.Id]cpuset.CPUSet)
		s.cpusets.node = make(map[sysfs.Id]cpuset.CPUSet)
		s.cpusets.core = make(map[sysfs.Id]cpuset.CPUSet)
	})

	return s.sys, s.err
}

// PackageCPUSet gets the CPUSet for the given package.
func (s *sysfsSingleton) PackageCPUSet(id sysfs.Id) cpuset.CPUSet {
	if cset, ok := s.cpusets.pkg[id]; ok {
		return cset
	}

	cset := s.sys.Package(id).CPUSet()
	s.cpusets.pkg[id] = cset

	return cset
}

// NodeCPUSet gets the CPUSet for the given node.
func (s *sysfsSingleton) NodeCPUSet(id sysfs.Id) cpuset.CPUSet {
	if cset, ok := s.cpusets.node[id]; ok {
		return cset
	}

	cset := s.sys.Node(id).CPUSet()
	s.cpusets.node[id] = cset

	return cset
}

// CoreCPUSet gets the CPUSet for the given core.
func (s *sysfsSingleton) CoreCPUSet(id sysfs.Id) cpuset.CPUSet {
	if cset, ok := s.cpusets.core[id]; ok {
		return cset
	}

	cset := s.sys.Cpu(id).ThreadCPUSet()
	for _, cid := range cset.ToSlice() {
		s.cpusets.core[sysfs.Id(cid)] = cset
	}

	return cset
}

// Pick packages, nodes or CPUs by filtering according to a function.
func (s *sysfsSingleton) pick(idSlice []sysfs.Id, f IdFilter) []sysfs.Id {
	ids := make([]sysfs.Id, len(idSlice))

	idx := 0
	for _, id := range idSlice {
		if f == nil || f(id) {
			ids[idx] = id
			idx++
		}
	}

	return ids[0:idx]
}

// NewCpuAllocator creates a new CPU allocator.
func NewCpuAllocator(sys *sysfs.System) *CpuAllocator {
	if sys == nil {
		sys, _ = system.get()
	}
	if sys == nil {
		return nil
	}

	a := &CpuAllocator{
		Logger: log,
		sys:    sys,
		flags:  AllocDefault,
	}

	return a
}

func (a *CpuAllocator) debug(format string, args ...interface{}) {
	if !debug {
		return
	}

	log.Info(format, args...)
}

// Allocate full idle CPU packages.
func (a *CpuAllocator) takeIdlePackages() {
	a.Debug("* takeIdlePackages()...")

	offline := a.sys.Offlined()

	// pick idle packages
	pkgs := system.pick(a.sys.PackageIds(),
		func(id sysfs.Id) bool {
			cset := system.PackageCPUSet(id).Difference(offline)
			return cset.Intersection(a.from).Equals(cset)
		})

	// sorted by id
	sort.Slice(pkgs,
		func(i, j int) bool {
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
func (a *CpuAllocator) takeIdleCores() {
	a.Debug("* takeIdleCores()...")

	offline := a.sys.Offlined()

	// pick (first id for all) idle cores
	cores := system.pick(a.sys.CpuIds(),
		func(id sysfs.Id) bool {
			cset := system.CoreCPUSet(id).Difference(offline)
			return cset.Intersection(a.from).Equals(cset) && cset.ToSlice()[0] == int(id)
		})

	// sorted by id
	sort.Slice(cores,
		func(i, j int) bool {
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
func (a *CpuAllocator) takeIdleThreads() {
	offline := a.sys.Offlined()

	// pick all threads with free capacity
	cores := system.pick(a.sys.CpuIds(),
		func(id sysfs.Id) bool {
			return a.from.Difference(offline).Contains(int(id))
		})

	a.Debug(" => idle threads unsorted: %v", cores)

	// sorted for preference by id, mimicking cpus_assignment.go for now:
	//   IOW, prefer CPUs
	//     - from packages with higher number of CPUs/cores already in a.result
	//     - from packages with fewer remaining free CPUs/cores in a.from
	//     - from cores with fewer remaining free CPUs/cores in a.from
	//     - from packages with lower id
	//     - with lower id
	sort.Slice(cores,
		func(i, j int) bool {
			iCore := cores[i]
			jCore := cores[j]
			iPkg := a.sys.Cpu(iCore).PackageId()
			jPkg := a.sys.Cpu(jCore).PackageId()

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

			// prefer CPUs from packages with
			//   - higher number of CPUs/cores already in a.result, and
			//   - fewer remaining free CPUs/cores in a.from
			//   - from cores with fewer remaining CPUs/cores in a.from
			//   - lower id

			switch {
			case iPkgColo != jPkgColo:
				return iPkgColo > jPkgColo
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

// Perform CPU allocation.
func (a *CpuAllocator) allocate() cpuset.CPUSet {
	if (a.flags & AllocIdlePackages) != 0 {
		a.takeIdlePackages()

		if a.cnt == 0 {
			return a.result
		}
	}

	if (a.flags & AllocIdleCores) != 0 {
		a.takeIdleCores()

		if a.cnt == 0 {
			return a.result
		}
	}

	a.takeIdleThreads()
	if a.cnt == 0 {
		return a.result
	}

	return cpuset.NewCPUSet()
}

func allocateCpus(from *cpuset.CPUSet, cnt int) (cpuset.CPUSet, error) {
	var result cpuset.CPUSet
	var err error

	switch {
	case from.Size() < cnt:
		result, err = cpuset.NewCPUSet(), fmt.Errorf("cpuset %s does not have %d CPUs", from, cnt)
	case from.Size() == cnt:
		result, err, *from = from.Clone(), nil, cpuset.NewCPUSet()
	default:
		a := NewCpuAllocator(nil)
		a.from = from.Clone()
		a.cnt = cnt

		result, err, *from = a.allocate(), nil, a.from.Clone()

		a.Debug("allocateCpus(#%s, %d) => #%s", from.Union(result).String(), cnt, result)
	}

	return result, err
}

// AllocateCpus allocates a number of CPUs from the given set.
func AllocateCpus(from *cpuset.CPUSet, cnt int) (cpuset.CPUSet, error) {
	result, err := allocateCpus(from, cnt)
	return result, err
}

// ReleaseCpus releases a number of CPUs from the given set.
func ReleaseCpus(from *cpuset.CPUSet, cnt int) (cpuset.CPUSet, error) {
	var oset cpuset.CPUSet

	if debug {
		oset = from.Clone()
	}

	result, err := allocateCpus(from, from.Size()-cnt)

	if debug {
		log.Info("ReleaseCpus(#%s, %d) => kept: #%s, released: #%s", oset.String(), cnt,
			from.String(), result)
	}

	return result, err
}
