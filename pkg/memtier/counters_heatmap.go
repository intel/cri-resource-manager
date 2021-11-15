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

// Counters
// round0: [--addrs-0--]  [addrs-1]
// flat:   [           ][][       ]
//         ^            ^ ^
//          \-----------+-+---------addrs-0 only
//                       \+---------noinfo
//                         \--------addrs-1 only

// round1:    [---addrs-2----]
// flat:   [ ][        ][][ ][    ]
//         ^  ^         ^ ^  ^
//          \-+---------+-+--+------addrs-0 from round 0
//             \--------+-----------addrs-0 round0 + addrs-2 round1
//                       \----------addrs-2 round1

// round2: [addrs-3][---addrs-4------]
// flat:   [  ][   ][  ][][  ][      ]

package memtier

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

type HeatmapConfig struct {
	// HeatMax is the maximum heat of a range
	HeatMax float64
	// HeatDelta is the portion of the remaining heat in a region
	// after one second of inactivity.
	// - 1.0: heat never cools down
	// - 0.0: all heat cools down immediately
	// - If you want that 5 % of the heat remains after 60 seconds of inactivity,
	//   HeatDelta = 0.05 ** (1.0/60) = 0.9513
	HeatDelta float64
}

var HeatmapConfigDefaults string = `{"HeatMax":1.0,"HeatDelta": 0.9513}`

type Heatmap struct {
	config *HeatmapConfig
	mutex  sync.Mutex
	// pidHeatRanges contains heats seen for each range.
	// - Array of HeatRanges is sorted by addr.
	// - Address ranges never overlap. That is,
	//   hr[i].addr + hr[i].length*PAGESIZE <= hr[i+1].addr
	pidHrs map[int]*HeatRanges
}

type HeatRange struct {
	addr   uint64
	length uint64  // number of pages
	heat   float64 // heat per page
	seen   int64
}

type HeatRanges []*HeatRange

func NewCounterHeatmap() *Heatmap {
	heatmap := &Heatmap{
		pidHrs: make(map[int]*HeatRanges),
	}
	if err := heatmap.SetConfigJson(HeatmapConfigDefaults); err != nil {
		panic(fmt.Sprintf("heatmap default configuration error: %s", err))
	}
	return heatmap
}

func (h *Heatmap) SetConfigJson(configJson string) error {
	config := HeatmapConfig{}
	if err := json.Unmarshal([]byte(configJson), &config); err != nil {
		return err
	}
	h.config = &config
	return nil
}

// Dump presents heatmap as a string that indicate heat of each range
func (h *Heatmap) Dump() string {
	lines := []string{}
	pids := make([]int, 0, len(h.pidHrs))
	for pid := range h.pidHrs {
		pids = append(pids, pid)
	}
	sort.Ints(pids)
	for _, pid := range pids {
		for _, hr := range *(h.pidHrs[pid]) {
			lines = append(lines, fmt.Sprintf("pid: %d: %s", pid, hr))
		}
	}
	return strings.Join(lines, "\n")
}

func (hr *HeatRange) String() string {
	return fmt.Sprintf("{%x-%x,heat:%f,seen:%d}",
		hr.addr,
		hr.addr+hr.length*uint64(constPagesize),
		hr.heat,
		hr.seen)
}

func (h *Heatmap) UpdateFromCounters(tcs *TrackerCounters, timestamp int64) {
	if h.pidHrs == nil {
		panic("Heatmap data structure missing, not instantiated with NewCounterHeatmap")
	}
	h.mutex.Lock()
	defer h.mutex.Unlock()
	for _, tc := range *tcs {
		h.updateFromCounter(&tc, timestamp)
	}
}

func (h *Heatmap) updateFromCounter(tc *TrackerCounter, timestamp int64) {
	pid := tc.AR.Pid()
	for _, ar := range tc.AR.Ranges() {
		length := ar.Length()
		thr := HeatRange{
			addr:   ar.Addr(),
			length: length,
			heat:   float64(tc.Accesses+tc.Reads+tc.Writes) / float64(length),
			seen:   timestamp,
		}
		if thr.heat > h.config.HeatMax {
			thr.heat = h.config.HeatMax
		}
		h.updateFromPidHeatRange(pid, &thr)
	}
}

func (h *Heatmap) updateFromPidHeatRange(pid int, thr *HeatRange) {
	hrs, ok := h.pidHrs[pid]
	if !ok {
		hrs = &HeatRanges{}
		h.pidHrs[pid] = hrs
	}
	overlappingRanges := hrs.Overlapping(thr)
	for _, hr := range *overlappingRanges {
		if hr.addr < thr.addr {
			// Case:
			// |-------hr-------...
			//         |---thr--...
			// Add newHr at hr start address, and move hr forward
			// |-newhr-|---hr---...
			//         |---thr--...
			newHr := HeatRange{
				addr:   hr.addr,
				length: (thr.addr - hr.addr) / uint64(constPagesize),
				heat:   hr.heat,
				seen:   hr.seen,
			}
			*hrs = append(*hrs, &newHr)
			hr.addr = thr.addr
			hr.length -= newHr.length
		}
		if thr.addr < hr.addr {
			// Case:
			//         |----hr--...
			// |----------thr---...
			// Add newHr at thr start address, move thr forward
			// |-newhr-|---hr---...
			//         |---thr--...
			newHr := HeatRange{
				addr:   thr.addr,
				length: (hr.addr - thr.addr) / uint64(constPagesize),
				heat:   thr.heat,
				seen:   thr.seen,
			}
			*hrs = append(*hrs, &newHr)
			thr.addr = hr.addr
			thr.length -= newHr.length
		}
		// now thr.addr == hr.addr
		hrEndAddr := hr.addr + hr.length*uint64(constPagesize)
		thrEndAddr := thr.addr + thr.length*uint64(constPagesize)
		endAddr := hrEndAddr
		if endAddr > thrEndAddr {
			endAddr = thrEndAddr
		}
		if thrEndAddr < hrEndAddr {
			// Case:
			// |--------hr-------|
			// |---thr---|
			// Add newHr at thr end address, cut hr length
			// |---hr----|-newhr-|
			// |---thr---|
			newHr := HeatRange{
				addr:   thrEndAddr,
				length: (hrEndAddr - thrEndAddr) / uint64(constPagesize),
				heat:   hr.heat,
				seen:   hr.seen,
			}
			*hrs = append(*hrs, &newHr)
			hr.length -= newHr.length
			hrEndAddr = thrEndAddr
		}
		// update hr heat
		seconds := float64(thr.seen-hr.seen) / float64(time.Second)
		if seconds < 0.0 {
			// There is something wrong with the
			// timestamps: new heatrange looks older than
			// existing hr. Prevent a jump in heat that
			// this might cause when calculating "cool
			// down".
			seconds = 0.0
		}
		hr.heat = thr.heat + hr.heat*math.Pow(h.config.HeatDelta, seconds)
		if hr.heat > h.config.HeatMax {
			hr.heat = h.config.HeatMax
		}
		hr.seen = thr.seen
		// now we have handled thr up to hr's end addr,
		// |----hr----|
		// |----thr-----------|
		// move remaining thr forward
		//            |--thr--|
		thr.length -= hr.length
		thr.addr = hrEndAddr
	}
	// Case: there is still a remaining, non-overlapping part of thr
	// --last-overlapping-hr---|
	//                         |---thr---|
	if thr.length > 0 {
		newHr := HeatRange{
			addr:   thr.addr,
			length: thr.length,
			heat:   thr.heat,
			seen:   thr.seen,
		}
		*hrs = append(*hrs, &newHr)
	}
	hrs.Sort()
}

func (hrs *HeatRanges) Sort() {
	sort.Slice(*hrs, func(i, j int) bool {
		return (*hrs)[i].addr < (*hrs)[j].addr
	})
}

func (hrs *HeatRanges) Overlapping(hr0 *HeatRange) *HeatRanges {
	// Optimize: bisect would be faster way to find the first overlapping hr
	first := 0
	for _, hr := range *hrs {
		if hr.addr+hr.length*uint64(constPagesize) > hr0.addr {
			break
		}
		first++
	}
	hr0EndAddr := hr0.addr + hr0.length*uint64(constPagesize)
	count := 0
	for _, hr := range (*hrs)[first:] {
		if hr0EndAddr <= hr.addr {
			break
		}
		count += 1
	}
	subHeatRanges := (*hrs)[first : first+count]
	return &subHeatRanges
}

// GetHeat returns the heat of a region
func (h *Heatmap) HeatRangeAt(pid int, addr uint64) *HeatRange {
	hrs, ok := h.pidHrs[pid]
	if !ok {
		return nil
	}
	hr := HeatRange{
		addr:   addr,
		length: 1,
	}
	overlapping := hrs.Overlapping(&hr)
	if len(*overlapping) != 1 {
		return nil
	}
	return (*overlapping)[0]
}
