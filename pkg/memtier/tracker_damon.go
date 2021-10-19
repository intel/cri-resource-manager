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
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type TrackerDamon struct {
	damonDir string
	config   *TrackerDamonConfig
	regions  map[int][]*AddrRanges
}

type TrackerDamonConfig struct {
	Connection       string
	SamplingUs       uint64 // interval in microseconds
	AggregationUs    uint64 // interval in microseconds
	RegionsUpdateUs  uint64 // interval in microseconds
	MinTargetRegions uint64
	MaxTargetRegions uint64
}

func init() {
	TrackerRegister("damon", NewTrackerDamon)
}

func NewTrackerDamon() (Tracker, error) {
	t := TrackerDamon{
		damonDir: "/sys/kernel/debug/damon",
		regions:  make(map[int][]*AddrRanges, 0),
	}

	if !procFileExists(t.damonDir) {
		return nil, fmt.Errorf("no platform support: %q missing", t.damonDir)
	}
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

func (t *TrackerDamon) AddRanges(ar *AddrRanges) {
	pid := ar.Pid()
	if regions, ok := t.regions[pid]; ok {
		t.regions[pid] = append(regions, ar)
	} else {
		t.regions[pid] = []*AddrRanges{ar}
	}
}

func (t *TrackerDamon) RemovePid(pid int) {
	delete(t.regions, pid)
}

func (t *TrackerDamon) Stop() {
	// Never mind about error: may cause "Operation not permitted"
	// if monitoring was already off.
	procWrite(t.damonDir+"/monitor_on", []byte("off"))
}

func (t *TrackerDamon) ResetCounters() {
	// Stop monitoring.
	t.Stop()

	// Reset configuration.
	t.applyAttrs(t.config)

	// Refresh all pids to be monitored.
	pids := make([]string, 0, len(t.regions))
	for pid := range t.regions {
		pidStr := strconv.Itoa(pid)
		if procFileExists("/proc/" + pidStr) {
			pids = append(pids, pidStr)
		} else {
			t.RemovePid(pid)
		}
	}
	pidsStr := strings.Join(pids, " ")
	procWrite(t.damonDir+"/target_ids", []byte(pidsStr))

	// Todo: reset connection (now restart perf, possibly delete file).

	// Start monitoring.
	if len(pids) > 0 {
		if err := procWrite(t.damonDir+"/monitor_on", []byte("on")); err != nil {
		}
	}
}

func (t *TrackerDamon) GetCounters() *TrackerCounters {
	return nil
}
