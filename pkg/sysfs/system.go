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

package sysfs

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	logger "github.com/intel/cri-resource-manager/pkg/log"
	"github.com/intel/cri-resource-manager/pkg/utils"
	"github.com/intel/cri-resource-manager/pkg/utils/cpuset"
	"github.com/intel/goresctrl/pkg/sst"
	idset "github.com/intel/goresctrl/pkg/utils"
)

var (
	// Parent directory under which host sysfs, etc. is mounted (if non-standard location).
	sysRoot = ""
)

const (
	// sysfs devices/cpu subdirectory path
	sysfsCPUPath = "devices/system/cpu"
	// sysfs device/node subdirectory path
	sysfsNumaNodePath = "devices/system/node"
)

// DiscoveryFlag controls what hardware details to discover.
type DiscoveryFlag uint

const (
	// DiscoverCPUTopology requests discovering CPU topology details.
	DiscoverCPUTopology DiscoveryFlag = 1 << iota
	// DiscoverMemTopology requests discovering memory topology details.
	DiscoverMemTopology
	// DiscoverCache requests discovering CPU cache details.
	DiscoverCache
	// DiscoverSst requests discovering details of Intel Speed Select Technology
	DiscoverSst
	// DiscoverNone is the zero value for discovery flags.
	DiscoverNone DiscoveryFlag = 0
	// DiscoverAll requests full supported discovery.
	DiscoverAll DiscoveryFlag = 0xffffffff
	// DiscoverDefault is the default set of discovery flags.
	DiscoverDefault DiscoveryFlag = (DiscoverCPUTopology | DiscoverMemTopology | DiscoverSst)
)

// MemoryType is an enum for the Node memory
type MemoryType int

const (
	// MemoryTypeDRAM means that the node has regular DRAM-type memory
	MemoryTypeDRAM MemoryType = iota
	// MemoryTypePMEM means that the node has persistent memory
	MemoryTypePMEM
	// MemoryTypeHBM means that the node has high bandwidth memory
	MemoryTypeHBM
)

// System devices
type System interface {
	Discover(flags DiscoveryFlag) error
	SetCpusOnline(online bool, cpus idset.IDSet) (idset.IDSet, error)
	SetCPUFrequencyLimits(min, max uint64, cpus idset.IDSet) error
	PackageIDs() []idset.ID
	NodeIDs() []idset.ID
	CPUIDs() []idset.ID
	PackageCount() int
	SocketCount() int
	CPUCount() int
	NUMANodeCount() int
	ThreadCount() int
	CPUSet() cpuset.CPUSet
	Package(id idset.ID) CPUPackage
	Node(id idset.ID) Node
	NodeDistance(from, to idset.ID) int
	CPU(id idset.ID) CPU
	Offlined() cpuset.CPUSet
	Isolated() cpuset.CPUSet
}

// System devices
type system struct {
	logger.Logger                          // our logger instance
	flags         DiscoveryFlag            // system discovery flags
	path          string                   // sysfs mount point
	packages      map[idset.ID]*cpuPackage // physical packages
	nodes         map[idset.ID]*node       // NUMA nodes
	cpus          map[idset.ID]*cpu        // CPUs
	cache         map[idset.ID]*Cache      // Cache
	offline       idset.IDSet              // offlined CPUs
	isolated      idset.IDSet              // isolated CPUs
	threads       int                      // hyperthreads per core
}

// CPUPackage is a physical package (a collection of CPUs).
type CPUPackage interface {
	ID() idset.ID
	CPUSet() cpuset.CPUSet
	DieIDs() []idset.ID
	NodeIDs() []idset.ID
	DieNodeIDs(idset.ID) []idset.ID
	DieCPUSet(idset.ID) cpuset.CPUSet
	SstInfo() *sst.SstPackageInfo
}

type cpuPackage struct {
	id       idset.ID                 // package id
	cpus     idset.IDSet              // CPUs in this package
	nodes    idset.IDSet              // nodes in this package
	dies     idset.IDSet              // dies in this package
	dieCPUs  map[idset.ID]idset.IDSet // CPUs per die
	dieNodes map[idset.ID]idset.IDSet // NUMA nodes per die
	sstInfo  *sst.SstPackageInfo      // Speed Select Technology info
}

// Node represents a NUMA node.
type Node interface {
	ID() idset.ID
	PackageID() idset.ID
	DieID() idset.ID
	CPUSet() cpuset.CPUSet
	Distance() []int
	DistanceFrom(id idset.ID) int
	MemoryInfo() (*MemInfo, error)
	GetMemoryType() MemoryType
	HasNormalMemory() bool
}

type node struct {
	path       string      // sysfs path
	id         idset.ID    // node id
	pkg        idset.ID    // package id
	die        idset.ID    // die id
	cpus       idset.IDSet // cpus in this node
	memoryType MemoryType  // node memory type
	normalMem  bool        // node has memory in a normal (kernel space allocatable) zone
	distance   []int       // distance/cost to other NUMA nodes
}

// CPU is a CPU core.
type CPU interface {
	ID() idset.ID
	PackageID() idset.ID
	DieID() idset.ID
	NodeID() idset.ID
	CoreID() idset.ID
	ThreadCPUSet() cpuset.CPUSet
	BaseFrequency() uint64
	FrequencyRange() CPUFreq
	EPP() EPP
	Online() bool
	Isolated() bool
	SetFrequencyLimits(min, max uint64) error
	SstClos() int
}

type cpu struct {
	path     string      // sysfs path
	id       idset.ID    // CPU id
	pkg      idset.ID    // package id
	die      idset.ID    // die id
	node     idset.ID    // node id
	core     idset.ID    // core id
	threads  idset.IDSet // sibling/hyper-threads
	baseFreq uint64      // CPU base frequency
	freq     CPUFreq     // CPU frequencies
	epp      EPP         // Energy Performance Preference from cpufreq governor
	online   bool        // whether this CPU is online
	isolated bool        // whether this CPU is isolated
	sstClos  int         // SST-CP CLOS the CPU is associated with
}

// CPUFreq is a CPU frequency scaling range
type CPUFreq struct {
	min uint64   // minimum frequency (kHz)
	max uint64   // maximum frequency (kHz)
	all []uint64 // discrete set of frequencies if applicable/known
}

// EPP represents the value of a CPU energy performance profile
type EPP int

const (
	EPPPerformance EPP = iota
	EPPBalancePerformance
	EPPBalancePower
	EPPPower
	EPPUnknown
)

// MemInfo contains data read from a NUMA node meminfo file.
type MemInfo struct {
	MemTotal uint64
	MemFree  uint64
	MemUsed  uint64
}

// CPU cache.
//   Notes: cache-discovery is forced off now (by forcibly clearing the related discovery bit)
//      Can't seem to make sense of the cache information exposed under sysfs. The cache ids
//      do not seem to be unique, which IIUC is contrary to the documentation.

// CacheType specifies a cache type.
type CacheType string

const (
	// DataCache marks data cache.
	DataCache CacheType = "Data"
	// InstructionCache marks instruction cache.
	InstructionCache CacheType = "Instruction"
	// UnifiedCache marks a unified data/instruction cache.
	UnifiedCache CacheType = "Unified"
)

// Cache has details about cache.
type Cache struct {
	id    idset.ID    // cache id
	kind  CacheType   // cache type
	size  uint64      // cache size
	level uint8       // cache level
	cpus  idset.IDSet // CPUs sharing this cache
}

// SetSysRoot sets the sys root directory.
func SetSysRoot(path string) {
	sysRoot = path
}

// SysRoot returns the sys root directory.
func SysRoot() string {
	return sysRoot
}

// DiscoverSystem performs discovery of the running systems details.
func DiscoverSystem(args ...DiscoveryFlag) (System, error) {
	return DiscoverSystemAt(filepath.Join("/", sysRoot, "sys"))
}

// DiscoverSystemAt performs discovery of the running systems details from sysfs mounted at path.
func DiscoverSystemAt(path string, args ...DiscoveryFlag) (System, error) {
	var flags DiscoveryFlag

	if len(args) < 1 {
		flags = DiscoverDefault
	} else {
		flags = DiscoverNone
		for _, flag := range args {
			flags |= flag
		}
	}

	sys := &system{
		Logger:  logger.NewLogger("sysfs"),
		path:    path,
		offline: idset.NewIDSet(),
	}

	if err := sys.Discover(flags); err != nil {
		return nil, err
	}

	return sys, nil
}

// Discover performs system/hardware discovery.
func (sys *system) Discover(flags DiscoveryFlag) error {
	sys.flags |= (flags &^ DiscoverCache)

	if (sys.flags & (DiscoverCPUTopology | DiscoverCache | DiscoverSst)) != 0 {
		if err := sys.discoverCPUs(); err != nil {
			return err
		}
		if err := sys.discoverNodes(); err != nil {
			return err
		}
		if err := sys.discoverPackages(); err != nil {
			return err
		}
	}

	if (sys.flags & DiscoverSst) != 0 {
		if err := sys.discoverSst(); err != nil {
			// Just consider SST unsupported if our detection fails for some reason
			sys.Warn("%v", err)
		}
	}

	if (sys.flags & DiscoverMemTopology) != 0 {
		if err := sys.discoverNodes(); err != nil {
			return err
		}
	}

	if len(sys.nodes) > 0 {
		for _, pkg := range sys.packages {
			for _, nodeID := range pkg.nodes.SortedMembers() {
				if node, ok := sys.nodes[nodeID]; ok {
					node.pkg = pkg.id
				} else {
					return sysfsError("NUMA nodes", "can't find NUMA node for ID %d", nodeID)
				}
			}
			for _, dieID := range pkg.DieIDs() {
				for _, nodeID := range pkg.DieNodeIDs(dieID) {
					if node, ok := sys.nodes[nodeID]; ok {
						node.die = dieID
					} else {
						return sysfsError("NUMA nodes", "can't find NUMA node for ID %d", nodeID)
					}
				}
			}
		}
	}

	if sys.DebugEnabled() {
		for id, pkg := range sys.packages {
			sys.Info("package #%d:", id)
			sys.Debug("   cpus: %s", pkg.cpus)
			sys.Debug("  nodes: %s", pkg.nodes)
			sys.Debug("   dies: %s", pkg.dies)
			for die := range pkg.dies {
				sys.Debug("    die #%v nodes: %v", die, pkg.DieNodeIDs(die))
				sys.Debug("    die #%v cpus: %s", die, pkg.DieCPUSet(die).String())
			}
		}

		for id, node := range sys.nodes {
			sys.Debug("node #%d:", id)
			sys.Debug("      cpus: %s", node.cpus)
			sys.Debug("  distance: %v", node.distance)
			sys.Debug("   package: #%d", node.pkg)
			sys.Debug("       die: #%d", node.die)
		}

		for id, cpu := range sys.cpus {
			sys.Debug("CPU #%d:", id)
			sys.Debug("        pkg: %d", cpu.pkg)
			sys.Debug("        die: %d", cpu.die)
			sys.Debug("       node: %d", cpu.node)
			sys.Debug("       core: %d", cpu.core)
			sys.Debug("    threads: %s", cpu.threads)
			sys.Debug("  base freq: %d", cpu.baseFreq)
			sys.Debug("       freq: %d - %d", cpu.freq.min, cpu.freq.max)
			sys.Debug("        epp: %d", cpu.epp)
		}

		sys.Debug("offline CPUs: %s", sys.offline)
		sys.Debug("isolated CPUs: %s", sys.isolated)

		for id, cch := range sys.cache {
			sys.Debug("cache #%d:", id)
			sys.Debug("   type: %v", cch.kind)
			sys.Debug("   size: %d", cch.size)
			sys.Debug("  level: %d", cch.level)
			sys.Debug("   CPUs: %s", cch.cpus)
		}
	}

	return nil
}

// SetCpusOnline puts a set of CPUs online. Return the toggled set. Nil set implies all CPUs.
func (sys *system) SetCpusOnline(online bool, cpus idset.IDSet) (idset.IDSet, error) {
	var entries []string

	if cpus == nil {
		entries, _ = filepath.Glob(filepath.Join(sys.path, sysfsCPUPath, "cpu[0-9]*"))
	} else {
		entries = make([]string, cpus.Size())
		for idx, id := range cpus.Members() {
			entries[idx] = sys.path + "/" + sysfsCPUPath + "/cpu" + strconv.Itoa(int(id))
		}
	}

	desired := map[bool]int{false: 0, true: 1}[online]
	changed := idset.NewIDSet()

	for _, entry := range entries {
		var current int

		id := getEnumeratedID(entry)
		if id <= 0 {
			continue
		}

		if _, err := writeSysfsEntry(entry, "online", desired, &current); err != nil {
			return nil, sysfsError(entry, "failed to set online to %d: %v", desired, err)
		}

		if desired != current {
			changed.Add(id)
			if cpu, found := sys.cpus[id]; found {
				cpu.online = online

				if online {
					sys.offline.Del(id)
				} else {
					sys.offline.Add(id)
				}
			}
		}
	}

	return changed, nil
}

// SetCPUFrequencyLimits sets the CPU frequency scaling limits. Nil set implies all CPUs.
func (sys *system) SetCPUFrequencyLimits(min, max uint64, cpus idset.IDSet) error {
	if cpus == nil {
		cpus = idset.NewIDSet(sys.CPUIDs()...)
	}

	for _, id := range cpus.Members() {
		if cpu, ok := sys.cpus[id]; ok {
			if err := cpu.SetFrequencyLimits(min, max); err != nil {
				return err
			}
		}
	}

	return nil
}

// PackageIDs gets the ids of all packages present in the system.
func (sys *system) PackageIDs() []idset.ID {
	ids := make([]idset.ID, len(sys.packages))
	idx := 0
	for id := range sys.packages {
		ids[idx] = id
		idx++
	}

	sort.Slice(ids, func(i, j int) bool {
		return int(ids[i]) < int(ids[j])
	})

	return ids
}

// NodeIDs gets the ids of all NUMA nodes present in the system.
func (sys *system) NodeIDs() []idset.ID {
	ids := make([]idset.ID, len(sys.nodes))
	idx := 0
	for id := range sys.nodes {
		ids[idx] = id
		idx++
	}

	sort.Slice(ids, func(i, j int) bool {
		return int(ids[i]) < int(ids[j])
	})

	return ids
}

// CPUIDs gets the ids of all CPUs present in the system.
func (sys *system) CPUIDs() []idset.ID {
	ids := make([]idset.ID, len(sys.cpus))
	idx := 0
	for id := range sys.cpus {
		ids[idx] = id
		idx++
	}

	sort.Slice(ids, func(i, j int) bool {
		return int(ids[i]) < int(ids[j])
	})

	return ids
}

// PackageCount returns the number of discovered CPU packages (sockets).
func (sys *system) PackageCount() int {
	return len(sys.packages)
}

// SocketCount returns the number of discovered CPU packages (sockets).
func (sys *system) SocketCount() int {
	return len(sys.packages)
}

// CPUCount resturns the number of discovered CPUs/cores.
func (sys *system) CPUCount() int {
	return len(sys.cpus)
}

// NUMANodeCount returns the number of discovered NUMA nodes.
func (sys *system) NUMANodeCount() int {
	cnt := len(sys.nodes)
	if cnt < 1 {
		cnt = 1
	}
	return cnt
}

// ThreadCount returns the number of threads per core discovered.
func (sys *system) ThreadCount() int {
	return sys.threads
}

// CPUSet gets the ids of all CPUs present in the system as a CPUSet.
func (sys *system) CPUSet() cpuset.CPUSet {
	return CPUSetFromIDSet(idset.NewIDSet(sys.CPUIDs()...))
}

// Package gets the package with a given package id.
func (sys *system) Package(id idset.ID) CPUPackage {
	return sys.packages[id]
}

// Node gets the node with a given node id.
func (sys *system) Node(id idset.ID) Node {
	return sys.nodes[id]
}

// NodeDistance gets the distance between two NUMA nodes.
func (sys *system) NodeDistance(from, to idset.ID) int {
	return sys.nodes[from].DistanceFrom(to)
}

// CPU gets the CPU with a given CPU id.
func (sys *system) CPU(id idset.ID) CPU {
	return sys.cpus[id]
}

// Offlined gets the set of offlined CPUs.
func (sys *system) Offlined() cpuset.CPUSet {
	return CPUSetFromIDSet(sys.offline)
}

// Isolated gets the set of isolated CPUs."
func (sys *system) Isolated() cpuset.CPUSet {
	return CPUSetFromIDSet(sys.isolated)
}

// Discover Cpus present in the system.
func (sys *system) discoverCPUs() error {
	if sys.cpus != nil {
		return nil
	}

	sys.cpus = make(map[idset.ID]*cpu)

	_, err := readSysfsEntry(sys.path, filepath.Join(sysfsCPUPath, "isolated"), &sys.isolated, ",")
	if err != nil {
		sys.Error("failed to get set of isolated cpus: %v", err)
	}

	entries, _ := filepath.Glob(filepath.Join(sys.path, sysfsCPUPath, "cpu[0-9]*"))
	for _, entry := range entries {
		if err := sys.discoverCPU(entry); err != nil {
			return fmt.Errorf("failed to discover cpu for entry %s: %v", entry, err)
		}
	}

	return nil
}

// Discover details of the given CPU.
func (sys *system) discoverCPU(path string) error {
	cpu := &cpu{path: path, id: getEnumeratedID(path), online: true, sstClos: -1}

	cpu.isolated = sys.isolated.Has(cpu.id)

	if online, err := readSysfsEntry(path, "online", nil); err == nil {
		cpu.online = (online != "" && online[0] != '0')
	}

	if cpu.online {
		if _, err := readSysfsEntry(path, "topology/physical_package_id", &cpu.pkg); err != nil {
			return err
		}
		readSysfsEntry(path, "topology/die_id", &cpu.die)
		if _, err := readSysfsEntry(path, "topology/core_id", &cpu.core); err != nil {
			return err
		}
		if _, err := readSysfsEntry(path, "topology/thread_siblings_list", &cpu.threads, ","); err != nil {
			return err
		}
	} else {
		sys.offline.Add(cpu.id)
	}

	if _, err := readSysfsEntry(path, "cpufreq/base_frequency", &cpu.baseFreq); err != nil {
		cpu.baseFreq = 0
	}
	if _, err := readSysfsEntry(path, "cpufreq/cpuinfo_min_freq", &cpu.freq.min); err != nil {
		cpu.freq.min = 0
	}
	if _, err := readSysfsEntry(path, "cpufreq/cpuinfo_max_freq", &cpu.freq.max); err != nil {
		cpu.freq.max = 0
	}
	if _, err := readSysfsEntry(path, "cpufreq/energy_performance_preference", &cpu.epp); err != nil {
		cpu.epp = EPPUnknown
	}
	if node, _ := filepath.Glob(filepath.Join(path, "node[0-9]*")); len(node) == 1 {
		cpu.node = getEnumeratedID(node[0])
	} else {
		return fmt.Errorf("exactly one node per cpu allowed")
	}

	if sys.threads < 1 {
		sys.threads = 1
	}
	if cpu.threads.Size() > sys.threads {
		sys.threads = cpu.threads.Size()
	}

	sys.cpus[cpu.id] = cpu

	if (sys.flags & DiscoverCache) != 0 {
		entries, _ := filepath.Glob(filepath.Join(path, "cache/index[0-9]*"))
		for _, entry := range entries {
			if err := sys.discoverCache(entry); err != nil {
				return err
			}
		}
	}

	return nil
}

// ID returns the id of this CPU.
func (c *cpu) ID() idset.ID {
	return c.id
}

// PackageID returns package id of this CPU.
func (c *cpu) PackageID() idset.ID {
	return c.pkg
}

// DieID returns the die id of this CPU.
func (c *cpu) DieID() idset.ID {
	return c.die
}

// NodeID returns the node id of this CPU.
func (c *cpu) NodeID() idset.ID {
	return c.node
}

// CoreID returns the core id of this CPU (lowest CPU id of all thread siblings).
func (c *cpu) CoreID() idset.ID {
	return c.core
}

// ThreadCPUSet returns the CPUSet for all threads in this core.
func (c *cpu) ThreadCPUSet() cpuset.CPUSet {
	return CPUSetFromIDSet(c.threads)
}

// BaseFrequency returns the base frequency setting for this CPU.
func (c *cpu) BaseFrequency() uint64 {
	return c.baseFreq
}

// FrequencyRange returns the frequency range for this CPU.
func (c *cpu) FrequencyRange() CPUFreq {
	return c.freq
}

// EPP returns the energy performance profile of this CPU.
func (c *cpu) EPP() EPP {
	return c.epp
}

// Online returns if this CPU is online.
func (c *cpu) Online() bool {
	return c.online
}

// Isolated returns if this CPU is isolated.
func (c *cpu) Isolated() bool {
	return c.isolated
}

// SstClos returns the Speed Select Core Power CLOS number assigned to the CPU
// -1 implies that no SST prioritization is in effect
func (c *cpu) SstClos() int {
	return c.sstClos
}

// SetFrequencyLimits sets the frequency scaling limits for this CPU.
func (c *cpu) SetFrequencyLimits(min, max uint64) error {
	if c.freq.min == 0 {
		return nil
	}

	min /= 1000
	max /= 1000
	if min < c.freq.min && min != 0 {
		min = c.freq.min
	}
	if min > c.freq.max {
		min = c.freq.max
	}
	if max < c.freq.min && max != 0 {
		max = c.freq.min
	}
	if max > c.freq.max {
		max = c.freq.max
	}

	if _, err := writeSysfsEntry(c.path, "cpufreq/scaling_min_freq", min, nil); err != nil {
		return err
	}
	if _, err := writeSysfsEntry(c.path, "cpufreq/scaling_max_freq", max, nil); err != nil {
		return err
	}

	return nil
}

func readCPUsetFile(base, entry string) (cpuset.CPUSet, error) {
	path := filepath.Join(base, entry)

	blob, err := ioutil.ReadFile(path)
	if err != nil {
		return cpuset.New(), sysfsError(path, "failed to read sysfs entry: %v", err)
	}

	return cpuset.Parse(strings.Trim(string(blob), "\n"))
}

// Discover NUMA nodes present in the system.
func (sys *system) discoverNodes() error {
	if sys.nodes != nil {
		return nil
	}

	sysNodesPath := filepath.Join(sys.path, sysfsNumaNodePath)
	sys.nodes = make(map[idset.ID]*node)
	entries, _ := filepath.Glob(filepath.Join(sysNodesPath, "node[0-9]*"))
	for _, entry := range entries {
		if err := sys.discoverNode(entry); err != nil {
			return fmt.Errorf("failed to discover node for entry %s: %v", entry, err)
		}
	}

	normalMemNodeIDs, err := readSysfsEntry(sysNodesPath, "has_normal_memory", nil)
	if err != nil {
		return fmt.Errorf("failed to discover nodes with normal memory: %v", err)
	}
	normalMemNodes, err := cpuset.Parse(normalMemNodeIDs)
	if err != nil {
		return fmt.Errorf("failed to parse nodes with normal memory (%q): %v",
			normalMemNodes, err)
	}
	memoryNodeIDs, err := readSysfsEntry(sysNodesPath, "has_memory", nil)
	if err != nil {
		return fmt.Errorf("failed to discover nodes with memory: %v", err)
	}
	memoryNodes, err := cpuset.Parse(memoryNodeIDs)
	if err != nil {
		return fmt.Errorf("failed to parse nodes with memory (%q): %v",
			memoryNodeIDs, err)
	}

	cpuNodesSlice := []int{}
	for id, node := range sys.nodes {
		if node.cpus.Size() > 0 {
			cpuNodesSlice = append(cpuNodesSlice, int(id))
		}
		if normalMemNodes.Contains(int(id)) {
			node.normalMem = true
		}
	}
	cpuNodes := cpuset.New(cpuNodesSlice...)

	sys.Logger.Info("NUMA nodes with CPUs: %s", cpuNodes.String())
	sys.Logger.Info("NUMA nodes with (any) memory: %s", memoryNodes.String())
	sys.Logger.Info("NUMA nodes with normal memory: %s", normalMemNodes.String())

	dramNodes := memoryNodes.Intersection(cpuNodes)
	pmemOrHbmNodes := memoryNodes.Difference(dramNodes)

	dramNodeIds := IDSetFromCPUSet(dramNodes)
	pmemOrHbmNodeIds := IDSetFromCPUSet(pmemOrHbmNodes)

	infos := make(map[idset.ID]*MemInfo)
	dramAvg := uint64(0)
	if len(pmemOrHbmNodeIds) > 0 && len(dramNodeIds) > 0 {
		// There is special memory present in the system.

		// FIXME assumption: if a node only has memory (and no CPUs), it's PMEM or HBM. Otherwise it's DRAM.
		// Also, we figure out if the memory is HBM or PMEM based on the amount. If the amount of memory is
		// smaller than the average amount of DRAM per node, it's HBM, otherwise PMEM.
		dramTotal := uint64(0)
		for _, node := range sys.nodes {
			info, err := node.MemoryInfo()
			if err != nil {
				return fmt.Errorf("failed to get memory info for node %v: %s", node, err)
			}
			infos[node.id] = info
			if _, ok := dramNodeIds[node.id]; ok {
				dramTotal += info.MemTotal
			}
		}
		dramAvg = dramTotal / uint64(len(dramNodeIds))
		if dramAvg == 0 {
			// FIXME: should be no reason to bail out when memory types are properly determined.
			return fmt.Errorf("no dram in the system, cannot determine special memory types")
		}
	}

	for _, node := range sys.nodes {
		if _, ok := pmemOrHbmNodeIds[node.id]; ok {
			mem, ok := infos[node.id]
			if !ok {
				return fmt.Errorf("not able to determine system special memory types")
			}
			if mem.MemTotal < dramAvg {
				sys.Logger.Info("node %d has HBM memory", node.id)
				node.memoryType = MemoryTypeHBM
			} else {
				sys.Logger.Info("node %d has PMEM memory", node.id)
				node.memoryType = MemoryTypePMEM
			}
		} else if _, ok := dramNodeIds[node.id]; ok {
			sys.Logger.Info("node %d has DRAM memory", node.id)
			node.memoryType = MemoryTypeDRAM
		} else {
			return fmt.Errorf("Unknown memory type for node %v (pmem nodes: %s, dram nodes: %s)", node, pmemOrHbmNodes, dramNodes)
		}
	}

	return nil
}

// Discover details of the given NUMA node.
func (sys *system) discoverNode(path string) error {
	node := &node{path: path, id: getEnumeratedID(path)}

	if _, err := readSysfsEntry(path, "cpulist", &node.cpus, ","); err != nil {
		return err
	}
	if _, err := readSysfsEntry(path, "distance", &node.distance); err != nil {
		return err
	}

	sys.nodes[node.id] = node

	return nil
}

// ID returns id of this node.
func (n *node) ID() idset.ID {
	return n.id
}

// PackageID returns the package id for this node.
func (n *node) PackageID() idset.ID {
	return n.pkg
}

// DieID returns the die id for this node.
func (n *node) DieID() idset.ID {
	return n.die
}

// CPUSet returns the CPUSet for all cores/threads in this node.
func (n *node) CPUSet() cpuset.CPUSet {
	return CPUSetFromIDSet(n.cpus)
}

// Distance returns the distance vector for this node.
func (n *node) Distance() []int {
	return n.distance
}

// DistanceFrom returns the distance of this and a given node.
func (n *node) DistanceFrom(id idset.ID) int {
	if int(id) < len(n.distance) {
		return n.distance[int(id)]
	}

	return -1
}

// MemoryInfo memory info for the node (partial content from the meminfo sysfs entry).
func (n *node) MemoryInfo() (*MemInfo, error) {
	meminfo := filepath.Join(n.path, "meminfo")
	buf := &MemInfo{}
	err := ParseFileEntries(meminfo,
		map[string]interface{}{
			"MemTotal:": &buf.MemTotal,
			"MemFree:":  &buf.MemFree,
		},
		func(line string) (string, string, error) {
			fields := strings.Fields(strings.TrimSpace(line))
			if len(fields) < 4 {
				return "", "", sysfsError(meminfo, "failed to parse entry: '%s'", line)
			}
			key := fields[2]
			val := fields[3]
			if len(fields) == 5 {
				val += " " + fields[4]
			}
			return key, val, nil
		},
	)

	if err != nil {
		return nil, err
	}

	//
	// On some HW and kernel combinations we've seen more free than total
	// memory being reported. This causes exorbitant usage of memory being
	// reported which later can cause failures in policies which trust and
	// rely on this information.
	//
	// Give here a clear(er) error about that. This should also prevent us
	// immediately from starting up.
	//
	if buf.MemFree > buf.MemTotal {
		return nil, sysfsError(meminfo, "System reports more free than total memory. "+
			"This can be caused by a kernel bug. Please update your kernel.")
	}

	buf.MemUsed = buf.MemTotal - buf.MemFree

	return buf, nil
}

// GetMemoryType returns the memory type for this node.
func (n *node) GetMemoryType() MemoryType {
	return n.memoryType
}

// HasNormalMemory returns true if the node has memory that belongs to a normal zone.
func (n *node) HasNormalMemory() bool {
	return n.normalMem
}

// Discover physical packages (CPU sockets) present in the system.
func (sys *system) discoverPackages() error {
	if sys.packages != nil {
		return nil
	}

	sys.packages = make(map[idset.ID]*cpuPackage)

	for _, cpu := range sys.cpus {
		pkg, found := sys.packages[cpu.pkg]
		if !found {
			pkg = &cpuPackage{
				id:       cpu.pkg,
				cpus:     idset.NewIDSet(),
				nodes:    idset.NewIDSet(),
				dies:     idset.NewIDSet(),
				dieCPUs:  make(map[idset.ID]idset.IDSet),
				dieNodes: make(map[idset.ID]idset.IDSet),
			}
			sys.packages[cpu.pkg] = pkg
		}
		pkg.cpus.Add(cpu.id)
		pkg.nodes.Add(cpu.node)
		pkg.dies.Add(cpu.die)

		if dieCPUs, ok := pkg.dieCPUs[cpu.die]; !ok {
			pkg.dieCPUs[cpu.die] = idset.NewIDSet(cpu.id)
		} else {
			dieCPUs.Add(cpu.id)
		}
		if dieNodes, ok := pkg.dieNodes[cpu.die]; !ok {
			pkg.dieNodes[cpu.die] = idset.NewIDSet(cpu.node)
		} else {
			dieNodes.Add(cpu.node)
		}
	}

	return nil
}

func (sys *system) discoverSst() error {
	if !sst.SstSupported() {
		sys.Info("Speed Select Technology (SST) support not detected")
		return nil
	}

	for _, pkg := range sys.packages {
		sstInfo, err := sst.GetPackageInfo(pkg.id)
		if err != nil {
			return fmt.Errorf("failed to get SST info for package %d: %v", pkg.id, err)
		}
		sys.DebugBlock("", "Speed Select Technology info detected for package %d:\n%s", pkg.id, utils.DumpJSON(sstInfo))

		if sstInfo[pkg.id].CPEnabled {
			ids := pkg.cpus.SortedMembers()

			for _, id := range ids {
				clos, err := sst.GetCPUClosID(id)
				if err != nil {
					return fmt.Errorf("failed to get SST-CP clos id for cpu %d: %v", id, err)
				}

				sys.cpus[id].sstClos = clos
			}
		}
		pkg.sstInfo = sstInfo[pkg.id]
	}

	return nil
}

// ID returns the id of this package.
func (p *cpuPackage) ID() idset.ID {
	return p.id
}

// CPUSet returns the CPUSet for all cores/threads in this package.
func (p *cpuPackage) CPUSet() cpuset.CPUSet {
	return CPUSetFromIDSet(p.cpus)
}

// DieIDs returns the die ids for this package.
func (p *cpuPackage) DieIDs() []idset.ID {
	return p.dies.SortedMembers()
}

// NodeIDs returns the NUMA node ids for this package.
func (p *cpuPackage) NodeIDs() []idset.ID {
	return p.nodes.SortedMembers()
}

// DieNodeIDs returns the set of NUMA nodes in the given die of this package.
func (p *cpuPackage) DieNodeIDs(id idset.ID) []idset.ID {
	if dieNodes, ok := p.dieNodes[id]; ok {
		return dieNodes.SortedMembers()
	}
	return []idset.ID{}
}

// DieCPUSet returns the set of CPUs in the given die of this package.
func (p *cpuPackage) DieCPUSet(id idset.ID) cpuset.CPUSet {
	if dieCPUs, ok := p.dieCPUs[id]; ok {
		return CPUSetFromIDSet(dieCPUs)
	}
	return cpuset.New()
}

func (p *cpuPackage) SstInfo() *sst.SstPackageInfo {
	return p.sstInfo
}

// Discover cache associated with the given CPU.
//
// Notes:
//
//	I'm not sure how to interpret the cache information under sysfs.
//	This code is now effectively disabled by forcing the associated
//	discovery bit off in the discovery flags.
func (sys *system) discoverCache(path string) error {
	var id idset.ID

	if _, err := readSysfsEntry(path, "id", &id); err != nil {
		return sysfsError(path, "can't read cache id: %v", err)
	}

	if sys.cache == nil {
		sys.cache = make(map[idset.ID]*Cache)
	}

	if _, found := sys.cache[id]; found {
		return nil
	}

	c := &Cache{id: id}

	if _, err := readSysfsEntry(path, "level", &c.level); err != nil {
		return sysfsError(path, "can't read cache level: %v", err)
	}
	if _, err := readSysfsEntry(path, "shared_cpu_list", &c.cpus, ","); err != nil {
		return sysfsError(path, "can't read shared CPUs: %v", err)
	}
	kind := ""
	if _, err := readSysfsEntry(path, "type", &kind); err != nil {
		return sysfsError(path, "can't read cache type: %v", err)
	}
	switch kind {
	case "Data":
		c.kind = DataCache
	case "Instruction":
		c.kind = InstructionCache
	case "Unified":
		c.kind = UnifiedCache
	default:
		return sysfsError(path, "unknown cache type: %s", kind)
	}

	size := ""
	if _, err := readSysfsEntry(path, "size", &size); err != nil {
		return sysfsError(path, "can't read cache size: %v", err)
	}

	base := size[0 : len(size)-1]
	suff := size[len(size)-1]
	unit := map[byte]uint64{'K': 1 << 10, 'M': 1 << 20, 'G': 1 << 30}

	val, err := strconv.ParseUint(base, 10, 0)
	if err != nil {
		return sysfsError(path, "can't parse cache size '%s': %v", size, err)
	}

	if u, ok := unit[suff]; ok {
		c.size = val * u
	} else {
		c.size = val*1000 + u - '0'
	}

	sys.cache[c.id] = c

	return nil
}

// eppStrings initialized this way to better catch changes in the enum
var eppStrings = func() [EPPUnknown]string {
	var e [EPPUnknown]string
	e[EPPPerformance] = "performance"
	e[EPPBalancePerformance] = "balance_performance"
	e[EPPBalancePower] = "balance_power"
	e[EPPPower] = "power"
	return e
}()

var eppValues = func() map[string]EPP {
	m := make(map[string]EPP, len(eppStrings))
	for i, v := range eppStrings {
		m[v] = EPP(i)
	}
	return m
}()

// String returns EPP value as string
func (e EPP) String() string {
	if int(e) < len(eppStrings) {
		return eppStrings[e]
	}
	return ""
}

// EPPFromString converts string to EPP value
func EPPFromString(s string) EPP {
	if v, ok := eppValues[s]; ok {
		return v
	}
	return EPPUnknown
}
