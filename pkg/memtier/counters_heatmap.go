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
	// HeatRetention is the portion of the remaining heat in a region
	// after one second of complete inactivity.
	// - 1.0: heat never cools down
	// - 0.0: all heat cools down immediately
	// - If you want that 5 % of the heat remains after 60 seconds of inactivity,
	//   HeatRetention = 0.05 ** (1.0/60) = 0.9513
	HeatRetention float64
	// HeatClasses is the number of discrete heat classes. The default is 10,
	// which means that address ranges are classified:
	// heat class 0: heat [HeatMax*0/10, HeatMax*1/10)
	// heat class 1: heat [HeatMax*1/10, HeatMax*2/10)
	// ...
	// heat class 9: heat [HeatMax*9/10, HeatMax*10/10]
	HeatClasses int
}

var HeatmapConfigDefaults string = `{"HeatMax":1.0,"HeatRetention":0.9513,"HeatClasses":10}`

type Heatmap struct {
	config *HeatmapConfig
	mutex  sync.Mutex
	// pidHrs (pid-heatranges map) contains heats seen for each range.
	// - Array of HeatRanges is sorted by addr.
	// - Address ranges never overlap. That is,
	//   hr[i].addr + hr[i].length*PAGESIZE <= hr[i+1].addr
	pidHrs Heats
}

type Heats map[int]*HeatRanges

type HeatRanges []*HeatRange

type HeatRange struct {
	addr    uint64
	length  uint64  // number of pages
	heat    float64 // heat per page
	created int64
	updated int64
}

func (hr *HeatRange) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("{\"addr\":%d,\"length\":%d,\"heat\":%f,\"created\":%d,\"updated\":%d}", hr.addr, hr.length, hr.heat, hr.created, hr.updated)), nil
}

func (hr *HeatRange) UnmarshalJSON(raw []byte) error {
	hrJson := struct {
		Addr    uint64
		Length  uint64
		Heat    float64
		Created int64
		Updated int64
	}{}
	if err := json.Unmarshal(raw, &hrJson); err != nil {
		return fmt.Errorf("failed to unmarshal heats from %v: %w", raw, err)
	}
	hr.addr = hrJson.Addr
	hr.length = hrJson.Length
	hr.heat = hrJson.Heat
	hr.created = hrJson.Created
	hr.updated = hrJson.Updated
	return nil
}

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
	config := &HeatmapConfig{}
	if err := json.Unmarshal([]byte(configJson), config); err != nil {
		return err
	}
	return h.SetConfig(config)
}

func (h *Heatmap) SetConfig(config *HeatmapConfig) error {
	h.config = config
	return nil
}

func (h *Heatmap) Pids() []int {
	pids := make([]int, 0, len(h.pidHrs))
	for pid := range h.pidHrs {
		pids = append(pids, pid)
	}
	return pids
}

// Dump presents heatmap as a string that indicate heat of each range
func (h *Heatmap) Dump() string {
	lines := []string{}
	pids := h.Pids()
	sort.Ints(pids)
	for _, pid := range pids {
		for n, hr := range *(h.pidHrs[pid]) {
			lines = append(lines, fmt.Sprintf("pid: %d: %d %s (class %d)", pid, n, hr, h.HeatClass(hr)))
		}
	}
	return strings.Join(lines, "\n")
}

func (hr *HeatRange) String() string {
	return fmt.Sprintf("{%x-%x(%d),%f,%d,%.6fs}",
		hr.addr,
		hr.addr+hr.length*constUPagesize,
		hr.length,
		hr.heat,
		hr.created,
		float64(hr.updated-hr.created)/float64(time.Second))
}

func (hr *HeatRange) AddrRange() AddrRange {
	return AddrRange{hr.addr, hr.length}
}

func (h *Heatmap) HeatClass(hr *HeatRange) int {
	if h.config.HeatMax == 0 {
		return 0
	}
	heatClass := int(float64(h.config.HeatClasses) * hr.heat / h.config.HeatMax)
	if heatClass >= h.config.HeatClasses {
		heatClass = h.config.HeatClasses - 1
	}
	return heatClass
}

func (h *Heatmap) UpdateFromCounters(tcs *TrackerCounters, timestamp int64) {
	if h.pidHrs == nil {
		panic("Heatmap data structure missing, not instantiated with NewCounterHeatmap")
	}
	h.mutex.Lock()
	defer h.mutex.Unlock()
	trackedPids := map[int]setMemberType{}
	for _, tc := range *tcs {
		trackedPids[tc.AR.Pid()] = setMember
		h.updateFromCounter(&tc, timestamp)
	}
	for pid, _ := range h.pidHrs {
		if _, ok := trackedPids[pid]; !ok {
			delete(h.pidHrs, pid)
		}
	}
}

func (h *Heatmap) updateFromCounter(tc *TrackerCounter, timestamp int64) {
	pid := tc.AR.Pid()
	for _, ar := range tc.AR.Ranges() {
		length := ar.Length()
		thr := HeatRange{
			addr:    ar.Addr(),
			length:  length,
			heat:    float64(tc.Accesses+tc.Reads+tc.Writes) / float64(length),
			created: timestamp,
			updated: timestamp,
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
				addr:    hr.addr,
				length:  (thr.addr - hr.addr) / constUPagesize,
				heat:    hr.heat,
				created: hr.created,
				updated: hr.updated,
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
				addr:    thr.addr,
				length:  (hr.addr - thr.addr) / constUPagesize,
				heat:    thr.heat,
				created: thr.created,
				updated: thr.updated,
			}
			*hrs = append(*hrs, &newHr)
			thr.addr = hr.addr
			thr.length -= newHr.length
		}
		// now thr.addr == hr.addr
		hrEndAddr := hr.addr + hr.length*constUPagesize
		thrEndAddr := thr.addr + thr.length*constUPagesize
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
				addr:    thrEndAddr,
				length:  (hrEndAddr - thrEndAddr) / constUPagesize,
				heat:    hr.heat,
				created: hr.created,
				updated: hr.updated,
			}
			*hrs = append(*hrs, &newHr)
			hr.length -= newHr.length
			hrEndAddr = thrEndAddr
		}
		// update hr heat
		seconds := float64(thr.updated-hr.updated) / float64(time.Second)
		if seconds < 0.0 {
			// There is something wrong with the
			// timestamps: new heatrange looks older than
			// existing hr. Prevent a jump in heat that
			// this might cause when calculating "cool
			// down".
			seconds = 0.0
		}
		hr.heat = thr.heat + hr.heat*math.Pow(h.config.HeatRetention, seconds)
		if hr.heat > h.config.HeatMax {
			hr.heat = h.config.HeatMax
		}
		hr.updated = thr.updated
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
			addr:    thr.addr,
			length:  thr.length,
			heat:    thr.heat,
			created: thr.created,
			updated: thr.updated,
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
	first := sort.Search(len(*hrs), func(i int) bool { return (*hrs)[i].addr+(*hrs)[i].length*constUPagesize > hr0.addr })
	hr0EndAddr := hr0.addr + hr0.length*constUPagesize
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

// HeatRangeAt returns a HeatRange at address
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

// ForEachRange iterates over heatranges of a pid in ascending address order.
//   - handleRange(*HeatRange) is called for every range.
//     0 (continue): ForEachRange continues iteration from the next range
//     -1 (break):   ForEachRange returns immediately.
func (h *Heatmap) ForEachRange(pid int, handleRange func(*HeatRange) int) {
	hrs, ok := h.pidHrs[pid]
	if !ok {
		return
	}
	for _, hr := range *hrs {
		next := handleRange(hr)
		switch next {
		case 0:
			continue
		case -1:
			return
		default:
			panic(fmt.Sprintf("illegal heat range handler return value %d", next))
		}
	}
}

func (h *Heatmap) Sorted(pid int, cmp func(*HeatRange, *HeatRange) bool) HeatRanges {
	hrs, ok := h.pidHrs[pid]
	if !ok || len(*hrs) == 0 {
		return HeatRanges{}
	}
	retval := make(HeatRanges, len(*hrs))
	copy(retval, *hrs)
	sort.Slice(retval, func(hri0, hri1 int) bool {
		return cmp(retval[hri0], retval[hri1])
	})
	return retval
}
