// The soft dirty tracker is capable of detecting memory writes.
// https://www.kernel.org/doc/Documentation/vm/soft-dirty.txt

package memtier

import (
	"io/ioutil"
	"strconv"
)

type TrackerSoftDirty struct {
	regions map[int][]*AddrRanges
}

func init() {
	trackers["softdirty"] = NewTrackerSoftDirty
}

func NewTrackerSoftDirty() Tracker {
	return &TrackerSoftDirty{
		regions: make(map[int][]*AddrRanges, 0),
	}
}

func (tsd *TrackerSoftDirty) SetConfigJson(configJson string) error {
	return nil
}

func (tsd *TrackerSoftDirty) AddRanges(ar *AddrRanges) {
	pid := ar.Pid()
	if regions, ok := tsd.regions[pid]; ok {
		tsd.regions[pid] = append(regions, ar)
	} else {
		tsd.regions[pid] = []*AddrRanges{ar}
	}
}

func (tsd *TrackerSoftDirty) RemovePid(pid int) {
	delete(tsd.regions, pid)
}

func (tsd *TrackerSoftDirty) Reset() {
	for pid := range tsd.regions {
		pidString := strconv.Itoa(pid)
		path := "/proc/" + pidString + "/clear_refs"
		err := ioutil.WriteFile(path, []byte("4"), 0600)
		if err != nil {
			// This process cannot be tracked anymore, remove it.
			tsd.RemovePid(pid)
		}
	}
}

func (tsd *TrackerSoftDirty) GetCounters() *TrackerCounters {
	// Room for optimization:
	// 1. We use only the number of pages per address range. This
	//    could be done without building the list of pages.
	// 2. We open and close /proc/pid/pagemap for each address range,
	//    yet once would be enough.
	tcs := &TrackerCounters{}
	pageAttrs := PagePresent | PageExclusive | PageDirty
	for pid, allPidAddrRanges := range tsd.regions {
		for _, addrRanges := range allPidAddrRanges {
			pageSet, err := addrRanges.PagesMatching(pageAttrs)
			if err != nil {
				tsd.RemovePid(pid)
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
