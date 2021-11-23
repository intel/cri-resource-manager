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

package procstats

import (
	"io/ioutil"
	"strconv"
	"strings"
	"sync"

	"github.com/intel/cri-resource-manager/pkg/log"
	"github.com/intel/cri-resource-manager/pkg/sysfs"
)

// CPUTimeStat is used to calculate the CPU usage.
type CPUTimeStat struct {
	sync.RWMutex
	PrevIdleTime       []uint64
	PrevTotalTime      []uint64
	CurIdleTime        []uint64
	CurTotalTime       []uint64
	DeltaIdleTime      []uint64
	DeltaTotalTime     []uint64
	CPUUsage           []float64
	IsGetCPUUsageBegin bool
}

var (
	// procRoot is the mount point for the proc filesystem
	procRoot = "/proc"
	procStat = procRoot + "/stat"
)

// GetCPUTimeStat calculates CPU usage by using the CPU time statistics from /proc/stat
func (t *CPUTimeStat) GetCPUTimeStat() error {
	// /proc/stat looks like this:
	// cpuid: user, nice, system, idle, iowait, irq, softirq
	// cpu  130216 19944 162525 1491240 3784 24749 17773 0 0 0
	// cpu0 40321 11452 49784 403099 2615 6076 6748 0 0 0
	// cpu1 26585 2425 36639 151166 404 2533 3541 0 0 0
	// ...
	stats, err := ioutil.ReadFile(procStat)
	if err != nil {
		return err
	}
	t.Lock()
	defer t.Unlock()
	sys, err := sysfs.DiscoverSystem()
	if err != nil {
		return err
	}
	cpuCount := len(sys.CPUIDs())
	for index, line := range strings.Split(string(stats), "\n") {
		if index > cpuCount {
			break
		}
		split := strings.Split(line, " ")

		if strings.HasPrefix(split[0], "cpu") && split[0] != "cpu" {
			i, err := strconv.Atoi(split[0][3:])
			if err != nil {
				log.Error("Fail to get CPU index.")
				return err
			}
			t.CurIdleTime[i], err = strconv.ParseUint(split[4], 10, 64)
			if err != nil {
				log.Error("Fail to get idle time.")
				return err
			}
			totalTime := uint64(0)
			for _, s := range split[1:] {
				u, err := strconv.ParseUint(s, 10, 64)
				if err == nil {
					totalTime += u
				}
			}
			t.CurTotalTime[i] = totalTime
			t.CPUUsage[i] = 0.0
			if t.IsGetCPUUsageBegin {
				t.DeltaIdleTime[i] = t.CurIdleTime[i] - t.PrevIdleTime[i]
				t.DeltaTotalTime[i] = t.CurTotalTime[i] - t.PrevTotalTime[i]
				if t.DeltaTotalTime[i] != 0 {
					t.CPUUsage[i] = (1.0 - float64(t.DeltaIdleTime[i])/float64(t.DeltaTotalTime[i])) * 100.0
				}
			}
			t.PrevIdleTime[i] = t.CurIdleTime[i]
			t.PrevTotalTime[i] = t.CurTotalTime[i]
		}
	}
	for _, i := range sys.Offlined().ToSlice() {
		t.DeltaIdleTime[i] = 0.0
		t.DeltaTotalTime[i] = 0.0
		t.PrevIdleTime[i] = t.CurIdleTime[i]
		t.PrevTotalTime[i] = t.CurTotalTime[i]
		t.CPUUsage[i] = 0.0
	}
	t.IsGetCPUUsageBegin = true
	return nil
}
