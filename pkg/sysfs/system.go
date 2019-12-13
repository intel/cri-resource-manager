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

// System devices
type System struct {
	logger.Logger                 // our logger instance
	flags         DiscoveryFlag   // system discovery flags
	path          string          // sysfs mount point
	packages      map[ID]*Package // physical packages
	nodes         map[ID]*Node    // NUMA nodes
	cpus          map[ID]*CPU     // CPUs
	cache         map[ID]*Cache   // Cache
	offline       IDSet           // offlined CPUs
	isolated      IDSet           // isolated CPUs
	threads       int             // hyperthreads per core
}

// Package is a physical package (a collection of CPUs).
type Package struct {
	id    ID    // package id
	Cpus  IDSet // CPUs in this package
	nodes IDSet // nodes in this package
}

// Node is a NUMA node.
type Node struct {
	path     string // sysfs path
	id       ID     // node id
	pkg      ID     // package id
	Cpus     IDSet  // cpus in this node
	distance []int  // distance/cost to other NUMA nodes
}

// CPU is a CPU core.
type CPU struct {
	path     string  // sysfs path
	id       ID      // CPU id
	pkg      ID      // package id
	node     ID      // node id
	threads  IDSet   // sibling/hyper-threads
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
		offline: NewIDSet(),
	}

	if err := sys.Discover(flags); err != nil {
		return nil, err
	}

	return sys, nil
}

// Discover performs system/hardware discovery.
func (sys *System) Discover(flags DiscoveryFlag) error {
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
			sys.Debug("   cpus: %s", pkg.Cpus)
			sys.Debug("  nodes: %s", pkg.nodes)
		}

		for id, node := range sys.nodes {
			sys.Debug("node #%d:", id)
			sys.Debug("      cpus: %s", node.Cpus)
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
func (sys *System) SetCpusOnline(online bool, cpus IDSet) (IDSet, error) {
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
func (sys *System) SetCPUFrequencyLimits(min, max uint64, cpus IDSet) error {
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
func (sys *System) PackageIDs() []ID {
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
func (sys *System) NodeIDs() []ID {
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

// PackageNodeIDs returns NUMA node ids for a given package.
func (sys *System) PackageNodeIDs(id ID) []ID {
	return sys.Package(id).NodeIDs()
}

// CPUIDs gets the ids of all CPUs present in the system.
func (sys *System) CPUIDs() []ID {
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
func (sys *System) PackageCount() int {
	return len(sys.packages)
}

// SocketCount returns the number of discovered CPU packages (sockets).
func (sys *System) SocketCount() int {
	return len(sys.packages)
}

// CPUCount resturns the number of discovered CPUs/cores.
func (sys *System) CPUCount() int {
	return len(sys.cpus)
}

// NUMANodeCount returns the number of discovered NUMA nodes.
func (sys *System) NUMANodeCount() int {
	cnt := len(sys.nodes)
	if cnt < 1 {
		cnt = 1
	}
	return cnt
}

// ThreadCount returns the number of threads per core discovered.
func (sys *System) ThreadCount() int {
	return sys.threads
}

// CPUSet gets the ids of all CPUs present in the system as a CPUSet.
func (sys *System) CPUSet() cpuset.CPUSet {
	return NewIDSet(sys.CPUIDs()...).CPUSet()
}

// Package gets the package with a given package id.
func (sys *System) Package(id ID) *Package {
	return sys.packages[id]
}

// Node gets the node with a given node id.
func (sys *System) Node(id ID) *Node {
	return sys.nodes[id]
}

// CPU gets the CPU with a given CPU id.
func (sys *System) CPU(id ID) *CPU {
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
func (sys *System) discoverCPUs() error {
	if sys.cpus != nil {
		return nil
	}

	sys.cpus = make(map[ID]*CPU)

	offline, err := sys.SetCpusOnline(true, nil)
	if err != nil {
		return fmt.Errorf("failed to set CPUs online: %v", err)
	}
	defer sys.SetCpusOnline(false, offline)

	_, err = readSysfsEntry(sys.path, filepath.Join(sysfsCPUPath, "isolated"), &sys.isolated, ",")
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
func (sys *System) discoverCPU(path string) error {
	cpu := &CPU{path: path, id: getEnumeratedID(path), online: true}

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
		cpu.node = getEnumeratedID(node[0])
	}

	if sys.threads < 1 {
		sys.threads = cpu.threads.Size()
	}
	if sys.threads < 1 {
		sys.threads = 1
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
func (c *CPU) ID() ID {
	return c.id
}

// PackageID returns package id of this CPU.
func (c *CPU) PackageID() ID {
	return c.pkg
}

// NodeID returns the node id of this CPU.
func (c *CPU) NodeID() ID {
	return c.node
}

// CoreID returns the core id of this CPU (lowest CPU id of all thread siblings).
func (c *CPU) CoreID() ID {
	return c.threads.SortedMembers()[0]
}

// ThreadCPUSet returns the CPUSet for all threads in this core.
func (c *CPU) ThreadCPUSet() cpuset.CPUSet {
	return c.threads.CPUSet()
}

// FrequencyRange returns the frequency range for this CPU.
func (c *CPU) FrequencyRange() CPUFreq {
	return c.freq
}

// Online returns if this CPU is online.
func (c *CPU) Online() bool {
	return c.online
}

// Isolated returns if this CPU is isolated.
func (c *CPU) Isolated() bool {
	return c.isolated
}

// SetFrequencyLimits sets the frequency scaling limits for this CPU.
func (c *CPU) SetFrequencyLimits(min, max uint64) error {
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

	sys.nodes = make(map[ID]*Node)

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
	node := &Node{path: path, id: getEnumeratedID(path)}

	if _, err := readSysfsEntry(path, "cpulist", &node.Cpus, ","); err != nil {
		return err
	}
	if _, err := readSysfsEntry(path, "distance", &node.distance); err != nil {
		return err
	}

	sys.nodes[node.id] = node

	return nil
}

// ID returns id of this node.
func (n *Node) ID() ID {
	return n.id
}

// PackageID returns the id of this node.
func (n *Node) PackageID() ID {
	return n.pkg
}

// CPUSet returns the CPUSet for all cores/threads in this node.
func (n *Node) CPUSet() cpuset.CPUSet {
	return n.Cpus.CPUSet()
}

// Distance returns the distance vector for this node.
func (n *Node) Distance() []int {
	return n.distance
}

// DistanceFrom returns the distance of this and a given node.
func (n *Node) DistanceFrom(id ID) int {
	if int(id) < len(n.distance) {
		return n.distance[int(id)]
	}

	return -1
}

// MemoryInfo memory info for the node (partial content from the meminfo sysfs entry).
func (n *Node) MemoryInfo() (*MemInfo, error) {
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

// Discover physical packages (CPU sockets) present in the system.
func (sys *System) discoverPackages() error {
	if sys.packages != nil {
		return nil
	}

	sys.packages = make(map[ID]*Package)

	for _, cpu := range sys.cpus {
		pkg, found := sys.packages[cpu.pkg]
		if !found {
			pkg = &Package{
				id:    cpu.pkg,
				Cpus:  NewIDSet(),
				nodes: NewIDSet(),
			}
			sys.packages[cpu.pkg] = pkg
		}
		pkg.Cpus.Add(cpu.id)
		pkg.nodes.Add(cpu.node)
	}

	return nil
}

// ID returns the id of this package.
func (p *Package) ID() ID {
	return p.id
}

// CPUSet returns the CPUSet for all cores/threads in this package.
func (p *Package) CPUSet() cpuset.CPUSet {
	return p.Cpus.CPUSet()
}

// NodeIDs returns the NUMA node ids for this package.
func (p *Package) NodeIDs() []ID {
	return p.nodes.SortedMembers()
}

// Discover cache associated with the given CPU.
// Notes:
//     I'm not sure how to interpret the cache information under sysfs. This code is now effectively
//     disabled by forcing the associated discovery bit off in the discovery flags.
func (sys *System) discoverCache(path string) error {
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
