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

	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"

	logger "github.com/intel/cri-resource-manager/pkg/log"
)

const (
	// SysfsRootPath is the mount path of sysfs.
	SysfsRootPath = "/sys"
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
	// DiscoverNone is the zero value for discovery flags.
	DiscoverNone DiscoveryFlag = 0
	// DiscoverAll requests full supported discovery.
	DiscoverAll DiscoveryFlag = 0xffffffff
	// DiscoverDefault is the default set of discovery flags.
	DiscoverDefault DiscoveryFlag = (DiscoverCPUTopology | DiscoverMemTopology)
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
	SetCpusOnline(online bool, cpus IDSet) (IDSet, error)
	SetCPUFrequencyLimits(min, max uint64, cpus IDSet) error
	PackageIDs() []ID
	NodeIDs() []ID
	CPUIDs() []ID
	PackageCount() int
	SocketCount() int
	CPUCount() int
	NUMANodeCount() int
	ThreadCount() int
	CPUSet() cpuset.CPUSet
	Package(id ID) CPUPackage
	Node(id ID) Node
	CPU(id ID) CPU
	Offlined() cpuset.CPUSet
	Isolated() cpuset.CPUSet
}

// System devices
type system struct {
	logger.Logger                    // our logger instance
	flags         DiscoveryFlag      // system discovery flags
	path          string             // sysfs mount point
	packages      map[ID]*cpuPackage // physical packages
	nodes         map[ID]*node       // NUMA nodes
	cpus          map[ID]*cpu        // CPUs
	cache         map[ID]*Cache      // Cache
	offline       IDSet              // offlined CPUs
	isolated      IDSet              // isolated CPUs
	threads       int                // hyperthreads per core
}

// CPUPackage is a physical package (a collection of CPUs).
type CPUPackage interface {
	ID() ID
	CPUSet() cpuset.CPUSet
	NodeIDs() []ID
}

type cpuPackage struct {
	id    ID    // package id
	cpus  IDSet // CPUs in this package
	nodes IDSet // nodes in this package
}

// Node represents a NUMA node.
type Node interface {
	ID() ID
	PackageID() ID
	CPUSet() cpuset.CPUSet
	Distance() []int
	DistanceFrom(id ID) int
	MemoryInfo() (*MemInfo, error)
	GetMemoryType() MemoryType
}

type node struct {
	path       string     // sysfs path
	id         ID         // node id
	pkg        ID         // package id
	cpus       IDSet      // cpus in this node
	memoryType MemoryType // node memory type
	distance   []int      // distance/cost to other NUMA nodes
}

// CPU is a CPU core.
type CPU interface {
	ID() ID
	PackageID() ID
	NodeID() ID
	CoreID() ID
	ThreadCPUSet() cpuset.CPUSet
	BaseFrequency() uint64
	FrequencyRange() CPUFreq
	Online() bool
	Isolated() bool
	SetFrequencyLimits(min, max uint64) error
}

type cpu struct {
	path     string  // sysfs path
	id       ID      // CPU id
	pkg      ID      // package id
	node     ID      // node id
	core     ID      // core id
	threads  IDSet   // sibling/hyper-threads
	baseFreq uint64  // CPU base frequency
	freq     CPUFreq // CPU frequencies
	online   bool    // whether this CPU is online
	isolated bool    // whether this CPU is isolated
}

// CPUFreq is a CPU frequency scaling range
type CPUFreq struct {
	min uint64   // minimum frequency (kHz)
	max uint64   // maximum frequency (kHz)
	all []uint64 // discrete set of frequencies if applicable/known
}

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
	id    ID        // cache id
	kind  CacheType // cache type
	size  uint64    // cache size
	level uint8     // cache level
	cpus  IDSet     // CPUs sharing this cache
}

// DiscoverSystem performs discovery of the running systems details.
func DiscoverSystem(args ...DiscoveryFlag) (System, error) {
	return DiscoverSystemAt(SysfsRootPath, args...)
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
		offline: NewIDSet(),
	}

	if err := sys.Discover(flags); err != nil {
		return nil, err
	}

	return sys, nil
}

// Discover performs system/hardware discovery.
func (sys *system) Discover(flags DiscoveryFlag) error {
	sys.flags |= (flags &^ DiscoverCache)

	if (sys.flags & (DiscoverCPUTopology | DiscoverCache)) != 0 {
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
				}
			}
		}
	}

	if sys.DebugEnabled() {
		for id, pkg := range sys.packages {
			sys.Info("package #%d:", id)
			sys.Debug("   cpus: %s", pkg.cpus)
			sys.Debug("  nodes: %s", pkg.nodes)
		}

		for id, node := range sys.nodes {
			sys.Debug("node #%d:", id)
			sys.Debug("      cpus: %s", node.cpus)
			sys.Debug("  distance: %v", node.distance)
		}

		for id, cpu := range sys.cpus {
			sys.Debug("CPU #%d:", id)
			sys.Debug("        pkg: %d", cpu.pkg)
			sys.Debug("       node: %d", cpu.node)
			sys.Debug("       core: %d", cpu.core)
			sys.Debug("    threads: %s", cpu.threads)
			sys.Debug("  base freq: %d", cpu.baseFreq)
			sys.Debug("       freq: %d - %d", cpu.freq.min, cpu.freq.max)
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
func (sys *system) SetCpusOnline(online bool, cpus IDSet) (IDSet, error) {
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
	changed := NewIDSet()

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
func (sys *system) SetCPUFrequencyLimits(min, max uint64, cpus IDSet) error {
	if cpus == nil {
		cpus = NewIDSet(sys.CPUIDs()...)
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
func (sys *system) PackageIDs() []ID {
	ids := make([]ID, len(sys.packages))
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
func (sys *system) NodeIDs() []ID {
	ids := make([]ID, len(sys.nodes))
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
func (sys *system) CPUIDs() []ID {
	ids := make([]ID, len(sys.cpus))
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
	return NewIDSet(sys.CPUIDs()...).CPUSet()
}

// Package gets the package with a given package id.
func (sys *system) Package(id ID) CPUPackage {
	return sys.packages[id]
}

// Node gets the node with a given node id.
func (sys *system) Node(id ID) Node {
	return sys.nodes[id]
}

// CPU gets the CPU with a given CPU id.
func (sys *system) CPU(id ID) CPU {
	return sys.cpus[id]
}

// Offlined gets the set of offlined CPUs.
func (sys *system) Offlined() cpuset.CPUSet {
	return sys.offline.CPUSet()
}

// Isolated gets the set of isolated CPUs."
func (sys *system) Isolated() cpuset.CPUSet {
	return sys.isolated.CPUSet()
}

// Discover Cpus present in the system.
func (sys *system) discoverCPUs() error {
	if sys.cpus != nil {
		return nil
	}

	sys.cpus = make(map[ID]*cpu)

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
	cpu := &cpu{path: path, id: getEnumeratedID(path), online: true}

	cpu.isolated = sys.isolated.Has(cpu.id)

	if online, err := readSysfsEntry(path, "online", nil); err == nil {
		cpu.online = (online != "" && online[0] != '0')
	}

	if cpu.online {
		if _, err := readSysfsEntry(path, "topology/physical_package_id", &cpu.pkg); err != nil {
			return err
		}
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
func (c *cpu) ID() ID {
	return c.id
}

// PackageID returns package id of this CPU.
func (c *cpu) PackageID() ID {
	return c.pkg
}

// NodeID returns the node id of this CPU.
func (c *cpu) NodeID() ID {
	return c.node
}

// CoreID returns the core id of this CPU (lowest CPU id of all thread siblings).
func (c *cpu) CoreID() ID {
	return c.core
}

// ThreadCPUSet returns the CPUSet for all threads in this core.
func (c *cpu) ThreadCPUSet() cpuset.CPUSet {
	return c.threads.CPUSet()
}

// BaseFrequency returns the base frequency setting for this CPU.
func (c *cpu) BaseFrequency() uint64 {
	return c.baseFreq
}

// FrequencyRange returns the frequency range for this CPU.
func (c *cpu) FrequencyRange() CPUFreq {
	return c.freq
}

// Online returns if this CPU is online.
func (c *cpu) Online() bool {
	return c.online
}

// Isolated returns if this CPU is isolated.
func (c *cpu) Isolated() bool {
	return c.isolated
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
		return cpuset.NewCPUSet(), sysfsError(path, "failed to read sysfs entry: %v", err)
	}

	return cpuset.Parse(strings.Trim(string(blob), "\n"))
}

// Discover NUMA nodes present in the system.
func (sys *system) discoverNodes() error {
	if sys.nodes != nil {
		return nil
	}

	sys.nodes = make(map[ID]*node)
	entries, _ := filepath.Glob(filepath.Join(sys.path, sysfsNumaNodePath, "node[0-9]*"))
	for _, entry := range entries {
		if err := sys.discoverNode(entry); err != nil {
			return fmt.Errorf("failed to discover node for entry %s: %v", entry, err)
		}
	}

	cpuNodesBuilder := cpuset.NewBuilder()
	memoryNodesBuilder := cpuset.NewBuilder()
	for _, node := range sys.nodes {
		if node.cpus.Size() > 0 {
			cpuNodesBuilder.Add(int(node.id))
		}
		mem, _ := filepath.Glob(filepath.Join(node.path, "memory[0-9]*"))
		if len(mem) > 0 {
			memoryNodesBuilder.Add(int(node.id))
		}
	}
	cpuNodes := cpuNodesBuilder.Result()
	memoryNodes := memoryNodesBuilder.Result()

	sys.Logger.Info("NUMA nodes with CPUs: %s", cpuNodes.String())
	sys.Logger.Info("NUMA nodes with memory: %s", memoryNodes.String())

	dramNodes := memoryNodes.Intersection(cpuNodes)
	pmemOrHbmNodes := memoryNodes.Difference(dramNodes)

	dramNodeIds := FromCPUSet(dramNodes)
	pmemOrHbmNodeIds := FromCPUSet(pmemOrHbmNodes)

	infos := make(map[ID]*MemInfo)
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
func (n *node) ID() ID {
	return n.id
}

// PackageID returns the id of this node.
func (n *node) PackageID() ID {
	return n.pkg
}

// CPUSet returns the CPUSet for all cores/threads in this node.
func (n *node) CPUSet() cpuset.CPUSet {
	return n.cpus.CPUSet()
}

// Distance returns the distance vector for this node.
func (n *node) Distance() []int {
	return n.distance
}

// DistanceFrom returns the distance of this and a given node.
func (n *node) DistanceFrom(id ID) int {
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
			"MemUsed:":  &buf.MemUsed,
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
	return buf, nil
}

// GetMemoryType returns the memory type for this node.
func (n *node) GetMemoryType() MemoryType {
	return n.memoryType
}

// Discover physical packages (CPU sockets) present in the system.
func (sys *system) discoverPackages() error {
	if sys.packages != nil {
		return nil
	}

	sys.packages = make(map[ID]*cpuPackage)

	for _, cpu := range sys.cpus {
		pkg, found := sys.packages[cpu.pkg]
		if !found {
			pkg = &cpuPackage{
				id:    cpu.pkg,
				cpus:  NewIDSet(),
				nodes: NewIDSet(),
			}
			sys.packages[cpu.pkg] = pkg
		}
		pkg.cpus.Add(cpu.id)
		pkg.nodes.Add(cpu.node)
	}

	return nil
}

// ID returns the id of this package.
func (p *cpuPackage) ID() ID {
	return p.id
}

// CPUSet returns the CPUSet for all cores/threads in this package.
func (p *cpuPackage) CPUSet() cpuset.CPUSet {
	return p.cpus.CPUSet()
}

// NodeIDs returns the NUMA node ids for this package.
func (p *cpuPackage) NodeIDs() []ID {
	return p.nodes.SortedMembers()
}

// Discover cache associated with the given CPU.
// Notes:
//     I'm not sure how to interpret the cache information under sysfs. This code is now effectively
//     disabled by forcing the associated discovery bit off in the discovery flags.
func (sys *system) discoverCache(path string) error {
	var id ID

	if _, err := readSysfsEntry(path, "id", &id); err != nil {
		return sysfsError(path, "can't read cache id: %v", err)
	}

	if sys.cache == nil {
		sys.cache = make(map[ID]*Cache)
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
