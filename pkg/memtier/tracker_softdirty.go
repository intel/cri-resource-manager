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
	"math/rand"
	"strconv"
	"sync"
	"time"
)

type TrackerSoftDirtyConfig struct {
	SkipPageProb    int    // 100000 = probability 1.0
	AggregationUs   uint64 // interval in microseconds
	PagesInRegion   uint64 // number of pages in a region
	RegionsUpdateUs uint64 // interval in microseconds
}

const trackerSoftDirtyDefaults string = `{"AggregationUs":1000000,"PagesInRegion":512,"SkipPageProb":0}`

type TrackerSoftDirty struct {
	mutex   sync.Mutex
	config  *TrackerSoftDirtyConfig
	regions map[int][]*AddrRanges
	// accesses maps pid -> startAddr -> lengthPages -> writeCount
	accesses  map[int]map[uint64]map[uint64]uint64
	toSampler chan byte
}

func init() {
	TrackerRegister("softdirty", NewTrackerSoftDirty)
}

func NewTrackerSoftDirty() (Tracker, error) {
	if !procFileExists("/proc/self/clear_refs") {
		return nil, fmt.Errorf("no platform support: /proc/pid/clear_refs missing")
	}
	t := &TrackerSoftDirty{
		regions:  make(map[int][]*AddrRanges),
		accesses: make(map[int]map[uint64]map[uint64]uint64),
	}
	err := t.SetConfigJson(trackerSoftDirtyDefaults)
	if err != nil {
		return nil, fmt.Errorf("invalid softdirty default configuration")
	}
	return t, nil
}

func (t *TrackerSoftDirty) SetConfigJson(configJson string) error {
	config := TrackerSoftDirtyConfig{}
	if err := json.Unmarshal([]byte(configJson), &config); err != nil {
		return err
	}
	t.config = &config
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
			ar = ar.SplitLength(t.config.PagesInRegion)
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
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.accesses = make(map[int]map[uint64]map[uint64]uint64)
}

func (t *TrackerSoftDirty) GetCounters() *TrackerCounters {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	tcs := &TrackerCounters{}
	for pid, addrLenCount := range t.accesses {
		for start, lenCount := range addrLenCount {
			for length, count := range lenCount {
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
					Writes:   count,
					AR:       &addrRange,
				}
				*tcs = append(*tcs, tc)
			}
		}
	}
	return tcs
}

func (t *TrackerSoftDirty) Start() error {
	if t.toSampler != nil {
		return fmt.Errorf("sampler already running")
	}
	t.toSampler = make(chan byte, 1)
	t.clearDirtyBits()
	go t.sampler()
	return nil
}

func (t *TrackerSoftDirty) Stop() {
	if t.toSampler != nil {
		t.toSampler <- 0
	}
}

func (t *TrackerSoftDirty) sampler() {
	ticker := time.NewTicker(time.Duration(t.config.AggregationUs) * time.Microsecond)
	defer ticker.Stop()
	for {
		select {
		case <-t.toSampler:
			close(t.toSampler)
			t.toSampler = nil
			return
		case <-ticker.C:
			t.countDirtyPages()
			t.clearDirtyBits()
		}
	}
}

func (t *TrackerSoftDirty) countDirtyPages() {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	pageAttrs := PagePresent | PageExclusive | PageDirty
	for pid, allPidAddrRanges := range t.regions {
		pmf, err := procPagemapOpen(pid)
		if err != nil {
			t.removePid(pid)
			continue
		}
		for _, addrRanges := range allPidAddrRanges {
			numberOfPagesWritten, err := addrRanges.PageCountMatching(pageAttrs, pmf,
				func(x uint64) bool {
					if t.config.SkipPageProb > 0 &&
						rand.Intn(100000) < t.config.SkipPageProb {
						return true
					}
					return false
				})
			if err != nil {
				t.removePid(pid)
				break
			}
			addrLenCount, ok := t.accesses[pid]
			if !ok {
				addrLenCount = make(map[uint64]map[uint64]uint64)
				t.accesses[pid] = addrLenCount
			}
			addr := addrRanges.Ranges()[0].Addr()
			lenCount, ok := addrLenCount[addr]
			if !ok {
				lenCount = make(map[uint64]uint64)
				addrLenCount[addr] = lenCount
			}
			lengthPages := addrRanges.Ranges()[0].Length()
			count, ok := lenCount[lengthPages]
			if !ok {
				count = 0
			}
			lenCount[lengthPages] = count + numberOfPagesWritten
		}
		pmf.Close()
	}
}

func (t *TrackerSoftDirty) clearDirtyBits() {
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
