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
	accesses   map[int64]map[uint64]map[uint64]uint64
	lostEvents uint
}

func init() {
	TrackerRegister("damon", NewTrackerDamon)
}

func NewTrackerDamon() (Tracker, error) {
	t := TrackerDamon{
		damonDir: "/sys/kernel/debug/damon",
		accesses: make(map[int64]map[uint64]map[uint64]uint64),
	}

	if !procFileExists(t.damonDir) {
		return nil, fmt.Errorf("no platform support: %q missing", t.damonDir)
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
	if err := procWrite(t.damonDir+"/monitor_on", []byte(value)); err != nil {
		return err
	}
	status, err := procRead(t.damonDir + "/monitor_on")
	if err != nil {
		return err
	}
	if status[:2] == value[:2] {
		log.Debugf("TrackerDamon.Start: monitoring is %q\n", value)
	} else {
		return fmt.Errorf("wrote %q %s/monitor_on, but value is %q", value, t.damonDir, status)
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
	t.accesses = make(map[int64]map[uint64]map[uint64]uint64)
	t.lostEvents = 0
}

func (t *TrackerDamon) GetCounters() *TrackerCounters {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	tcs := &TrackerCounters{}
	for targetId, startLengthCount := range t.accesses {
		pid := 0
		if len(t.pids) > 0 {
			// TODO: a proper targetId-to-pid detection.
			// pid should be based on targetId.
			var _ = targetId // unused, but
			pid = t.pids[0]
		}

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
	perfTraceArgs := []string{"trace", "-e", "damon:damon_aggregated"}
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

func (t *TrackerDamon) perfHandleLine(line string) error {
	// Parse line. Example of perf damon:damon_aggregated output lines:
	// 12400.049 kdamond.0/30572 damon:damon_aggregated(target_id: -121171723722880, nr_regions: 13, start: 139995295608832, end: 139995297484800, nr_accesses: 1)
	// 12400.049 kdamond.0/30572 damon:damon_aggregated(target_id: -121171723722880, nr_regions: 13, start: 139995295608832, end: 139995297484800)
	// LOST 123 events!
	if strings.HasPrefix(line, "LOST ") {
		lostEventsStr := strings.Split(line, " ")[1]
		lostEvents, err := strconv.ParseUint(lostEventsStr, 10, 0)
		if err != nil {
			return fmt.Errorf("parse error on lost event count %q line: %s", lostEventsStr, line)
		}
		t.lostEvents += uint(lostEvents)
		return nil
	}
	csLine := strings.Split(line, ", ")
	if len(csLine) < 4 {
		return fmt.Errorf("bad line %q", csLine)
	}
	targetIdStrSlice := strings.SplitAfterN(csLine[0], "target_id: ", 2)
	if len(targetIdStrSlice) != 2 {
		return fmt.Errorf("target_id not found from %q", csLine[0])
	}
	targetIdStr := targetIdStrSlice[1]
	targetId, err := strconv.ParseInt(targetIdStr, 10, 64)
	if err != nil {
		return fmt.Errorf("parse error on targetIdStr %q line %q", targetIdStr, line)
	}
	startStr := csLine[2][7:]
	endStr := csLine[3][5:]
	nrStr := ""
	// strip ")\n" from the end of nrStr or endStr
	if len(csLine) == 5 {
		nrStr = csLine[4][13 : len(csLine[4])-2]
	} else {
		endStr = endStr[:len(endStr)-2]
	}
	start, err := strconv.Atoi(startStr)
	if err != nil {
		return fmt.Errorf("parse error on startStr %q element %q line %q", startStr, csLine[2], line)
	}
	end, err := strconv.Atoi(endStr)
	if err != nil {
		return fmt.Errorf("parse error on endStr %q element %q line %q", endStr, csLine[3], line)
	}
	if start >= end {
		return fmt.Errorf("parse error: start >= end (%d >= %d) line %q", start, end, line)
	}
	nr := 0
	if len(nrStr) > 0 {
		nr, err = strconv.Atoi(nrStr)
		if err != nil {
			return fmt.Errorf("parse error on nrStr %q element %q line %q", nrStr, csLine[4], line)
		}
	}
	// TODO: avoid locking this often
	t.mutex.Lock()
	defer t.mutex.Unlock()
	startLengthCount, ok := t.accesses[targetId]
	if !ok {
		startLengthCount = make(map[uint64]map[uint64]uint64)
		t.accesses[targetId] = startLengthCount
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
	return nil
}
