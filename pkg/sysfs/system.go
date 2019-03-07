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
	"path/filepath"
	"strconv"

	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"

	logger "github.com/intel/cri-resource-manager/pkg/log"
)

const (
	// SysfsRootPath is the mount path of sysfs.
	SysfsRootPath = "/sys"
	// sysfs devices/cpu subdirectory path
	sysfsCpuPath = "devices/system/cpu"
	// sysfs device/node subdirectory path
	sysfsNumaNodePath = "devices/system/node"
)

// DiscoveryFlag controls what hardware details to discover.
type DiscoveryFlag uint

const (
	// DiscoverCpuTopology requests discovering CPU topology details.
	DiscoverCpuTopology DiscoveryFlag = 1 << iota
	// DiscoverMemTopology requests discovering memory topology details.
	DiscoverMemTopology
	// DiscoverCache requests discovering CPU cache details.
	DiscoverCache
	// DiscoverNone is the zero value for discovery flags.
	DiscoverNone DiscoveryFlag = 0
	// DiscoverAll requests full supported discovery.
	DiscoverAll DiscoveryFlag = 0xffffffff
	// DiscoverDefault is the default set of discovery flags.
	DiscoverDefault DiscoveryFlag = DiscoverCpuTopology
)

// System devices
type System struct {
	logger.Logger                 // our logger instance
	flags         DiscoveryFlag   // system discovery flags
	path          string          // sysfs mount point
	packages      map[Id]*Package // physical packages
	nodes         map[Id]*Node    // NUMA nodes
	cpus          map[Id]*Cpu     // CPUs
	cache         map[Id]*Cache   // Cache
	offline       IdSet           // offlined CPUs
	isolated      IdSet           // isolated CPUs
}

// Package is a physical package (a collection of CPUs).
type Package struct {
	id    Id    // package id
	cpus  IdSet // CPUs in this package
	nodes IdSet // nodes in this package
}

// Node is a NUMA node.
type Node struct {
	path     string // sysfs path
	id       Id     // node id
	cpus     IdSet  // cpus in this node
	distance []int  // distance/cost to other NUMA nodes
	memory   IdSet  // memory in this node
}

// Cpu is a CPU core.
type Cpu struct {
	path     string  // sysfs path
	id       Id      // CPU id
	pkg      Id      // package id
	node     Id      // node id
	threads  IdSet   // sibling/hyper-threads
	freq     CpuFreq // CPU frequencies
	online   bool    // whether this CPU is online
	isolated bool    // whether this CPU is isolated
}

// CpuFreq is a CPU frequency scaling range
type CpuFreq struct {
	min uint64   // minimum frequency (kHz)
	max uint64   // maximum frequency (kHz)
	all []uint64 // discrete set of frequencies if applicable/known
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
	id    Id        // cache id
	kind  CacheType // cache type
	size  uint64    // cache size
	level uint8     // cache level
	cpus  IdSet     // CPUs sharing this cache
}

// DiscoverSystem performs discovery of the running systems details.
func DiscoverSystem(args ...DiscoveryFlag) (*System, error) {
	var flags DiscoveryFlag

	if len(args) < 1 {
		flags = DiscoverDefault
	} else {
		flags = DiscoverNone
		for _, flag := range args {
			flags |= flag
		}
	}

	sys := &System{
		Logger:  logger.NewLogger("sysfs"),
		path:    SysfsRootPath,
		offline: NewIdSet(),
	}

	if err := sys.Discover(flags); err != nil {
		return nil, err
	}

	return sys, nil
}

// Discover performs system/hardware discovery.
func (sys *System) Discover(flags DiscoveryFlag) error {
	sys.flags |= (flags &^ DiscoverCache)

	if (sys.flags & (DiscoverCpuTopology | DiscoverCache)) != 0 {
		if err := sys.discoverCpus(); err != nil {
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

	if sys.DebugEnabled() {
		for id, pkg := range sys.packages {
			sys.Info("package #%d:", id)
			sys.Debug("   cpus: %s", pkg.cpus)
			sys.Debug("  nodes: %s", pkg.nodes)
		}

		for id, node := range sys.nodes {
			sys.Debug("node #%d:", id)
			sys.Debug("      cpus: %s", node.cpus)
			sys.Debug("    memory: %s", node.memory)
			sys.Debug("  distance: %v", node.distance)
		}

		for id, cpu := range sys.cpus {
			sys.Debug("CPU #%d:", id)
			sys.Debug("      pkg: %d", cpu.pkg)
			sys.Debug("     node: %d", cpu.node)
			sys.Debug("  threads: %s", cpu.threads)
			sys.Debug("     freq: %d - %d", cpu.freq.min, cpu.freq.max)
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
func (sys *System) SetCpusOnline(online bool, cpus IdSet) (IdSet, error) {
	var entries []string

	if cpus == nil {
		entries, _ = filepath.Glob(filepath.Join(sys.path, sysfsCpuPath, "cpu[0-9]*"))
	} else {
		entries = make([]string, cpus.Size())
		for idx, id := range cpus.Members() {
			entries[idx] = sys.path + "/" + sysfsCpuPath + "/cpu" + strconv.Itoa(int(id))
		}
	}

	desired := map[bool]int{false: 0, true: 1}[online]
	changed := NewIdSet()

	for _, entry := range entries {
		var current int

		id := getEnumeratedId(entry)
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

// SetCpuFrequencyLimits sets the CPU frequency scaling limits. Nil set implies all CPUs.
func (sys *System) SetCpuFrequencyLimits(min, max uint64, cpus IdSet) error {
	if cpus == nil {
		cpus = NewIdSet(sys.CpuIds()...)
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

// PackageIds gets the ids of all packages present in the system.
func (sys *System) PackageIds() []Id {
	ids := make([]Id, len(sys.packages))
	idx := 0
	for id := range sys.packages {
		ids[idx] = id
		idx++
	}
	return ids
}

// NodeIds gets the ids of all NUMA nodes present in the system.
func (sys *System) NodeIds() []Id {
	ids := make([]Id, len(sys.nodes))
	idx := 0
	for id := range sys.nodes {
		ids[idx] = id
		idx++
	}
	return ids
}

// CpuIds gets the ids of all CPUs present in the system.
func (sys *System) CpuIds() []Id {
	ids := make([]Id, len(sys.cpus))
	idx := 0
	for id := range sys.cpus {
		ids[idx] = id
		idx++
	}
	return ids
}

// CPUSet gets the ids of all CPUs present in the system as a CPUSet.
func (sys *System) CPUSet() cpuset.CPUSet {
	return NewIdSet(sys.CpuIds()...).CPUSet()
}

// Package gets the package with a given package id.
func (sys *System) Package(id Id) *Package {
	return sys.packages[id]
}

// Node gets the node with a given node id.
func (sys *System) Node(id Id) *Node {
	return sys.nodes[id]
}

// Cpu gets the CPU with a given CPU id.
func (sys *System) Cpu(id Id) *Cpu {
	return sys.cpus[id]
}

// Offlined gets the set of offlined CPUs.
func (sys *System) Offlined() cpuset.CPUSet {
	return sys.offline.CPUSet()
}

// Isolated gets the set of isolated CPUs."
func (sys *System) Isolated() cpuset.CPUSet {
	return sys.isolated.CPUSet()
}

// Discover Cpus present in the system.
func (sys *System) discoverCpus() error {
	if sys.cpus != nil {
		return nil
	}

	sys.cpus = make(map[Id]*Cpu)

	offline, err := sys.SetCpusOnline(true, nil)
	if err != nil {
		return fmt.Errorf("failed to set CPUs online: %v", err)
	}
	defer sys.SetCpusOnline(false, offline)

	_, err = readSysfsEntry(sys.path, filepath.Join(sysfsCpuPath, "isolated"), &sys.isolated, ",")
	if err != nil {
		sys.Error("failed to get set of isolated cpus: %v", err)
	}

	entries, _ := filepath.Glob(filepath.Join(sys.path, sysfsCpuPath, "cpu[0-9]*"))
	for _, entry := range entries {
		if err := sys.discoverCpu(entry); err != nil {
			return fmt.Errorf("failed to discover cpu for entry %s: %v", entry, err)
		}
	}

	return nil
}

// Discover details of the given CPU.
func (sys *System) discoverCpu(path string) error {
	cpu := &Cpu{path: path, id: getEnumeratedId(path), online: true}

	cpu.isolated = sys.isolated.Has(cpu.id)

	if _, err := readSysfsEntry(path, "topology/physical_package_id", &cpu.pkg); err != nil {
		return err
	}
	if _, err := readSysfsEntry(path, "topology/thread_siblings_list", &cpu.threads, ","); err != nil {
		return err
	}
	if _, err := readSysfsEntry(path, "cpufreq/cpuinfo_min_freq", &cpu.freq.min); err != nil {
		cpu.freq.min = 0
	}
	if _, err := readSysfsEntry(path, "cpufreq/cpuinfo_max_freq", &cpu.freq.max); err != nil {
		cpu.freq.max = 0
	}
	if node, _ := filepath.Glob(filepath.Join(path, "node[0-9]*")); len(node) == 1 {
		cpu.node = getEnumeratedId(node[0])
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

// Id returns the id of this CPU.
func (c *Cpu) Id() Id {
	return c.id
}

// PackageId returns package id of this CPU.
func (c *Cpu) PackageId() Id {
	return c.pkg
}

// NodeId returns the node id of this CPU.
func (c *Cpu) NodeId() Id {
	return c.node
}

// CoreId returns the core id of this CPU (lowest CPU id of all thread siblings).
func (c *Cpu) CoreId() Id {
	return c.threads.SortedMembers()[0]
}

// ThreadCPUSet returns the CPUSet for all threads in this core.
func (c *Cpu) ThreadCPUSet() cpuset.CPUSet {
	return c.threads.CPUSet()
}

// FrequencyRange returns the frequency range for this CPU.
func (c *Cpu) FrequencyRange() CpuFreq {
	return c.freq
}

// Online returns if this CPU is online.
func (c *Cpu) Online() bool {
	return c.online
}

// Isolated returns if this CPU is isolated.
func (c *Cpu) Isolated() bool {
	return c.isolated
}

// SetFrequencyLimits sets the frequency scaling limits for this CPU.
func (c *Cpu) SetFrequencyLimits(min, max uint64) error {
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

// Discover NUMA nodes present in the system.
func (sys *System) discoverNodes() error {
	if sys.nodes != nil {
		return nil
	}

	sys.nodes = make(map[Id]*Node)

	entries, _ := filepath.Glob(filepath.Join(sys.path, sysfsNumaNodePath, "node[0-9]*"))
	for _, entry := range entries {
		if err := sys.discoverNode(entry); err != nil {
			return fmt.Errorf("failed to discover node for entry %s: %v", entry, err)
		}
	}

	return nil
}

// Discover details of the given NUMA node.
func (sys *System) discoverNode(path string) error {
	node := &Node{path: path, id: getEnumeratedId(path)}

	if _, err := readSysfsEntry(path, "cpulist", &node.cpus, ","); err != nil {
		return err
	}
	if _, err := readSysfsEntry(path, "distance", &node.distance); err != nil {
		return err
	}

	if (sys.flags & DiscoverMemTopology) != 0 {
		node.memory = NewIdSet()

		entries, _ := filepath.Glob(filepath.Join(path, "memory[0-9]*"))
		for _, entry := range entries {
			node.memory.Add(getEnumeratedId(entry))
		}
	}

	sys.nodes[node.id] = node

	return nil
}

// Id returns id of this node.
func (n *Node) Id() Id {
	return n.id
}

// CPUSet returns the CPUSet for all cores/threads in this node.
func (n *Node) CPUSet() cpuset.CPUSet {
	return n.cpus.CPUSet()
}

// Distance returns the distance vector for this node.
func (n *Node) Distance() []int {
	return n.distance
}

// DistanceFrom returns the distance of this and a given node.
func (n *Node) DistanceFrom(id Id) int {
	if int(id) < len(n.distance) {
		return n.distance[int(id)]
	}

	return -1
}

// Discover physical packages (CPU sockets) present in the system.
func (sys *System) discoverPackages() error {
	if sys.packages != nil {
		return nil
	}

	sys.packages = make(map[Id]*Package)

	for _, cpu := range sys.cpus {
		pkg, found := sys.packages[cpu.pkg]
		if !found {
			pkg = &Package{
				id:    cpu.pkg,
				cpus:  NewIdSet(),
				nodes: NewIdSet(),
			}
			sys.packages[cpu.pkg] = pkg
		}
		pkg.cpus.Add(cpu.id)
		pkg.nodes.Add(cpu.node)
	}

	return nil
}

// Id returns the id of this package.
func (p *Package) Id() Id {
	return p.id
}

// CPUSet returns the CPUSet for all cores/threads in this package.
func (p *Package) CPUSet() cpuset.CPUSet {
	return p.cpus.CPUSet()
}

// Discover cache associated with the given CPU.
// Notes:
//     I'm not sure how to interpret the cache information under sysfs. This code is now effectively
//     disabled by forcing the associated discovery bit off in the discovery flags.
func (sys *System) discoverCache(path string) error {
	var id Id

	if _, err := readSysfsEntry(path, "id", &id); err != nil {
		return sysfsError(path, "can't read cache id: %v", err)
	}

	if sys.cache == nil {
		sys.cache = make(map[Id]*Cache)
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
