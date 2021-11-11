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
	Connection       string
	SamplingUs       uint64 // interval in microseconds
	AggregationUs    uint64 // interval in microseconds
	RegionsUpdateUs  uint64 // interval in microseconds
	MinTargetRegions uint64
	MaxTargetRegions uint64
}

const trackerDamonDefaults string = "{\"Connection\":\"perf\"}"

type TrackerDamon struct {
	mutex             sync.Mutex
	damonDir          string
	config            *TrackerDamonConfig
	pids              []int
	perfReaderRunning bool
	// accesses maps startAddr -> lengthPgs -> accessCount
	accesses map[uint64]map[uint64]uint64
}

func init() {
	TrackerRegister("damon", NewTrackerDamon)
}

func NewTrackerDamon() (Tracker, error) {
	t := TrackerDamon{
		damonDir: "/sys/kernel/debug/damon",
		accesses: make(map[uint64]map[uint64]uint64),
	}

	if !procFileExists(t.damonDir) {
		return nil, fmt.Errorf("no platform support: %q missing", t.damonDir)
	}
	t.Stop()
	return &t, nil
}

func (t *TrackerDamon) SetConfigJson(configJson string) error {
	config := TrackerDamonConfig{}
	if err := json.Unmarshal([]byte(configJson), &config); err != nil {
		return err
	}
	if config.Connection != "perf" {
		return fmt.Errorf("invalid damon connection %q, supported: perf", config.Connection)
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
	if err := t.applyAttrs(&config); err != nil {
		return err
	}
	t.config = &config
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
	for _, pid := range pids {
		t.pids = append(t.pids, pid)
	}
}

func (t *TrackerDamon) RemovePids(pids []int) {
	if pids == nil {
		t.pids = []int{}
		return
	}
	for _, pid := range pids {
		t.removePid(pid)
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

func (t *TrackerDamon) Start() error {
	// Reset configuration.
	if t.config == nil {
		if err := t.SetConfigJson(trackerDamonDefaults); err != nil {
			return fmt.Errorf("start failed on default configuration error: %w", err)
		}
	}
	t.applyAttrs(t.config)

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
	}

	if t.config.Connection == "perf" && !t.perfReaderRunning {
		t.perfReaderRunning = true
		go t.perfReader()
	}

	// Start monitoring.
	if len(pids) > 0 {
		if err := procWrite(t.damonDir+"/monitor_on", []byte("on")); err != nil {
		}
	}
	return nil
}

func (t *TrackerDamon) Stop() {
	// Never mind about error: may cause "Operation not permitted"
	// if monitoring was already off.
	procWrite(t.damonDir+"/monitor_on", []byte("off"))
}

func (t *TrackerDamon) ResetCounters() {
	// TODO: lock!? so that perfReader wouldn't need lock on every line?
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.accesses = make(map[uint64]map[uint64]uint64)
}

func (t *TrackerDamon) GetCounters() *TrackerCounters {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	tcs := &TrackerCounters{}
	for start, lengthCount := range t.accesses {
		for length, count := range lengthCount {
			addrRange := AddrRanges{
				pid: 0, // FIXME: this is bad
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
	return tcs
}

func (t *TrackerDamon) perfReader() error {
	cmd := exec.Command("perf", "trace", "-e", "damon:damon_aggregated")
	errPipe, err := cmd.StderrPipe()
	perfOutput := bufio.NewReader(errPipe)
	if err != nil {
		return fmt.Errorf("creating stderr pipe for perf failed: %w", err)
	}
	if err = cmd.Start(); err != nil {
		return fmt.Errorf("starting perf failed: %w", err)
	}
	for {
		line, err := perfOutput.ReadString('\n')
		if err != nil || line == "" {
			break
		}
		t.perfHandleLine(line)
	}
	cmd.Wait()
	fmt.Printf("perfReader quitting\n")
	return nil
}

func (t *TrackerDamon) perfHandleLine(line string) error {
	// Parse line. Example of perf damon:damon_aggregated output line:
	// 12400.049 kdamond.0/30572 damon:damon_aggregated(target_id: -121171723722880, nr_regions: 13, start: 139995295608832, end: 139995297484800, nr_accesses: 1)

	// TODO: how to convert target_id to pid?
	csLine := strings.Split(line, ", ")
	if len(csLine) < 4 {
		return fmt.Errorf("invalid bad line %q", csLine)
	}
	startStr := csLine[2][7:]
	endStr := csLine[3][5:]
	nrStr := ""
	if len(csLine) == 5 {
		nrStr = csLine[4][13 : len(csLine[4])-2]
	}
	start, err := strconv.Atoi(startStr)
	if err != nil {
		return fmt.Errorf("parse error on startStr %q element %q line %q\n", startStr, csLine[2], line)
	}
	end, err := strconv.Atoi(endStr)
	if err != nil {
		return fmt.Errorf("parse error on endStr %q element %q line %q\n", endStr, csLine[3], line)
	}
	nr := 0
	if len(nrStr) > 0 {
		nr, err = strconv.Atoi(nrStr)
		if err != nil {
			return fmt.Errorf("parse error on nrStr %q element %q line %q\n", nrStr, csLine[4], line)
		}
	}
	// ?? How to avoid locking this often
	t.mutex.Lock()
	defer t.mutex.Unlock()
	lengthPgs := uint64(int64(end-start) / constPagesize)
	lengthCount, ok := t.accesses[uint64(start)]
	if !ok {
		lengthCount = make(map[uint64]uint64)
		t.accesses[uint64(start)] = lengthCount
	}
	if count, ok := lengthCount[lengthPgs]; ok {
		lengthCount[lengthPgs] = count + uint64(nr)
	} else {
		lengthCount[lengthPgs] = uint64(nr)
	}
	return nil
}
