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
	TrackReferenced bool   // Track /proc/kpageflags PKF_REFERENCED bit.
	TrackSoftDirty  bool   // Track /proc/PID/pagemap PM_SOFT_DIRTY bit.
	PagesInRegion   uint64 // size of a memory region in the
	// number of pages.
	MaxCountPerRegion uint64 // 0: unlimited, increase counters by
	// number of pages with tracked bits in
	// whole region. 1: increase counters
	// by at most 1 per tracked bits in
	// pages in the region.
	Interval        uint64 // interval in microseconds
	RegionsUpdateUs uint64 // interval in microseconds
	SkipPageProb    int    // Sampling premilles: 0: read all
	// pages, 1000: skip next page with
	// probability 1.0.
}

// TODO: Referenced tracking does not work properly.
// TODO: if PFNs are tracked, refuse to start or disable if enabled
// /proc/sys/kernel/numa_balancing
const trackerSoftDirtyDefaults string = `{"Interval":1000000,"PagesInRegion":512,"SkipPageProb":0,"TrackReferenced":false,"TrackSoftDirty":true,"MaxCountPerRegion":1}`

type accessCounter struct {
	a uint64 // number of times pages getting accessed
	w uint64 // number of times pages getting written
}

type TrackerSoftDirty struct {
	mutex   sync.Mutex
	config  *TrackerSoftDirtyConfig
	regions map[int][]*AddrRanges
	// accesses maps pid -> startAddr -> lengthPages -> num of access & writes
	accesses  map[int]map[uint64]map[uint64]*accessCounter
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
		accesses: make(map[int]map[uint64]map[uint64]*accessCounter),
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

func (t *TrackerSoftDirty) GetConfigJson() string {
	if t.config == nil {
		return ""
	}
	if configStr, err := json.Marshal(t.config); err == nil {
		return string(configStr)
	}
	return ""
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
	log.Debugf("TrackerSoftDirty: AddPids(%v)\n", pids)
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
	log.Debugf("TrackerSoftDirty: RemovePids(%v)\n", pids)
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
	t.accesses = make(map[int]map[uint64]map[uint64]*accessCounter)
}

func (t *TrackerSoftDirty) GetCounters() *TrackerCounters {
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
					Writes:   accessCounts.w,
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
	t.clearPageBits()
	go t.sampler()
	return nil
}

func (t *TrackerSoftDirty) Stop() {
	if t.toSampler != nil {
		t.toSampler <- 0
	}
}

func (t *TrackerSoftDirty) sampler() {
	log.Debugf("TrackerSoftDirty: online\n")
	defer log.Debugf("TrackerSoftDirty: offline\n")
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
			t.clearPageBits()
		}
	}
}

func (t *TrackerSoftDirty) countPages() {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	pmAttrs := PMPresentSet | PMExclusiveSet

	var kpfFile *procKpageflagsFile
	var err error

	trackSoftDirty := t.config.TrackSoftDirty
	trackReferenced := t.config.TrackReferenced
	maxCount := t.config.MaxCountPerRegion
	skipPageProb := t.config.SkipPageProb

	cntPagesAccessed := uint64(0)
	cntPagesWritten := uint64(0)

	if trackReferenced {
		// Referenced bits are in /proc/kpageflags.
		// Open the file already.
		kpfFile, err = ProcKpageflagsOpen()
		if err != nil {
			return
		}
		defer kpfFile.Close()
	}

	// pageHandler is called for all matching pages in the pagemap.
	// It counts number of pages accessed and written in a region.
	// The result is stored to cntPagesAccessed and cntPagesWritten.
	pageHandler := func(pagemapBits uint64, pageAddr uint64) int {
		if trackSoftDirty {
			if pagemapBits&PM_SOFT_DIRTY == PM_SOFT_DIRTY {
				cntPagesWritten += 1
				if !trackReferenced {
					cntPagesAccessed += 1
				}
			}
		}
		if trackReferenced {
			pfn := pagemapBits & PM_PFN
			flags, err := kpfFile.ReadFlags(pfn)
			if err != nil {
				return -1
			}
			if flags&KPF_REFERENCED == KPF_REFERENCED {
				cntPagesAccessed += 1
			}
		}
		// If we have exceeded the max count per region on the
		// counters we are tracking, stop reading pages further.
		if (!trackSoftDirty || cntPagesWritten > maxCount) &&
			(!trackReferenced || cntPagesAccessed > maxCount) {
			return -1
		}
		if skipPageProb > 0 {
			// skip pages in sampling read
			if skipPageProb >= 1000 {
				return -1
			}
			n := 0
			for rand.Intn(1000) < skipPageProb {
				n += 1
			}
			return n
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
			cntPagesWritten = 0

			err := pmFile.ForEachPage(addrRanges.Ranges(), pmAttrs, pageHandler)
			if err != nil {
				t.removePid(pid)
				break
			}
			if cntPagesAccessed > maxCount {
				cntPagesAccessed = maxCount
			}
			if cntPagesWritten > maxCount {
				cntPagesWritten = maxCount
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
			counts.w += cntPagesWritten
		}
		pmFile.Close()
	}
}

func (t *TrackerSoftDirty) clearPageBits() {
	var err error
	for pid := range t.regions {
		pidString := strconv.Itoa(pid)
		path := "/proc/" + pidString + "/clear_refs"
		if t.config.TrackSoftDirty {
			err = procWrite(path, []byte("4\n"))
		}
		if t.config.TrackReferenced && err == nil {
			err = procWrite(path, []byte("1\n"))
		}
		if err != nil {
			// This process cannot be tracked anymore, remove it.
			t.removePid(pid)
		}
	}
}
