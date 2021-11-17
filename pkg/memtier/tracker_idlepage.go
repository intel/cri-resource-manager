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

// The idle page tracker uses /sys/kernel/mm/page_idle/bitmap

package memtier

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type TrackerIdlePageConfig struct {
	PagesInRegion uint64 // size of a memory region in the
	// number of pages.
	MaxCountPerRegion uint64 // 0: unlimited, increase counters by
	// number of pages with tracked bits in
	// whole region. 1: increase counters
	// by at most 1 per tracked bits in
	// pages in the region.
	Interval        uint64 // interval in microseconds
	RegionsUpdateUs uint64 // interval in microseconds
}

const trackerIdlePageDefaults string = `{"Interval":5000000,"PagesInRegion":512,"MaxCountPerRegion":1}`

type TrackerIdlePage struct {
	mutex   sync.Mutex
	config  *TrackerIdlePageConfig
	regions map[int][]*AddrRanges
	pmAttrs uint64 // only pages with these pagemap attribute
	// requirements are handled accesses maps pid
	// -> startAddr -> lengthPages -> num of
	// accesses
	accesses  map[int]map[uint64]map[uint64]*accessCounter
	toSampler chan byte
}

func init() {
	TrackerRegister("idlepage", NewTrackerIdlePage)
}

func NewTrackerIdlePage() (Tracker, error) {
	if bmFile, err := ProcPageIdleBitmapOpen(); err != nil {
		return nil, fmt.Errorf("no idle page platform support: %s", err)
	} else {
		bmFile.Close()
	}
	t := &TrackerIdlePage{
		regions:  make(map[int][]*AddrRanges),
		accesses: make(map[int]map[uint64]map[uint64]*accessCounter),
		pmAttrs:  PMPresentSet | PMExclusiveSet,
	}
	err := t.SetConfigJson(trackerIdlePageDefaults)
	if err != nil {
		return nil, fmt.Errorf("invalid idlepage default configuration")
	}
	return t, nil
}

func (t *TrackerIdlePage) SetConfigJson(configJson string) error {
	config := TrackerIdlePageConfig{}
	if err := json.Unmarshal([]byte(configJson), &config); err != nil {
		return err
	}
	t.config = &config
	return nil
}

func (t *TrackerIdlePage) GetConfigJson() string {
	if t.config == nil {
		return ""
	}
	if configStr, err := json.Marshal(t.config); err == nil {
		return string(configStr)
	}
	return ""
}

func (t *TrackerIdlePage) addRanges(ar *AddrRanges) {
	pid := ar.Pid()
	for _, r := range ar.Flatten() {
		if regions, ok := t.regions[pid]; ok {
			t.regions[pid] = append(regions, r)
		} else {
			t.regions[pid] = []*AddrRanges{r}
		}
	}
}

func (t *TrackerIdlePage) AddPids(pids []int) {
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

func (t *TrackerIdlePage) RemovePids(pids []int) {
	if pids == nil {
		t.regions = make(map[int][]*AddrRanges, 0)
		return
	}
	for _, pid := range pids {
		t.removePid(pid)
	}
}

func (t *TrackerIdlePage) removePid(pid int) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	delete(t.regions, pid)
	delete(t.accesses, pid)
}

func (t *TrackerIdlePage) ResetCounters() {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.accesses = make(map[int]map[uint64]map[uint64]*accessCounter)
}

func (t *TrackerIdlePage) GetCounters() *TrackerCounters {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	tcs := &TrackerCounters{}
	for pid, addrLenCount := range t.accesses {
		for start, lenCount := range addrLenCount {
			for length, accessCounts := range lenCount {
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
					Accesses: accessCounts.a,
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

func (t *TrackerIdlePage) Start() error {
	if t.toSampler != nil {
		return fmt.Errorf("sampler already running")
	}
	if n, err := procReadInt("/proc/sys/kernel/numa_balancing"); err != nil || n != 0 {
		return fmt.Errorf("/proc/sys/kernel/numa_balancing must be 0")
	}
	t.toSampler = make(chan byte, 1)
	t.setIdleBits()
	go t.sampler()
	log.Debugf("TrackerIdlePage: online\n")
	return nil
}

func (t *TrackerIdlePage) Stop() {
	if t.toSampler != nil {
		t.toSampler <- 0
	}
	log.Debugf("TrackerIdlePage: offline\n")
}

func (t *TrackerIdlePage) sampler() {
	ticker := time.NewTicker(time.Duration(t.config.Interval) * time.Microsecond)
	defer ticker.Stop()
	for {
		select {
		case <-t.toSampler:
			close(t.toSampler)
			t.toSampler = nil
			return
		case <-ticker.C:
			t.countPages()
			t.setIdleBits()
		}
	}
}

func (t *TrackerIdlePage) countPages() {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	maxCount := t.config.MaxCountPerRegion
	cntPagesAccessed := uint64(0)

	// Referenced bits are in /proc/kpageflags.
	// Open the file already.
	kpfFile, err := ProcKpageflagsOpen()
	if err != nil {
		return
	}
	defer kpfFile.Close()
	kpfFile.SetReadahead(256)

	bmFile, err := ProcPageIdleBitmapOpen()
	if err != nil {
		return
	}
	defer bmFile.Close()
	bmFile.SetReadahead(8)

	// pageHandler is called for all matching pages in the pagemap.
	// It counts number of pages accessed and written in a region.
	// The result is stored to cntPagesAccessed and cntPagesWritten.
	pageHandler := func(pagemapBits uint64, pageAddr uint64) int {
		pfn := pagemapBits & PM_PFN
		pageIdle, err := bmFile.GetIdle(pfn)
		if err != nil {
			return -1
		}
		if !pageIdle {
			flags, err := kpfFile.ReadFlags(pfn)
			if err != nil {
				return -1
			}
			if ((flags>>KPFB_COMPOUND_HEAD)&1 == 0 &&
				(flags>>KPFB_COMPOUND_TAIL)&1 == 0) ||
				((flags>>KPFB_COMPOUND_HEAD)&1 == 1) {
				// Compound tail pages never get idle bit,
				// so read accesses only from idle bits of
				// normal pages and heads of compound pages.
				cntPagesAccessed += 1
			}
		}
		if cntPagesAccessed >= maxCount {
			return -1
		}
		return 0
	}

	for pid, allPidAddrRanges := range t.regions {
		pmFile, err := ProcPagemapOpen(pid)
		if err != nil {
			t.removePid(pid)
			continue
		}
		for _, addrRanges := range allPidAddrRanges {
			cntPagesAccessed = 0

			err := pmFile.ForEachPage(addrRanges.Ranges(), t.pmAttrs, pageHandler)
			if err != nil {
				t.removePid(pid)
				break
			}
			if cntPagesAccessed > maxCount {
				cntPagesAccessed = maxCount
			}
			addrLenCounts, ok := t.accesses[pid]
			if !ok {
				addrLenCounts = make(map[uint64]map[uint64]*accessCounter)
				t.accesses[pid] = addrLenCounts
			}
			addr := addrRanges.Ranges()[0].Addr()
			lenCounts, ok := addrLenCounts[addr]
			if !ok {
				lenCounts = make(map[uint64]*accessCounter)
				addrLenCounts[addr] = lenCounts
			}
			lengthPages := addrRanges.Ranges()[0].Length()
			counts, ok := lenCounts[lengthPages]
			if !ok {
				counts = &accessCounter{0, 0}
				lenCounts[lengthPages] = counts
			}
			counts.a += cntPagesAccessed
		}
		pmFile.Close()
	}
}

func (t *TrackerIdlePage) setIdleBits() {
	bmFile, err := ProcPageIdleBitmapOpen()
	if err != nil {
		return
	}
	defer bmFile.Close()
	// Avoid making 1 seek + 1 write syscalls for each page that
	// we mark idle by setting 64 pages idle on one round.
	// alreadyIdle stores pages that have been already marked idle,
	// so those can be skipped.
	alreadyIdle := map[uint64]setMemberType{}

	pageHandler := func(pagemapBits uint64, pageAddr uint64) int {
		pfn := pagemapBits & PM_PFN
		pfnFileOffset := pfn / 64 * 8
		if _, ok := alreadyIdle[pfnFileOffset]; !ok {
			if err := bmFile.SetIdleAll(pfn); err != nil {
				return -1
			}
			alreadyIdle[pfnFileOffset] = setMember
		}
		return 0
	}

	for pid, allPidAddrRanges := range t.regions {
		pmFile, err := ProcPagemapOpen(pid)
		if err != nil {
			t.removePid(pid)
			continue
		}
		for _, addrRanges := range allPidAddrRanges {
			err := pmFile.ForEachPage(addrRanges.Ranges(), t.pmAttrs, pageHandler)
			if err != nil {
				t.removePid(pid)
				break
			}
		}
		pmFile.Close()
	}
}
