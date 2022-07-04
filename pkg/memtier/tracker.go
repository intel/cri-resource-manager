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
	"fmt"
	"sort"
	"strings"
	"sync"
)

type TrackerConfig struct {
	Name   string
	Config string
}

type TrackerCounters []TrackerCounter

type TrackerCounter struct {
	Accesses uint64
	Reads    uint64
	Writes   uint64
	AR       *AddrRanges
}

type RangeHeat struct {
	Pid   int
	Range AddrRange
	Heat  uint64
}

type Tracker interface {
	SetConfigJson(string) error // Set new configuration.
	GetConfigJson() string      // Get current configuration.
	AddPids([]int)              // Add pids to be tracked.
	RemovePids([]int)           // Remove pids, RemovePids(nil) clears all.
	Start() error               // Start tracking.
	Stop()                      // Stop tracking.
	Dump([]string) string       // Peek at tracker internals.
	ResetCounters()
	GetCounters() *TrackerCounters
}

var raeBufSize int = 4096

type rawAccessEntry struct {
	timestamp int64
	pid       int
	addr      uint64
	length    uint64
	accessCounter
}

type rawAccessEntries struct {
	mutex sync.Mutex
	data  []*rawAccessEntry
}

type TrackerCreator func() (Tracker, error)

// trackers is a map of tracker name -> tracker creator
var trackers map[string]TrackerCreator = make(map[string]TrackerCreator, 0)

func TrackerRegister(name string, creator TrackerCreator) {
	trackers[name] = creator
}

func TrackerList() []string {
	keys := make([]string, 0, len(trackers))
	for key := range trackers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func NewTracker(name string) (Tracker, error) {
	if creator, ok := trackers[name]; ok {
		return creator()
	}
	return nil, fmt.Errorf("invalid tracker name %q", name)
}

func (raes *rawAccessEntries) dump(args []string) string {
	if len(args) == 0 {
		return "Usage: dump raw <start|new|stop>"
	}
	if raes.data != nil {
		raes.mutex.Lock()
		defer raes.mutex.Unlock()
	}
	switch args[0] {
	case "start":
		raes.data = make([]*rawAccessEntry, 0, raeBufSize)
		return "raw access events recording started"
	case "stop":
		raes.data = nil
		return "raw access events recording stopped"
	case "new":
		if raes.data == nil {
			return "error: not recording"
		}
		raeData := raes.data
		raes.data = make([]*rawAccessEntry, 0, raeBufSize)
		return raeDataToString(raeData)
	default:
		return fmt.Sprintf("invalid raw parameter: %q", args[0])
	}
}

func (raes *rawAccessEntries) store(rae *rawAccessEntry) {
	if raes.data == nil {
		return
	}
	raes.mutex.Lock()
	defer raes.mutex.Unlock()
	raes.data = append(raes.data, rae)
}

func raeDataToString(raeData []*rawAccessEntry) string {
	lines := make([]string, len(raeData)+1)
	lines[0] = "          timestamp       pid             addr    pages     accs   writes"
	lineFmt := "%20d %8d %16x %8d %8d %8d"
	for raeIndex, rae := range raeData {
		lines[raeIndex+1] = fmt.Sprintf(lineFmt, rae.timestamp, rae.pid, rae.addr, rae.length, rae.a, rae.w)
	}
	return strings.Join(lines, "\n")
}

func (tcs *TrackerCounters) SortByAccesses() {
	sort.Slice(*tcs, func(i, j int) bool {
		return (*tcs)[i].Accesses < (*tcs)[j].Accesses ||
			((*tcs)[i].Accesses == (*tcs)[j].Accesses && (*tcs)[i].Writes < (*tcs)[j].Writes) ||
			((*tcs)[i].Accesses == (*tcs)[j].Accesses && (*tcs)[i].Writes < (*tcs)[j].Writes && (*tcs)[i].AR.Ranges()[0].Addr() < (*tcs)[j].AR.Ranges()[0].Addr())
	})
}

func (tcs *TrackerCounters) SortByAddr() {
	sort.Slice(*tcs, func(i, j int) bool {
		return (*tcs)[i].AR.Pid() < (*tcs)[j].AR.Pid() ||
			(*tcs)[i].AR.Pid() == (*tcs)[j].AR.Pid() && (*tcs)[i].AR.Ranges()[0].Addr() < (*tcs)[j].AR.Ranges()[0].Addr() ||
			(*tcs)[i].AR.Pid() == (*tcs)[j].AR.Pid() && (*tcs)[i].AR.Ranges()[0].Addr() == (*tcs)[j].AR.Ranges()[0].Addr() && (*tcs)[i].AR.Ranges()[0].Length() < (*tcs)[j].AR.Ranges()[0].Length()
	})
}

func (tcs *TrackerCounters) String() string {
	lines := make([]string, 0, len(*tcs))
	for _, tc := range *tcs {
		lines = append(lines, fmt.Sprintf("a=%d r=%d w=%d %s",
			tc.Accesses, tc.Reads, tc.Writes, tc.AR))
	}
	return strings.Join(lines, "\n")
}

func flattenDefaultCut(tc0 TrackerCounter, ar *AddrRange) TrackerCounter {
	startAddr := tc0.AR.Ranges()[0].Addr()
	if ar.Addr() > startAddr {
		startAddr = ar.Addr()
	}
	endAddr := tc0.AR.Ranges()[0].EndAddr()
	if ar.EndAddr() < endAddr {
		endAddr = ar.EndAddr()
	}
	return TrackerCounter{
		Accesses: tc0.Accesses,
		Reads:    tc0.Reads,
		Writes:   tc0.Writes,
		AR:       NewAddrRanges(tc0.AR.Pid(), *NewAddrRange(startAddr, endAddr)),
	}
}

func flattenDefaultUnion(tc0, tc1 TrackerCounter) TrackerCounter {
	return TrackerCounter{
		Accesses: tc0.Accesses + tc1.Accesses,
		Reads:    tc0.Reads + tc1.Reads,
		Writes:   tc0.Writes + tc1.Writes,
		AR:       NewAddrRanges(tc0.AR.Pid(), tc0.AR.Ranges()...),
	}
}

// Flattened returns tracker counters with overlapping parts squashed.
// Parameters:
//   cut - returns new tc that is a cut from the address range part of tc0.
//   union - returns new tc that represents union of tc0 and tc1.
func (tcs *TrackerCounters) Flattened(cut func(tc0 TrackerCounter, ar *AddrRange) TrackerCounter, union func(tc0, tc1 TrackerCounter) TrackerCounter) *TrackerCounters {
	tcs.SortByAddr()
	// Invariants:
	// - flatTcs is sorted by pid, then by start address, then by end address
	// - flatTcs has no overlapping addresses.
	// - the last end address in flatTcs is the end addr of the last item.
	if cut == nil {
		cut = flattenDefaultCut
	}
	if union == nil {
		union = flattenDefaultUnion
	}
	flatTcs := TrackerCounters{}
	for _, tc := range *tcs {
		tcStartAddr := tc.AR.Ranges()[0].Addr()
		tcEndAddr := tc.AR.Ranges()[0].EndAddr()

		if len(flatTcs) == 0 ||
			flatTcs[len(flatTcs)-1].AR.Pid() != tc.AR.Pid() ||
			flatTcs[len(flatTcs)-1].AR.Ranges()[0].EndAddr() <= tcStartAddr {
			flatTcs = append(flatTcs, tc)
			continue
		}

		// oltci indexes flattened TrackerCounters that
		// overlap with tc.
		// Walk backwards to find the first overlapping TrackerCounter.
		oltci := len(flatTcs) - 1
		for oltci >= 0 {
			prevEndAddr := flatTcs[oltci].AR.Ranges()[0].EndAddr()
			if prevEndAddr < tcStartAddr {
				// No overlap at this index, the
				// previous index was the last one.
				oltci++
				break
			}
			oltci--
		}
		if oltci < 0 {
			oltci = 0
		}
		oldFlatTcsTail := flatTcs[oltci:]
		newFlatTcsTail := make(TrackerCounters, 0, len(oldFlatTcsTail)+1)
		// Walk forward from the first overlapping
		// TrackerConter. oltc is the overlapping tracker
		// counter being handled.
		for oltci <= len(flatTcs)-1 {
			oltc := flatTcs[oltci]
			oltcStartAddr := oltc.AR.Ranges()[0].Addr()
			oltcEndAddr := oltc.AR.Ranges()[0].EndAddr()
			if oltcStartAddr < tcStartAddr {
				handledOltc := cut(oltc, NewAddrRange(oltcStartAddr, tcStartAddr))
				oltc = cut(oltc, NewAddrRange(tcStartAddr, oltcEndAddr))
				oltcStartAddr = tcStartAddr
				newFlatTcsTail = append(newFlatTcsTail, handledOltc)
			}
			if oltcStartAddr > tcStartAddr {
				panic("trackercounters flatten assertion error: not sorted by addresses")
			}
			switch {
			case oltcEndAddr < tcEndAddr:
				handledTc := cut(tc, NewAddrRange(tcStartAddr, oltcEndAddr))
				newFlatTcsTail = append(newFlatTcsTail, union(oltc, handledTc))
				tc = cut(tc, NewAddrRange(oltcEndAddr, tcEndAddr))
				tcStartAddr = oltcEndAddr
			case oltcEndAddr == tcEndAddr:
				newFlatTcsTail = append(newFlatTcsTail, union(oltc, tc))
				tcStartAddr = tcEndAddr
			case oltcEndAddr > tcEndAddr:
				handledOltc := cut(oltc, NewAddrRange(oltcStartAddr, tcEndAddr))
				newFlatTcsTail = append(newFlatTcsTail, union(handledOltc, tc))
				oltc = cut(oltc, NewAddrRange(tcEndAddr, oltcEndAddr))
				newFlatTcsTail = append(newFlatTcsTail, oltc)
				tcStartAddr = tcEndAddr
			}
			oltci++
		}
		if tcStartAddr < tcEndAddr {
			newFlatTcsTail = append(newFlatTcsTail, cut(tc, NewAddrRange(tcStartAddr, tcEndAddr)))
		}
		flatTcs = append(flatTcs[:len(flatTcs)-len(oldFlatTcsTail)], newFlatTcsTail...)
	}
	return &flatTcs
}

func (tcs *TrackerCounters) RegionsMerged() *TrackerCounters {
	tcs.SortByAddr()
	mergedTcs := &TrackerCounters{}
	// TODO: this proto works currently only for disjoint tc.AR's
	for _, tc := range *tcs {
		if len(tc.AR.Ranges()) != 1 {
			// TODO: this proto works only for single-range counters
			return nil
		}
		r := tc.AR.Ranges()[0]
		if len(*mergedTcs) > 0 {
			prevTc := &(*mergedTcs)[len(*mergedTcs)-1]
			if prevTc.AR.Pid() == tc.AR.Pid() &&
				prevTc.AR.Ranges()[0].EndAddr() == r.Addr() {
				prevTc.AR = &AddrRanges{
					pid: prevTc.AR.Pid(),
					addrs: []AddrRange{
						*NewAddrRange(prevTc.AR.Ranges()[0].Addr(), r.EndAddr()),
					},
				}
				continue
			}
		}
		*mergedTcs = append(*mergedTcs, tc)
	}
	return mergedTcs
}

func (tcs *TrackerCounters) RangeHeat() []*RangeHeat {
	tcs.SortByAddr()
	rhs := []*RangeHeat{}
	// TODO: this proto works currently only for disjoint tc.AR's
	for _, tc := range *tcs {
		heat := tc.Accesses + tc.Reads + tc.Writes
		if len(tc.AR.Ranges()) != 1 {
			// TODO: this proto works only for single-range counters
			return nil
		}
		r := tc.AR.Ranges()[0]
		if len(rhs) > 0 {
			prevRh := rhs[len(rhs)-1]
			if prevRh.Range.EndAddr() == r.Addr() &&
				prevRh.Heat == heat {
				// two ranges with the same heat: combine them
				prevRh.Range = *NewAddrRange(prevRh.Range.Addr(), r.EndAddr())
				continue
			}
		}
		rhs = append(rhs, &RangeHeat{tc.AR.Pid(), r, heat})
	}
	return rhs
}
