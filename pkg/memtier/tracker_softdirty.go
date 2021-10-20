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

// The soft dirty tracker is capable of detecting memory writes.
// https://www.kernel.org/doc/Documentation/vm/soft-dirty.txt

package memtier

import (
	"encoding/json"
	"fmt"
	"strconv"
)

type TrackerSoftDirtyConfig struct {
}

type TrackerSoftDirty struct {
	config  *TrackerSoftDirtyConfig
	regions map[int][]*AddrRanges
}

func init() {
	TrackerRegister("softdirty", NewTrackerSoftDirty)
}

func NewTrackerSoftDirty() (Tracker, error) {
	if !procFileExists("/proc/self/clear_refs") {
		return nil, fmt.Errorf("no platform support: /proc/pid/clear_refs missing")
	}
	return &TrackerSoftDirty{
		regions: make(map[int][]*AddrRanges, 0),
	}, nil
}

func (t *TrackerSoftDirty) SetConfigJson(configJson string) error {
	config := TrackerSoftDirtyConfig{}
	if err := json.Unmarshal([]byte(configJson), &config); err != nil {
		return err
	}
	return nil
}

func (t *TrackerSoftDirty) addRanges(ar *AddrRanges) {
	pid := ar.Pid()
	for _, r := range ar.Flatten() {
		if regions, ok := t.regions[pid]; ok {
			t.regions[pid] = append(regions, r)
		} else {
			t.regions[pid] = []*AddrRanges{r}
		}
	}
}

func (t *TrackerSoftDirty) AddPids(pids []int) {
	for _, pid := range pids {
		p := NewProcess(pid)
		if ar, err := p.AddressRanges(); err == nil {
			// filter out single-page address ranges
			ar = ar.Filter(func(r AddrRange) bool { return r.Length() > 1 })
			ar = ar.SplitLength(256) // 256 * 4k pages = 1MB regions
			t.addRanges(ar)
			continue
		}
	}
}

func (t *TrackerSoftDirty) RemovePids(pids []int) {
	if pids == nil {
		t.regions = make(map[int][]*AddrRanges, 0)
		return
	}
	for _, pid := range pids {
		t.removePid(pid)
	}
}

func (t *TrackerSoftDirty) removePid(pid int) {
	delete(t.regions, pid)
}

func (t *TrackerSoftDirty) ResetCounters() {
	for pid := range t.regions {
		pidString := strconv.Itoa(pid)
		path := "/proc/" + pidString + "/clear_refs"
		err := procWrite(path, []byte("4"))
		if err != nil {
			// This process cannot be tracked anymore, remove it.
			t.removePid(pid)
		}
	}
}

func (t *TrackerSoftDirty) GetCounters() *TrackerCounters {
	// Room for optimization:
	// 1. We use only the number of pages per address range. This
	//    could be done without building the list of pages.
	// 2. We open and close /proc/pid/pagemap for each address range,
	//    yet once would be enough.
	tcs := &TrackerCounters{}
	pageAttrs := PagePresent | PageExclusive | PageDirty
	for pid, allPidAddrRanges := range t.regions {
		for _, addrRanges := range allPidAddrRanges {
			pageSet, err := addrRanges.PagesMatching(pageAttrs)
			if err != nil {
				t.removePid(pid)
				break
			}
			numberOfPagesWritten := len(pageSet.Pages())
			tc := TrackerCounter{
				Accesses: numberOfPagesWritten,
				Reads:    0,
				Writes:   numberOfPagesWritten,
				AR:       addrRanges,
			}
			*tcs = append(*tcs, tc)
		}
	}
	return tcs
}

func (t *TrackerSoftDirty) Start() error {
	t.ResetCounters()
	return nil
}

func (t *TrackerSoftDirty) Stop() {
}
