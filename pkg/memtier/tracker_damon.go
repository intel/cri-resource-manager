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
}

const trackerDamonDefaults string = "{\"Connection\":\"perf\"}"

type TrackerDamon struct {
	mutex        sync.Mutex
	damonDir     string
	config       *TrackerDamonConfig
	pids         []int
	started      bool
	toPerfReader chan byte
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
		damonDir: "/sys/kernel/debug/damon",
		accesses: make(map[int]map[uint64]map[uint64]uint64),
		tidpid:   make(map[int64]int),
	}

	if !procFileExists(t.damonDir) {
		return nil, fmt.Errorf("no platform support: %q missing", t.damonDir)
	}

	if err := t.applyMonitor("off"); err != nil {
		return nil, err
	}
	return &t, nil
}

func (t *TrackerDamon) SetConfigJson(configJson string) error {
	config := &TrackerDamonConfig{}
	if err := unmarshal(configJson, config); err != nil {
		return err
	}
	if !strings.HasPrefix(config.Connection, "perf") {
		return fmt.Errorf("invalid damon connection %q, supported: \"perf [options]\"", config.Connection)
	}
	if config.SamplingUs == 0 {
		config.SamplingUs = 1000 // sampling interval, 1 ms
	}
	if config.AggregationUs == 0 {
		config.AggregationUs = 100000 // aggregation interval, 100 ms
	}
	if config.RegionsUpdateUs == 0 {
		config.RegionsUpdateUs = 5000000 // regions update interval, 5 s
	}
	if config.MinTargetRegions == 0 {
		config.MinTargetRegions = 10
	}
	if config.MaxTargetRegions == 0 {
		config.MaxTargetRegions = 1000
	}
	if err := t.applyAttrs(config); err != nil {
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
	log.Debugf("TrackerDamon: AddPids(%v)\n", pids)
	for _, pid := range pids {
		t.pids = append(t.pids, pid)
	}
	if t.started {
		t.applyMonitor("off")
		t.applyTargetIds()
		t.applyMonitor("on")
	}
}

func (t *TrackerDamon) RemovePids(pids []int) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	log.Debugf("TrackerDamon: RemovePids(%v)\n", pids)
	if pids == nil {
		t.pids = []int{}
		return
	}
	for _, pid := range pids {
		t.removePid(pid)
	}
	if t.started {
		t.applyMonitor("off")
		t.applyTargetIds()
		t.applyMonitor("on")
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

func (t *TrackerDamon) applyAttrs(config *TrackerDamonConfig) error {
	utoa := func(u uint64) string { return strconv.FormatUint(u, 10) }
	configStr := utoa(config.SamplingUs) +
		" " + utoa(config.AggregationUs) +
		" " + utoa(config.RegionsUpdateUs) +
		" " + utoa(config.MinTargetRegions) +
		" " + utoa(config.MaxTargetRegions) + "\n"
	if err := procWrite(t.damonDir+"/attrs", []byte(configStr)); err != nil {
		return fmt.Errorf("when writing %q: %w", configStr, err)
	}
	return nil
}

func (t *TrackerDamon) applyTargetIds() error {
	// Refresh all pids to be monitored.
	// Writing a non-existing pids to target_ids causes an error.
	pids := make([]string, 0, len(t.pids))
	for _, pid := range t.pids {
		pidStr := strconv.Itoa(pid)
		if procFileExists("/proc/" + pidStr) {
			pids = append(pids, pidStr)
		} else {
			t.removePid(pid)
		}
	}
	pidsStr := strings.Join(pids, " ")
	if err := procWrite(t.damonDir+"/target_ids", []byte(pidsStr)); err != nil {
		return err
	}
	return nil
}

func (t *TrackerDamon) applyMonitor(value string) error {
	monitorFilename := t.damonDir + "/monitor_on"
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

func (t *TrackerDamon) Start() error {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	// Reset configuration.
	if t.config == nil {
		if err := t.SetConfigJson(trackerDamonDefaults); err != nil {
			return fmt.Errorf("start failed on default configuration error: %w", err)
		}
	}

	t.applyMonitor("off")

	t.applyAttrs(t.config)

	t.applyTargetIds()

	// Even if damon start monitor fails, the tracker state is
	// "started" from this point on. That is, removing bad pids
	// and adding new pids will try restarting monitor.
	t.started = true

	if strings.HasPrefix(t.config.Connection, "perf") && t.toPerfReader == nil {
		t.toPerfReader = make(chan byte, 1)
		go t.perfReader()
	}

	// Start monitoring.
	if len(t.pids) > 0 {
		if err := t.applyMonitor("on"); err != nil {
			return err
		}
		log.Debugf("TrackerDamon.Start: monitoring is on")
	}
	return nil
}

func (t *TrackerDamon) Stop() {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	// Never mind about error: may cause "Operation not permitted"
	// if monitoring was already off.
	t.applyMonitor("off")
	t.started = false
	if t.toPerfReader != nil {
		t.toPerfReader <- 0
	}
}

func (t *TrackerDamon) ResetCounters() {
	// TODO: lock!? so that perfReader wouldn't need lock on every line?
	t.mutex.Lock()
	defer t.mutex.Unlock()
	if t.lostEvents > 0 {
		log.Debugf("TrackerDamon: ResetCounters: events lost %d\n", t.lostEvents)
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

func (t *TrackerDamon) perfReader() error {
	log.Debugf("TrackerDamon: online\n")
	defer log.Debugf("TrackerDamon: offline\n")
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
	log.Debugf("TrackerDamon: launching perf...\n")
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
				log.Debugf("TrackerDamon: perf parse error: %s\n", err)
			}
		case <-t.toPerfReader:
			close(t.toPerfReader)
			t.toPerfReader = nil
			if err := cmd.Process.Kill(); err != nil {
				log.Debugf("TrackerDamon: perf kill error: %s\n", err)
			}
			perfLines <- ""
			quit = true
		}
	}
	cmd.Wait()
	return nil
}

func (t *TrackerDamon) targetIdToPid(targetId int64, start uint64, end uint64, targetIdIsPidIndex bool) int {
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

func (t *TrackerDamon) perfHandleLine(line string) error {
	// Parse line. Example of "perf trace -e damon:damon_aggregated --libtraceevent_print" output lines, Linux 5.15, 5.16:
	//   0.000 kdamond.0/1527 damon:damon_aggregated(target_id=18446634001245894528 nr_regions=7 4194304-185102770176: 0)
	// LOST 123 events!
	// (The last three numbers on the first line being start_addr, end_addr and nr_accesses.)
	// Linux 5.17+:
	//   0.030 kdamond.0/262863 damon:damon_aggregated(target_id=0 nr_regions=202 824633720832-824700829696: 0 120)
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
	start, err := strconv.Atoi(startStr)
	if err != nil {
		stats.Store(StatsHeartbeat{"TrackerDamon.perfHandleLine: parse error: start address syntax error"})
		return fmt.Errorf("parse error (%w) on startStr %q line %q", err, startStr, line)
	}
	end, err := strconv.Atoi(endStr)
	if err != nil {
		stats.Store(StatsHeartbeat{"TrackerDamon.perfHandleLine: parse error: end address syntax error"})
		return fmt.Errorf("parse error (%w) on endStr %q line %q", err, endStr, line)
	}
	if start >= end {
		stats.Store(StatsHeartbeat{"TrackerDamon.perfHandleLine: parse error: start addr after end addr"})
		return fmt.Errorf("parse error: start >= end (%d >= %d) line %q", start, end, line)
	}
	nr := 0
	nr, err = strconv.Atoi(nrStr)
	if err != nil {
		stats.Store(StatsHeartbeat{"TrackerDamon.perfHandleLine: parse error: nr_access syntax error"})
		return fmt.Errorf("parse error (%w) on nrStr %q line %q", err, nrStr, line)
	}
	targetIdIsPidIndex := false
	if len(csLine) > 10 {
		// Linux 5.17+: target_id is an index in to the pids in the target_id's file.
		targetIdIsPidIndex = true
	}
	pid := t.targetIdToPid(int64(targetId), uint64(start), uint64(end), targetIdIsPidIndex)
	if pid < 1 {
		stats.Store(StatsHeartbeat{"TrackerDamon.perfHandleLine:unknown target id"})
		return nil
	}
	// TODO: avoid locking this often
	t.mutex.Lock()
	startLengthCount, ok := t.accesses[pid]
	if !ok {
		startLengthCount = make(map[uint64]map[uint64]uint64)
		t.accesses[pid] = startLengthCount
	}
	lengthPgs := uint64(int64(end-start) / constPagesize)
	lengthCount, ok := startLengthCount[uint64(start)]
	if !ok {
		lengthCount = make(map[uint64]uint64)
		startLengthCount[uint64(start)] = lengthCount
	}
	if count, ok := lengthCount[lengthPgs]; ok {
		lengthCount[lengthPgs] = count + uint64(nr)
	} else {
		lengthCount[lengthPgs] = uint64(nr)
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
				a: uint64(nr),
			},
		}
		t.raes.store(rae)
	}
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
