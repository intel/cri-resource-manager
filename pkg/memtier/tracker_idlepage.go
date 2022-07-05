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
	// PagesInRegion is the number of pages in every address range
	// that is being watched and moved from a NUMA node to another.
	PagesInRegion uint64
	// MaxCountPerRegion is the maximum number of pages that are
	// reported to be accessed. When the maximum number is reached
	// during scanning a region, the rest of the pages in the
	// region are skipped. Value 0 means unlimited (that is, the
	// maximum number will be at most the same as PagesInRegion).
	MaxCountPerRegion uint64
	// ScanIntervalMs defines page scan interval in milliseconds.
	ScanIntervalMs uint64
	// RegionsUpdateMs defines process memory region update
	// interval in milliseconds. Regions are updated just before
	// scanning pages if the interval has passed. Value 0 means
	// that regions are updated before every scan.
	RegionsUpdateMs uint64
	// PagemapReadahead is the number of pages to be read ahead
	// from /proc/PID/pagemap. Every page information is 16 B. If
	// 0 (undefined) use a default, if -1, disable readahead.
	PagemapReadahead int
	// KpageflagsReadahead is the number of pages to be read ahead
	// from /proc/kpageflags. Every page information is 16 B. If 0
	// (undefined) use a default, if -1, disable readahead.
	KpageflagsReadahead int
	// BitmapReadahead is the number of chunks of 64 pages to be
	// read ahead from /sys/kernel/mm/page_idle/bitmap. If 0
	// (undefined) use a default, if -1, disable readahead.
	BitmapReadahead int
}

const trackerIdlePageDefaults string = `{"PagesInRegion":512,"MaxCountPerRegion":1,"ScanIntervalMs":5000,"RegionsUpdateMs":10000}`

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
	raes      rawAccessEntries
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
	config := &TrackerIdlePageConfig{}
	if err := unmarshal(configJson, config); err != nil {
		return err
	}
	t.config = config
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

func (t *TrackerIdlePage) addRanges(pid int) {
	delete(t.regions, pid)
	p := NewProcess(pid)
	if ar, err := p.AddressRanges(); err == nil {
		// filter out single-page address ranges
		ar = ar.Filter(func(r AddrRange) bool { return r.Length() > 1 })
		ar = ar.SplitLength(t.config.PagesInRegion)
		for _, r := range ar.Flatten() {
			if regions, ok := t.regions[pid]; ok {
				t.regions[pid] = append(regions, r)
			} else {
				t.regions[pid] = []*AddrRanges{r}
			}
		}
	}
}

func (t *TrackerIdlePage) AddPids(pids []int) {
	log.Debugf("TrackerIdlePage: AddPids(%v)\n", pids)
	for _, pid := range pids {
		t.addRanges(pid)
	}
}

func (t *TrackerIdlePage) RemovePids(pids []int) {
	log.Debugf("TrackerIdlePage: RemovePids(%v)\n", pids)
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
		log.Warnf("unreliable idlepage tracking: /proc/sys/kernel/numa_balancing is not 0")
	}
	t.toSampler = make(chan byte, 1)
	t.setIdleBits()
	go t.sampler()
	return nil
}

func (t *TrackerIdlePage) Stop() {
	if t.toSampler != nil {
		t.toSampler <- 0
	}
}

func (t *TrackerIdlePage) sampler() {
	log.Debugf("TrackerIdlePage: online\n")
	defer log.Debugf("TrackerIdlePage: offline\n")
	ticker := time.NewTicker(time.Duration(t.config.ScanIntervalMs) * time.Millisecond)
	defer ticker.Stop()
	lastRegionsUpdateNs := time.Now().UnixNano()
	for {
		stats.Store(StatsHeartbeat{"TrackerIdlePage.sampler"})
		select {
		case <-t.toSampler:
			close(t.toSampler)
			t.toSampler = nil
			return
		case <-ticker.C:
			currentNs := time.Now().UnixNano()
			if time.Duration(currentNs-lastRegionsUpdateNs) >= time.Duration(t.config.RegionsUpdateMs)*time.Millisecond {
				for pid, _ := range t.regions {
					t.addRanges(pid)
				}
				lastRegionsUpdateNs = currentNs
			}
			t.countPages()
			t.setIdleBits()
		}
	}
}

func (t *TrackerIdlePage) countPages() {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	maxCount := t.config.MaxCountPerRegion
	if maxCount == 0 {
		maxCount = t.config.PagesInRegion
	}
	cntPagesAccessed := uint64(0)

	totAccessed := uint64(0)
	totScanned := uint64(0)

	kpfFile, err := ProcKpageflagsOpen()
	if err != nil {
		return
	}
	defer kpfFile.Close()
	if t.config.KpageflagsReadahead > 0 {
		kpfFile.SetReadahead(t.config.KpageflagsReadahead)
	}
	if t.config.KpageflagsReadahead == -1 {
		kpfFile.SetReadahead(0)
	}

	bmFile, err := ProcPageIdleBitmapOpen()
	if err != nil {
		return
	}
	defer bmFile.Close()
	if t.config.BitmapReadahead > 0 {
		bmFile.SetReadahead(t.config.BitmapReadahead)
	}
	if t.config.BitmapReadahead == -1 {
		bmFile.SetReadahead(0)
	}

	// pageHandler is called for all matching pages in the pagemap.
	// It counts number of pages accessed and written in a region.
	// The result is stored to cntPagesAccessed and cntPagesWritten.
	pageHandler := func(pagemapBits uint64, pageAddr uint64) int {
		totScanned += 1
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

	scanStartTime := time.Now().UnixNano()
	for pid, allPidAddrRanges := range t.regions {
		totScanned = 0
		totAccessed = 0
		pmFile, err := ProcPagemapOpen(pid)
		if err != nil {
			t.removePid(pid)
			continue
		}
		if t.config.PagemapReadahead > 0 {
			pmFile.SetReadahead(t.config.PagemapReadahead)
		}
		if t.config.PagemapReadahead == -1 {
			pmFile.SetReadahead(0)
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
			totAccessed += cntPagesAccessed
			if t.raes.data != nil {
				rae := &rawAccessEntry{
					timestamp: scanStartTime,
					pid:       pid,
					addr:      addr,
					length:    lengthPages,
					accessCounter: accessCounter{
						a: cntPagesAccessed,
					},
				}
				t.raes.store(rae)
			}
		}
		pmFile.Close()
		scanEndTime := time.Now().UnixNano()
		stats.Store(StatsPageScan{
			pid:      pid,
			scanned:  totScanned,
			accessed: totAccessed,
			written:  0,
			timeUs:   (scanEndTime - scanStartTime) / int64(time.Microsecond),
		})
		scanStartTime = scanEndTime
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

func (t *TrackerIdlePage) Dump(args []string) string {
	usage := "Usage: dump raw PARAMS"
	if len(args) == 0 {
		return usage
	}
	if args[0] == "raw" {
		return t.raes.dump(args[1:])
	}
	return ""
}
