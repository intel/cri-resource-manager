// Copyright 2021 Intel Corporation. All Rights Reserved.
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

// The damon tracker.
// https://damonitor.github.io/doc/html/latest-damon/admin-guide/mm/damon/usage.html
// https://damonitor.github.io/doc/html/latest-damon/vm/damon/design.html

package memtier

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type TrackerDamonConfig struct {
	// Connection specifies how to connect to the damon. "perf"
	// connects by tracing damon:aggregated using perf. Options
	// can be appended to the perf trace command. For example,
	// trace only address ranges where accesses have been detected
	// by adding a filter: "perf --filter nr_accesses>0".
	// The default is "bpftrace".
	Connection string
	// SamplingUs is the sampling interval in debugfs/damon attrs
	// (microseconds).
	SamplingUs uint64
	// AggregationUs is the aggregation interval in debugfs/damon
	// attrs (microseconds).
	AggregationUs uint64
	// RegionsUpdateUs is the regions update interval in
	// debugfs/damon attrs (microseconds).
	RegionsUpdateUs uint64
	// MinTargetRegions is the minimum number of monitoring target
	// regions in debugfs/damon attrs.
	MinTargetRegions uint64
	// MaxTargetRegions is the maximum number of monitoring target
	// regions in debugfs/damon attrs.
	MaxTargetRegions uint64
	// FilterAddressRangeSizeMax sets the maximum size for address
	// ranges reported by DAMON. The DAMON aggregations may report
	// start and end addresses from different memory mappings, or
	// they may be from the same memory mapping but so large that
	// the information is not very reliable.
	// The default is 33554432 (32 MB).
	FilterAddressRangeSizeMax uint64
	// Interface: 0 is autodetect, 1 is sysfs, 2 is debugfs
	Interface int
	// SysfsRegionsManager: 0 is DAMON, 1 is memtierd (write targets/TID/regions/RID/{start,stop})
	SysfsRegionsManager int
	// KdamondsList contains kdamond instances available for this
	// tracker instance. The default is to use any kdamond that
	// has 0 contexts.
	KdamondsList []int
	// KdamondsNr is the number of kdamonds to initialize in the
	// system if none exists when the tracker is configured.
	NrKdamonds int
}

const (
	trackerDamonDebugfsPath   string = "/sys/kernel/debug/damon"
	trackerDamonSysfsPath     string = "/sys/kernel/mm/damon/admin/kdamonds"
	trackerDamonSysfsKdamonds int    = 32
)

type DamonUserspaceInterface interface {
	// ApplyAttrs (re)configures DAMON with TrackerDamonConfig parameters.
	ApplyAttrs(config *TrackerDamonConfig) error
	// ApplyTargetIds replaces PIDs to be tracked in the DAMON interface.
	ApplyTargetIds(pids []int) error
	// ApplyState switches DAMON state on/off.
	ApplyState(value string) error
	// AggregatedPid returns the PID of the tracked workload on an aggregation line.
	AggregatedPid(kdamondPid int, targetId uint64) int
	KdamondPids() []int
}

type damonDebugfs struct {
	appliedPids []int
}

type kdamondInfo struct {
	id          int
	state       string
	targetIdPid []int
	targetsPath string
	statePath   string
	pidPath     string
	attrsPath   string
}

type damonSysfs struct {
	nrKdamonds   int   // number of kdamonds available in the system
	kdamondsList []int // kdamonds instances available for this tracker
	kdamonds     []*kdamondInfo
	// // kdamondIndexPids: index in the kdamondsList (not in the system) -> pids of tracked worloads
	// kdamondIndexPids [][]int
	// kdamondPid -> targetId -> pid of tracked workload
	kdamondPidTargetIdPid map[int][]int
	regionsManager        int
	isRunning             bool
}

type TrackerDamon struct {
	mutex             sync.Mutex
	config            *TrackerDamonConfig
	ifaceAvailSysfs   bool
	ifaceAvailDebugfs bool
	iface             DamonUserspaceInterface
	pids              []int
	started           bool
	toPerfReader      chan byte
	toBpftraceReader  chan byte
	// accesses maps pid -> startAddr -> lengthPgs -> accessCount
	accesses   map[int]map[uint64]map[uint64]uint64
	tidpid     map[int64]int
	lostEvents uint
	raes       rawAccessEntries
}

func init() {
	TrackerRegister("damon", NewTrackerDamon)
}

func NewTrackerDamon() (Tracker, error) {
	t := TrackerDamon{
		ifaceAvailDebugfs: procFileExists(trackerDamonDebugfsPath),
		ifaceAvailSysfs:   procFileExists(trackerDamonSysfsPath),
		accesses:          make(map[int]map[uint64]map[uint64]uint64),
		tidpid:            make(map[int64]int),
	}

	if !t.ifaceAvailDebugfs && !t.ifaceAvailSysfs {
		return nil, fmt.Errorf("no platform support: both %q and %q missing", trackerDamonSysfsPath, trackerDamonDebugfsPath)
	}

	// if err := t.iface.ApplyState("off"); err != nil {
	// 	return nil, err
	// }
	return &t, nil
}

func (t *TrackerDamon) SetConfigJson(configJson string) error {
	config := &TrackerDamonConfig{}
	if configJson != "" {
		if err := unmarshal(configJson, config); err != nil {
			return err
		}
	}
	if config.Connection == "" {
		config.Connection = "bpftrace"
	}
	if !strings.HasPrefix(config.Connection, "perf") && config.Connection != "bpftrace" {
		return fmt.Errorf("invalid damon connection %q, supported: \"perf [options]\" \"bpftrace\"", config.Connection)
	}
	if config.SamplingUs == 0 {
		config.SamplingUs = 5000 // sampling interval, 5 ms
	}
	if config.AggregationUs == 0 {
		config.AggregationUs = 100000 // aggregation interval, 100 ms
	}
	if config.RegionsUpdateUs == 0 {
		config.RegionsUpdateUs = 1000000 // regions update interval, 1 s
	}
	if config.MinTargetRegions == 0 {
		config.MinTargetRegions = 10
	}
	if config.MaxTargetRegions == 0 {
		config.MaxTargetRegions = 1000
	}
	if config.FilterAddressRangeSizeMax == 0 {
		config.FilterAddressRangeSizeMax = 32 * 1024 * 1024
	}
	if t.ifaceAvailSysfs && config.Interface != 2 {
		t.iface = &damonSysfs{}
	} else {
		t.iface = &damonDebugfs{}
	}
	if err := t.iface.ApplyAttrs(config); err != nil {
		return err
	}
	t.config = config
	return nil
}

func (t *TrackerDamon) GetConfigJson() string {
	if t.config == nil {
		return ""
	}
	if configStr, err := json.Marshal(t.config); err == nil {
		return string(configStr)
	}
	return ""
}

func (t *TrackerDamon) AddPids(pids []int) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	log.Debugf("TrackerDamon.AddPids(%v)\n", pids)
	for _, pid := range pids {
		t.pids = append(t.pids, pid)
	}
	if t.started {
		t.iface.ApplyState("off")
		t.applyPids()
		t.iface.ApplyState("on")
		t.updateKdamondConnection()
	}
}

func (t *TrackerDamon) RemovePids(pids []int) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	log.Debugf("TrackerDamon.RemovePids(%v)\n", pids)
	if pids == nil {
		t.pids = []int{}
		return
	}
	for _, pid := range pids {
		t.removePid(pid)
	}
	if t.started {
		t.iface.ApplyState("off")
		t.applyPids()
		t.iface.ApplyState("on")
		t.updateKdamondConnection()
	}
}

func (t *TrackerDamon) removePid(pid int) {
	for index, p := range t.pids {
		if p == pid {
			if index < len(t.pids)-1 {
				t.pids[index] = t.pids[len(t.pids)-1]
			}
			t.pids = t.pids[:len(t.pids)-1]
			break
		}
	}
}

func (t *TrackerDamon) applyPids() error {
	lostPids := []int{}
	for _, pid := range t.pids {
		pidStr := strconv.Itoa(pid)
		if !procFileExists("/proc/" + pidStr) {
			lostPids = append(lostPids, pid)
		}
	}
	for _, pid := range lostPids {
		t.removePid(pid)
	}
	return t.iface.ApplyTargetIds(t.pids)
}

func (debugfs *damonDebugfs) ApplyAttrs(config *TrackerDamonConfig) error {
	utoa := func(u uint64) string { return strconv.FormatUint(u, 10) }
	configStr := utoa(config.SamplingUs) +
		" " + utoa(config.AggregationUs) +
		" " + utoa(config.RegionsUpdateUs) +
		" " + utoa(config.MinTargetRegions) +
		" " + utoa(config.MaxTargetRegions) + "\n"
	if err := procWrite(trackerDamonDebugfsPath+"/attrs", []byte(configStr)); err != nil {
		return fmt.Errorf("when writing %q: %w", configStr, err)
	}
	return nil
}

func (debugfs *damonDebugfs) ApplyTargetIds(pids []int) error {
	// Refresh all pids to be monitored.
	// Writing a non-existing pids to target_ids causes an error.
	appliedPids := make([]int, 0, len(pids))
	pidsStrs := make([]string, 0, len(pids))
	for _, pid := range pids {
		pidsStrs = append(pidsStrs, strconv.Itoa(pid))
		appliedPids = append(appliedPids, pid)
	}
	pidsStr := strings.Join(pidsStrs, " ")
	if err := procWrite(trackerDamonDebugfsPath+"/target_ids", []byte(pidsStr)); err != nil {
		return err
	}
	debugfs.appliedPids = appliedPids
	return nil
}

func (debugfs *damonDebugfs) ApplyState(value string) error {
	monitorFilename := trackerDamonDebugfsPath + "/monitor_on"
	currentStatus, err := procRead(monitorFilename)
	if err != nil {
		return fmt.Errorf("reading %q failed before writing it: %w", monitorFilename, err)
	}
	if currentStatus[:2] == value[:2] {
		return nil // already correct value, skip writing (might cause an error)
	}
	if err = procWrite(monitorFilename, []byte(value)); err != nil {
		return err
	}
	newStatus, err := procRead(monitorFilename)
	if err != nil {
		return fmt.Errorf("reading %q failed after setting it: %w", monitorFilename, err)
	}
	if newStatus[:2] != value[:2] {
		return fmt.Errorf("wrote %q to %q, but value is still %q", value, monitorFilename, newStatus)
	}
	return nil
}

func (debugfs *damonDebugfs) AggregatedPid(kdamondPid int, targetId uint64) int {
	if targetId < uint64(len(debugfs.appliedPids)) {
		return debugfs.appliedPids[targetId]
	}
	return 0
}

func (debugfs *damonDebugfs) KdamondPids() []int {
	pathKdamondPid := trackerDamonDebugfsPath + "/kdamond_pid"
	kpid, err := procReadInt(pathKdamondPid)
	if err != nil || kpid == 0 {
		log.Debugf("damonDebugfs.KdamondPids: failed to read %q: %s", pathKdamondPid, err)
		return []int{}
	}
	return []int{kpid}
}

func (sysfs *damonSysfs) initialize(config *TrackerDamonConfig) error {
	if sysfs.nrKdamonds != 0 {
		return fmt.Errorf("damonSysfs interface already initialized: %+v", sysfs)
	}
	sysfs.regionsManager = config.SysfsRegionsManager
	switch sysfs.regionsManager {
	case 0:
		log.Debugf("damonSysfs.initialize: regions will be chosen by DAMON")
	case 1:
		log.Debugf("damonSysfs.initialize: regions will be written by TrackerDamon")
	}
	// Modifying nr_kdamonds is possible only if all kdamonds are
	// off, and it destroys all contexts and tracked pids in
	// them. Therefore initialize() initializes them only if they
	// are not already initializes nr_kdamonds only if it has
	// value 0 in the system. Any other value means that
	// nr_kdamonds is managed by someone else, and it will not be
	// touched.
	//
	// Then trace only the pids of kdamonds in order to
	// allow multiple sets of processes to be tracked with
	// DAMON simultaneously:
	//
	// bpftrace -e 'tracepoint:damon:damon_aggregated / pid == 52100 || pid == 52101 / { printf( ... ) } '
	nrKdamondsPath := trackerDamonSysfsPath + "/nr_kdamonds"
	globalNrKdamonds, err := procReadInt(nrKdamondsPath)
	if err != nil {
		return fmt.Errorf("damon sysfs.initialize: failed to read %q: %w", nrKdamondsPath, err)
	}
	if globalNrKdamonds == 0 {
		if config.NrKdamonds == 0 {
			return fmt.Errorf("no kdamonds available in the system (%q) and DAMON tracker configuration NrKdamonds equals 0. Either one must be > 0.", nrKdamondsPath)
		}
		if err = procWriteInt(nrKdamondsPath, config.NrKdamonds); err != nil {
			return fmt.Errorf("writing DAMON tracker configuration NrKdamonds (%d) to %q failed: %s", config.NrKdamonds, nrKdamondsPath, err)
		}
		globalNrKdamonds = config.NrKdamonds
	}
	sysfs.nrKdamonds = globalNrKdamonds
	if len(config.KdamondsList) > 0 {
		// Take control over all kdamonds that have been listed for this damon tracker
		for _, kdamondId := range config.KdamondsList {
			if kdamondId >= sysfs.nrKdamonds {
				return fmt.Errorf("illegal kdamond %d in DAMON tracker configuration KdamondsList: last available kdamond in system is %d", kdamondId, sysfs.nrKdamonds-1)
			}
			contextsPath := fmt.Sprintf("%s/%d/contexts", trackerDamonSysfsPath, kdamondId)
			nrContextsPath := fmt.Sprintf("%s/%d/contexts/nr_contexts", trackerDamonSysfsPath, kdamondId)
			targetsPath := fmt.Sprintf("%s/%d/contexts/0/targets", trackerDamonSysfsPath, kdamondId)
			statePath := fmt.Sprintf("%s/%d/state", trackerDamonSysfsPath, kdamondId)
			pidPath := fmt.Sprintf("%s/%d/pid", trackerDamonSysfsPath, kdamondId)
			attrsPath := fmt.Sprintf("%s/%d/contexts/0/monitoring_attrs", trackerDamonSysfsPath, kdamondId)
			if currState, err := procReadTrimmed(statePath); currState != "off" && err == nil {
				log.Warnf("taking control over kdamond %d despite %q was %q", kdamondId, statePath, currState)
				if err = procWrite(statePath, []byte("off")); err != nil {
					return fmt.Errorf("failed to switch off %q", statePath)
				}
			}
			if err = procWriteInt(nrContextsPath, 1); err != nil {
				return fmt.Errorf("kdamond context creation failed: error when writing 1 to %q: %w", nrContextsPath, err)
			}
			if err = procWrite(contextsPath+"/0/operations", []byte("vaddr")); err != nil {
				return fmt.Errorf("kdamond context operation \"vaddr\" failed: %w", err)
			}
			sysfs.kdamonds = append(sysfs.kdamonds, &kdamondInfo{
				id:          kdamondId,
				targetsPath: targetsPath,
				statePath:   statePath,
				pidPath:     pidPath,
				attrsPath:   attrsPath,
			})
			sysfs.kdamondsList = append(sysfs.kdamondsList, kdamondId)
		}
	} else {
		return fmt.Errorf("missing DAMON configuration kdamondslist")
	}
	return nil
}

func (sysfs *damonSysfs) ApplyAttrs(config *TrackerDamonConfig) error {
	if sysfs.nrKdamonds == 0 {
		if err := sysfs.initialize(config); err != nil {
			log.Debugf("damonSysfs.ApplyAttrs: initialization failed: %s", err)
			return err
		}
	}
	for _, kdamond := range sysfs.kdamonds {
		fnameValue := map[string]uint64{
			filepath.Join(kdamond.attrsPath, "intervals", "aggr_us"):   config.AggregationUs,
			filepath.Join(kdamond.attrsPath, "intervals", "sample_us"): config.SamplingUs,
			filepath.Join(kdamond.attrsPath, "intervals", "update_us"): config.RegionsUpdateUs,
			filepath.Join(kdamond.attrsPath, "nr_regions", "min"):      config.MinTargetRegions,
			filepath.Join(kdamond.attrsPath, "nr_regions", "max"):      config.MaxTargetRegions,
		}
		for fname, value := range fnameValue {
			if err := procWriteUint64(fname, value); err != nil {
				return fmt.Errorf("failed to write %d to %q: %w", value, fname, err)
			}
		}
	}
	return nil
}

func (sysfs *damonSysfs) ApplyTargetIds(pids []int) error {
	/* TODO: optimize, small changes in pids could be limited to
	   only some kdamonds, thus some of them could be kept running
	   without a reset. */
	sysfs.ApplyState("off")
	kdamondIndexPids := make([][]int, len(sysfs.kdamondsList))
	kdamondIndex := 0
	for _, pid := range pids {
		kdamondIndexPids[kdamondIndex] = append(kdamondIndexPids[kdamondIndex], pid)
		kdamondIndex++
		if kdamondIndex >= len(sysfs.kdamonds) {
			kdamondIndex = 0
		}
	}
	log.Debugf("damonSysfs.ApplyTargetIds(%v): kdamondIndexPids=%v", pids, kdamondIndexPids)
	for kdamondIndex, kdamond := range sysfs.kdamonds {
		log.Debugf("damonSysfs.ApplyTargetIds: writing targets to kdamondIndex=%d kdamond=%+v", kdamondIndex, kdamond)
		kdamond.targetIdPid = kdamondIndexPids[kdamondIndex]
		nrTargetsPath := filepath.Join(kdamond.targetsPath, "nr_targets")
		nrTargets := len(kdamond.targetIdPid)
		if err := procWriteInt(nrTargetsPath, nrTargets); err != nil {
			return fmt.Errorf("failed to write pid count %d to %q: %w", nrTargets, nrTargetsPath, err)
		}
		for targetId, pid := range kdamond.targetIdPid {
			addrRanges, err := procMaps(pid)
			if err != nil || len(addrRanges) == 0 {
				// pid is gone or it has no interesting address ranges
				return fmt.Errorf("failed to read address ranges of pid %d: %w", pid, err)
			}
			pidTargetPath := filepath.Join(kdamond.targetsPath, strconv.Itoa(targetId), "pid_target")
			if err := procWriteInt(pidTargetPath, pid); err != nil {
				return fmt.Errorf("failed to write pid %d to %q: %w", pid, pidTargetPath, err)
			}
			if sysfs.regionsManager == 0 {
				// damonSysfs is not expected to manage resources of targets.
				// Skip the rest of the loop that would write targets/TID/regions/*.
				continue
			}
			regionsPath := filepath.Join(kdamond.targetsPath, strconv.Itoa(targetId), "regions")
			nrRegionsPath := filepath.Join(regionsPath, "nr_regions")
			if err := procWriteInt(nrRegionsPath, len(addrRanges)); err != nil {
				return fmt.Errorf("failed to write address range count %d to %q: %w", len(addrRanges), nrRegionsPath, err)
			}
			for regionId, ar := range addrRanges {
				startPath := filepath.Join(regionsPath, strconv.Itoa(regionId), "start")
				endPath := filepath.Join(regionsPath, strconv.Itoa(regionId), "end")
				if err := procWriteUint64(startPath, ar.Addr()); err != nil {
					return fmt.Errorf("failed to write region start address %d (%x) of pid %d to %q: %w", ar.Addr(), ar.Addr(), pid, startPath, err)
				}
				if err := procWriteUint64(endPath, ar.EndAddr()); err != nil {
					return fmt.Errorf("failed to write region end address %d (%x) of pid %d to %q: %w", ar.EndAddr(), ar.EndAddr(), pid, endPath, err)
				}
			}
		}
	}
	return nil
}

func (sysfs *damonSysfs) ApplyState(value string) error {
	for _, kdamond := range sysfs.kdamonds {
		if value != kdamond.state &&
			((value == "on" && len(kdamond.targetIdPid) > 0) ||
				(value != "on")) {
			// Writing "off" to state causes an error if kdamond was already off.
			// Ignore that error, but not others.
			if err := procWrite(kdamond.statePath, []byte(value)); value != "off" && err != nil {
				return fmt.Errorf("failed to write state %q to %q: %w", value, kdamond.statePath, err)
			}
			kdamond.state = value
		}
	}
	switch value {
	case "off":
		sysfs.isRunning = false
	case "on":
		sysfs.isRunning = true
		// Writing "on" to kdamond state launches a kdamond
		// kernel thread. Read pids of all our kdamond threads
		// in order to map aggregation tracepoint information
		// (kdamond pid and target id) to the pid of the
		// workload.
		sysfs.kdamondPidTargetIdPid = map[int][]int{}
		for _, kdamond := range sysfs.kdamonds {
			kdamondPid, err := procReadInt(kdamond.pidPath)
			if err != nil {
				log.Debugf("damonSysfs.ApplyState(\"on\"): cannot read from %q: %s", kdamond.pidPath, err)
				return fmt.Errorf("failed to read kdamond pid from %q: %w", kdamond.pidPath, err)
			}
			log.Debugf("damonSysfs.ApplyState(\"on\"): %q: %d", kdamond.pidPath, kdamondPid)
			if kdamondPid > 0 {
				sysfs.kdamondPidTargetIdPid[kdamondPid] = kdamond.targetIdPid
			}
		}
	default: // other states do not affect isRunning
	}
	return nil
}

func (sysfs *damonSysfs) AggregatedPid(kdamondPid int, targetId uint64) int {
	if targetIdPid, ok := sysfs.kdamondPidTargetIdPid[kdamondPid]; ok {
		if targetId < uint64(len(targetIdPid)) {
			return targetIdPid[targetId]
		} else {
			stats.Store(StatsHeartbeat{"TrackerDamon.sysfs.AggregatedPid: unknown targetId"})
		}
	} else {
		stats.Store(StatsHeartbeat{"TrackerDamon.sysfs.AggregatedPid: unknown kdamond pid"})
	}
	return 0
}

func (sysfs *damonSysfs) KdamondPids() []int {
	kpids := make([]int, len(sysfs.kdamondPidTargetIdPid))
	i := 0
	for kdamondPid := range sysfs.kdamondPidTargetIdPid {
		kpids[i] = kdamondPid
		i++
	}
	return kpids
}

// updateKdamondConnection() ensures that memory access data keeps
// flowing from correct kdamond processes to the tracker.
func (t *TrackerDamon) updateKdamondConnection() error {
	switch {
	case t.toPerfReader == nil && t.toBpftraceReader == nil:
		// Connection has not been updated before. Initialize
		// correct connection.
		if strings.HasPrefix(t.config.Connection, "perf") && t.toPerfReader == nil {
			t.toPerfReader = make(chan byte, 1)
			go t.perfReader()
		} else if strings.HasPrefix(t.config.Connection, "bpftrace") && t.toBpftraceReader == nil {
			t.toBpftraceReader = make(chan byte, 1)
			go t.bpftraceReader()
		} else {
			return fmt.Errorf("invalid Connection in TrackerDamon configuration: %q", t.config.Connection)
		}
		return nil
	case t.toBpftraceReader != nil:
		t.toBpftraceReader <- 1 // signal bpftrace reader to update kdamond pidsd
	case t.toPerfReader != nil:
		// nothing to do, perf reader does not support kdamond pid filtering at the moment
	}
	return nil
}

func (t *TrackerDamon) Start() error {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	// Reset configuration.
	if t.config == nil {
		if err := t.SetConfigJson(""); err != nil {
			return fmt.Errorf("start failed on default configuration error: %w", err)
		}
	}
	t.iface.ApplyState("off")
	t.iface.ApplyAttrs(t.config)
	t.applyPids()

	// Even if damon start monitor fails, the tracker state is
	// "started" from this point on. That is, removing bad pids
	// and adding new pids will try restarting monitor.
	t.started = true

	// Start monitoring.
	if len(t.pids) > 0 {
		if err := t.iface.ApplyState("on"); err != nil {
			return err
		}
		log.Debugf("TrackerDamon.Start: monitoring is on")
	}
	// Establish connection to monitoring processes.
	if err := t.updateKdamondConnection(); err != nil {
		t.iface.ApplyState("off") // ignore error, starting has failed already.
		return fmt.Errorf("TrackerDamon.Start: %w", err)
	}
	return nil
}

func (t *TrackerDamon) Stop() {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	log.Debugf("TrackerDamon.Stop()")
	// Never mind about error: may cause "Operation not permitted"
	// if monitoring was already off.
	t.iface.ApplyState("off")
	t.started = false
	if t.toPerfReader != nil {
		log.Debugf("TrackerDamon.Stop: stopping perfReader")
		t.toPerfReader <- 0
	}
	if t.toBpftraceReader != nil {
		log.Debugf("TrackerDamon.Stop: stopping bpftraceReader")
		t.toBpftraceReader <- 0
	}
}

func (t *TrackerDamon) ResetCounters() {
	// TODO: lock!? so that perfReader wouldn't need lock on every line?
	t.mutex.Lock()
	defer t.mutex.Unlock()
	if t.lostEvents > 0 {
		log.Debugf("TrackerDamon.ResetCounters: events lost %d\n", t.lostEvents)
	}
	t.accesses = make(map[int]map[uint64]map[uint64]uint64)
	t.lostEvents = 0
}

func (t *TrackerDamon) GetCounters() *TrackerCounters {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	tcs := &TrackerCounters{}
	for pid, startLengthCount := range t.accesses {
		for start, lengthCount := range startLengthCount {
			for length, count := range lengthCount {
				addrRange := AddrRanges{
					pid: pid,
					addrs: []AddrRange{
						{
							addr:   start,
							length: length,
						},
					},
				}
				tc := TrackerCounter{
					Accesses: count,
					Reads:    0,
					Writes:   0,
					AR:       &addrRange,
				}
				*tcs = append(*tcs, tc)
			}
		}
	}
	return tcs
}

func (t *TrackerDamon) bpftraceParser(bpftraceOutput *bufio.Reader) {
	/* bpftrace output handler routine */
	// from /sys/kernel/tracing/events/damon/damon_aggregated/format: ...
	// format: ...
	//         field:unsigned long target_id;  offset:8;       size:8; signed:0;
	//         field:unsigned int nr_regions;  offset:16;      size:4; signed:0;
	//         field:unsigned long start;      offset:24;      size:8; signed:0;
	//         field:unsigned long end;        offset:32;      size:8; signed:0;
	//         field:unsigned int nr_accesses; offset:40;      size:4; signed:0;
	//         field:unsigned int age; offset:44;      size:4; signed:0;
	var targetId uint64
	var start, end uint64
	var nrAccesses, age uint
	var kdamondPid int
	for true {
		_, err := fmt.Fscanf(bpftraceOutput, "%d %d %x %x %d %d\n", &kdamondPid, &targetId, &start, &end, &nrAccesses, &age)
		if err != nil {
			log.Debugf("TrackerDamon.bpftraceParser: unexpected output error: %s", err)
			stats.Store(StatsHeartbeat{fmt.Sprintf("TrackerDamon.bpftraceParser.error: %s", err)})
			break
		}
		stats.Store(StatsHeartbeat{"TrackerDamon.bpftraceParser.line"})
		pid := t.iface.AggregatedPid(kdamondPid, targetId)
		t.storeAggregated(pid, start, end, nrAccesses, age)
	}
	log.Debugf("TrackerDamon.bpftraceParser: exit")
}

func (t *TrackerDamon) bpftraceStart(kpids []int) (*exec.Cmd, *bufio.Reader, error) {
	filters := []string{}
	if len(kpids) > 0 {
		pidFilters := make([]string, len(kpids))
		for i, kpid := range kpids {
			pidFilters[i] = fmt.Sprintf("pid == %d", kpid)
		}
		filters = append(filters, strings.Join(pidFilters, " || "))
	}
	if t.config.FilterAddressRangeSizeMax > 1 {
		filters = append(filters, fmt.Sprintf("args->end - args->start <= %d", t.config.FilterAddressRangeSizeMax))
	}
	filterStr := ""
	if len(filters) > 0 {
		filterStr = "/ (" + strings.Join(filters, ") && (") + ") /"
	}
	bpftraceProgram := "tracepoint:damon:damon_aggregated " + filterStr + " { printf(\"%d %ld %lx %lx %d %d\\n\", pid, args->target_id, args->start, args->end, args->nr_accesses, args->age); }"
	log.Debugf("TrackerDamon.bpftraceStart: command: bpftrace -e \"%v\"", bpftraceProgram)
	cmd := exec.Command("bpftrace", "-e", bpftraceProgram)
	outPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("creating stdout pipe for bpftrace failed: %w", err)
	}
	bpftraceOutput := bufio.NewReader(outPipe)
	log.Debugf("TrackerDamon: launching bpftrace...")
	if err = cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("starting bpftrace failed: %w", err)
	} else {
		/* Read the "Attaching 1 probe..." line */
		_, err := fmt.Fscanf(bpftraceOutput, "Attaching 1 probe...\n")
		if err != nil {
			return nil, nil, fmt.Errorf("TrackerDamon.bpftraceStart: reading the first line of bpftrace output failed: %s", err)
		}
		log.Debugf("TrackerDamon.bpftraceStart: bpftrace started successfully")
	}
	return cmd, bpftraceOutput, nil
}

func (t *TrackerDamon) bpftraceReader() error {
	log.Debugf("TrackerDamon.bpftraceReader: online\n")
	defer log.Debugf("TrackerDamon.bpftraceReader: offline\n")
	loop := true
	for loop {
		var cmd *exec.Cmd
		var bpftraceOutput *bufio.Reader
		var err error
		kpids := t.iface.KdamondPids()
		if len(kpids) > 0 {
			log.Debugf("TrackerDamon.bpftraceReader: kdamond pids %v", kpids)
			cmd, bpftraceOutput, err = t.bpftraceStart(kpids)
			if err == nil {
				go t.bpftraceParser(bpftraceOutput)
			} else {
				log.Errorf("TrackerDamon.bpftraceReader: bpftrace start failed: %s", err)
				break
			}
		} else {
			log.Debugf("TrackerDamon.bpftraceReader: no kdamond pids, wait")
		}
		switch <-t.toBpftraceReader {
		case 0:
			log.Debugf("TrackerDamon.bpftraceReader: quitting")
			loop = false
		case 1:
			log.Debugf("TrackerDamon.bpftraceReader: restarting")
		default:
			log.Debugf("trackerDamon.bpftraceReader: unexpected value from the toBpftraceReader channel")
		}
		if cmd != nil {
			if err := cmd.Process.Kill(); err != nil {
				log.Debugf("TrackerDamon.bpftraceReader: bpftrace kill error: %s\n", err)
			} else {
				log.Debugf("TrackerDamon.bpftraceReader: bpftrace signaled, waiting to terminate")
				cmd.Wait()
				log.Debugf("TrackerDamon.bpftraceReader: bpftrace terminated")
			}
		}
	}
	close(t.toBpftraceReader)
	t.toBpftraceReader = nil
	return nil
}

func (t *TrackerDamon) perfReader() error {
	log.Debugf("TrackerDamon.perfReader: online\n")
	defer log.Debugf("TrackerDamon.perfReader: offline\n")
	// Tracing without filtering produces many "LOST n events!" lines
	// and a lot of information that we might not even need:
	// ranges were sampling didn't find any accesses.
	//
	// Currently we handle only lines where sampling found accesses.
	// TODO: If we keep it like this, our heatmap should have
	// cool-down for regions where we don't get any reports but that
	// are still in process's address space. Now those regions are
	// considered possibly free()'d by tracked process.
	perfTraceArgs := []string{"trace", "-e", "damon:damon_aggregated", "--libtraceevent_print"}
	perfExtraArgs := strings.Split(t.config.Connection, " ")[1:]
	perfArgs := append(perfTraceArgs, perfExtraArgs...)
	cmd := exec.Command("perf", perfArgs...)
	errPipe, err := cmd.StderrPipe()
	perfOutput := bufio.NewReader(errPipe)
	if err != nil {
		return fmt.Errorf("creating stderr pipe for perf failed: %w", err)
	}
	log.Debugf("TrackerDamon.perfReader: launching perf...\n")
	if err = cmd.Start(); err != nil {
		return fmt.Errorf("starting perf failed: %w", err)
	}
	perfLines := make(chan string, 1024)
	go func() {
		for true {
			line, err := perfOutput.ReadString('\n')
			if err != nil || line == "" {
				break
			}
			perfLines <- line
		}
		if t.toPerfReader != nil {
			t.toPerfReader <- 0
		}
	}()
	quit := false
	for !quit {
		stats.Store(StatsHeartbeat{"TrackerDamon.perfReader"})
		select {
		case line := <-perfLines:
			if line == "" {
				quit = true
			}
			if err := t.perfHandleLine(line); err != nil {
				log.Debugf("TrackerDamon.perfReader: perf parse error: %s\n", err)
			}
		case <-t.toPerfReader:
			close(t.toPerfReader)
			t.toPerfReader = nil
			if err := cmd.Process.Kill(); err != nil {
				log.Debugf("TrackerDamon.perfReader: perf kill error: %s\n", err)
			}
			perfLines <- ""
			quit = true
		}
	}
	cmd.Wait()
	return nil
}

// legacyTargetIdToPid is an opportunistic and unreliable way of
// trying to guess the pid of the workload from targetId reported by
// old versions of DAMON (before Linux 5.17).
func (t *TrackerDamon) legacyTargetIdToPid(targetId int64, start uint64, end uint64, targetIdIsPidIndex bool) int {
	// If targetId is already mapped to pid, return it.
	if pid, ok := t.tidpid[targetId]; ok {
		return pid
	}

	if len(t.pids) == 1 {
		t.tidpid[targetId] = t.pids[0]
		return t.tidpid[targetId]
	}

	if targetIdIsPidIndex && targetId > 0 && targetId < int64(len(t.pids)) {
		return t.pids[targetId]
	}

	// Unseen targetId. Read address ranges of all current
	// processes. If we would go through only address ranges we
	// have seen sometime earlier, we might end up trusting only
	// matching address range yet that would belong to a wrong
	// processs.
	stats.Store(StatsHeartbeat{"TrackerDamon.targetIdToPid:read /proc/PID/*maps"})
	matchingPid := 0
	matchingPids := 0
	for _, pid := range t.pids {
		arlist, err := procMaps(pid)
		if err != nil {
			continue
		}
		for _, ar := range arlist {
			if start >= ar.addr && end < ar.addr+ar.length*constUPagesize {
				matchingPid = pid
				matchingPids += 1
				break
			}
		}
	}
	if matchingPids == 1 {
		log.Debugf("TrackerDamon: associating tid=%d with pid=%d\n", targetId, matchingPid)
		t.tidpid[targetId] = matchingPid
		return matchingPid
	}
	return 0
}

func (t *TrackerDamon) storeAggregated(pid int, start, end uint64, nrAccesses, age uint) error {
	// Filter out address ranges that are too large to be
	// meaningful. The DAMON tracker may sometimes report start
	// and end addresses from separate address ranges.
	if end-start > t.config.FilterAddressRangeSizeMax {
		stats.Store(StatsHeartbeat{"TrackerDamon.storeAggregated:ignored too large address range"})
		return nil
	}
	// TODO: avoid locking this often
	t.mutex.Lock()
	startLengthCount, ok := t.accesses[pid]
	if !ok {
		startLengthCount = make(map[uint64]map[uint64]uint64)
		t.accesses[pid] = startLengthCount
	}
	lengthPgs := (end - start) / constUPagesize
	lengthCount, ok := startLengthCount[uint64(start)]
	if !ok {
		lengthCount = make(map[uint64]uint64)
		startLengthCount[uint64(start)] = lengthCount
	}
	if count, ok := lengthCount[lengthPgs]; ok {
		lengthCount[lengthPgs] = count + uint64(nrAccesses)
	} else {
		lengthCount[lengthPgs] = uint64(nrAccesses)
	}
	t.mutex.Unlock()
	if t.raes.data != nil {
		timestamp := time.Now().UnixNano()
		rae := &rawAccessEntry{
			timestamp: timestamp,
			pid:       pid,
			addr:      uint64(start),
			length:    lengthPgs,
			accessCounter: accessCounter{
				a: uint64(nrAccesses),
			},
		}
		t.raes.store(rae)
	}
	return nil
}

func (t *TrackerDamon) perfHandleLine(line string) error {
	// Parse line. Example of "perf trace -e damon:damon_aggregated --libtraceevent_print" output lines, Linux 5.15, 5.16:
	//   0.000 kdamond.0/1527 damon:damon_aggregated(target_id=18446634001245894528 nr_regions=7 4194304-185102770176: 0)
	// LOST 123 events!
	// (The last three numbers on the first line being start_addr, end_addr and nr_accesses.)
	// Linux 5.17+:
	//   0.030 kdamond.0/262863 damon:damon_aggregated(target_id=0 nr_regions=202 824633720832-824700829696: 0 120)
	// Linux 6.0+:
	// 201.650 kdamond.0/37860 damon:damon_aggregated(target_id: 1, nr_regions: 9, start: 824633720832, end: 824635813888, nr_accesses: 19, age: 201)
	// (The last four numbers being start_addr, end_addr, nr_accesses and age.)
	if strings.HasPrefix(line, "LOST ") {
		stats.Store(StatsHeartbeat{"TrackerDamon.perfHandleLine:events lost"})
		lostEventsStr := strings.Split(line, " ")[1]
		lostEvents, err := strconv.ParseUint(lostEventsStr, 10, 0)
		if err != nil {
			return fmt.Errorf("parse error on lost event count %q line: %s", lostEventsStr, line)
		}
		t.lostEvents += uint(lostEvents)
		return nil
	}
	csLine := strings.Split(strings.TrimSpace(strings.NewReplacer(
		"(", " ",
		")", "",
		":", "",
		"=", " ",
		"-", " ").Replace(line)), " ")
	// After the replacements and trimming, lines are as follows.
	// Linux 5.15, 5.16, followed by field indices in csLine:
	// 0.000 kdamond.0/1527 damon:damon_aggregated target_id 18446634001245894528 nr_regions 7 4194304 185102770176 0
	// 0     1              2                      3         4                    5          6 7       8            9
	// Linux 5.17, followed by field indices in csLine:
	// 0.030 kdamond.0/262863 damon:damon_aggregated target_id 0 nr_regions 202 824633720832 824700829696 0 120
	// 0     1                2                      3         4 5          6   7            8            9 10
	if len(csLine) < 10 {
		stats.Store(StatsHeartbeat{"TrackerDamon.perfHandleLine: parse error: bad line"})
		return fmt.Errorf("bad line %q", csLine)
	}
	if csLine[3] != "target_id" {
		stats.Store(StatsHeartbeat{"TrackerDamon.perfHandleLine: parse error: target_id not found"})
		return fmt.Errorf("target_id not found from %q line %q", csLine[3], line)
	}
	targetIdStr := csLine[4]
	startStr := csLine[7]
	endStr := csLine[8]
	nrStr := csLine[9]
	targetId, err := strconv.ParseUint(targetIdStr, 10, 64)
	if err != nil {
		stats.Store(StatsHeartbeat{"TrackerDamon.perfHandleLine: parse error: target_id syntax error"})
		return fmt.Errorf("parse error (%w) on targetIdStr %q line %q", err, targetIdStr, line)
	}
	start, err := strconv.ParseUint(startStr, 10, 64)
	if err != nil {
		stats.Store(StatsHeartbeat{"TrackerDamon.perfHandleLine: parse error: start address syntax error"})
		return fmt.Errorf("parse error (%w) on startStr %q line %q", err, startStr, line)
	}
	end, err := strconv.ParseUint(endStr, 10, 64)
	if err != nil {
		stats.Store(StatsHeartbeat{"TrackerDamon.perfHandleLine: parse error: end address syntax error"})
		return fmt.Errorf("parse error (%w) on endStr %q line %q", err, endStr, line)
	}
	if start >= end {
		stats.Store(StatsHeartbeat{"TrackerDamon.perfHandleLine: parse error: start addr after end addr"})
		return fmt.Errorf("parse error: start >= end (%d >= %d) line %q", start, end, line)
	}
	nrAccesses, err := strconv.ParseUint(nrStr, 10, 32)
	if err != nil {
		stats.Store(StatsHeartbeat{"TrackerDamon.perfHandleLine: parse error: nr_access syntax error"})
		return fmt.Errorf("parse error (%w) on nrStr %q line %q", err, nrStr, line)
	}
	pid := 0
	if len(csLine) > 10 {
		// Linux 5.17+: target_id is an index in to the pids in the target_id's file.
		pid = t.iface.AggregatedPid(0, targetId)
	} else {
		pid = t.legacyTargetIdToPid(int64(targetId), start, end, false)
	}
	if pid < 1 {
		stats.Store(StatsHeartbeat{"TrackerDamon.perfHandleLine:unknown target id"})
		return nil
	}
	age := uint(0)
	/* age is not parsed */
	t.storeAggregated(pid, start, end, uint(nrAccesses), age)
	return nil
}

func (t *TrackerDamon) Dump(args []string) string {
	usage := "Usage: dump raw PARAMS"
	if len(args) == 0 {
		return usage
	}
	if args[0] == "raw" {
		return t.raes.dump(args[1:])
	}
	return ""
}

/*
tracking a pid with damo / debug output:

write '0' to '/sys/kernel/mm/damon/admin/kdamonds/nr_kdamonds'
read '/sys/kernel/mm/damon/admin/kdamonds/nr_kdamonds': '0'
read '/sys/kernel/mm/damon/admin/kdamonds/nr_kdamonds': '0'
write '1' to '/sys/kernel/mm/damon/admin/kdamonds/nr_kdamonds'
read '/sys/kernel/mm/damon/admin/kdamonds/nr_kdamonds': '0'
write '1' to '/sys/kernel/mm/damon/admin/kdamonds/nr_kdamonds'
read '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/nr_contexts': '0'
write '1' to '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/nr_contexts'
read '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/targets/nr_targets': '0'
write '1' to '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/targets/nr_targets'
read '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/targets/0/regions/nr_regions': '0'
read '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/schemes/nr_schemes': '0'
write '1' to '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/schemes/nr_schemes'
write 'vaddr' to '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/operations'
write '5000' to '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/monitoring_attrs/intervals/sample_us'
write '100000' to '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/monitoring_attrs/intervals/aggr_us'
write '1000000' to '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/monitoring_attrs/intervals/update_us'
write '10' to '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/monitoring_attrs/nr_regions/min'
write '1000' to '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/monitoring_attrs/nr_regions/max'
write '34971' to '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/targets/0/pid_target'
write '0' to '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/schemes/0/access_pattern/sz/min'
write '18446744073709551615' to '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/schemes/0/access_pattern/sz/max'
write '0' to '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/schemes/0/access_pattern/nr_accesses/min'
write '20' to '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/schemes/0/access_pattern/nr_accesses/max'
write '0' to '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/schemes/0/access_pattern/age/min'
write '1' to '/sys/kernel/mm/damon/admin/kdamonds/nr_kdamonds'
read '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/nr_contexts': '0'
write '1' to '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/nr_contexts'
read '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/targets/nr_targets': '0'
write '1' to '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/targets/nr_targets'
read '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/targets/0/regions/nr_regions': '0'
read '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/schemes/nr_schemes': '0'
write '1' to '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/schemes/nr_schemes'
write 'vaddr' to '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/operations'
write '5000' to '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/monitoring_attrs/intervals/sample_us'
write '100000' to '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/monitoring_attrs/intervals/aggr_us'
write '1000000' to '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/monitoring_attrs/intervals/update_us'
write '10' to '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/monitoring_attrs/nr_regions/min'
write '1000' to '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/monitoring_attrs/nr_regions/max'
write '34971' to '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/targets/0/pid_target'
write '0' to '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/schemes/0/access_pattern/sz/min'
write '18446744073709551615' to '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/schemes/0/access_pattern/sz/max'
write '0' to '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/schemes/0/access_pattern/nr_accesses/min'
write '20' to '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/schemes/0/access_pattern/nr_accesses/max'
write '0' to '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/schemes/0/access_pattern/age/min'
write '184467440737095' to '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/schemes/0/access_pattern/age/max'
write 'stat' to '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/schemes/0/action'
write '0' to '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/schemes/0/quotas/ms'
write '0' to '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/schemes/0/quotas/bytes'
write '18446744073709551615' to '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/schemes/0/quotas/reset_interval_ms'
write '0' to '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/schemes/0/quotas/weights/sz_permil'
write '0' to '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/schemes/0/quotas/weights/nr_accesses_permil'
write '0' to '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/schemes/0/quotas/weights/age_permil'
write 'none' to '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/schemes/0/watermarks/metric'
write '0' to '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/schemes/0/watermarks/interval_us'
write '0' to '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/schemes/0/watermarks/high'
write '0' to '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/schemes/0/watermarks/mid'
write '0' to '/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/schemes/0/watermarks/low'
write 'on' to '/sys/kernel/mm/damon/admin/kdamonds/0/state'
*/
