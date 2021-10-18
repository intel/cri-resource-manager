package memtier

import (
	"fmt"
	"sort"
	"strings"
)

type TrackerCounters []TrackerCounter

type TrackerCounter struct {
	Accesses int
	Reads    int
	Writes   int
	AR       *AddrRanges
}

type Tracker interface {
	SetConfigJson(configJson string) error
	AddRanges(ar *AddrRanges)
	RemovePid(int)
	Reset()
	GetCounters() *TrackerCounters
}

// trackers is a map of tracker name -> tracker creator
var trackers map[string]func() Tracker = make(map[string]func() Tracker, 0)

func Trackers() []string {
	keys := make([]string, 0, len(trackers))
	for key := range trackers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func NewTracker(name string) Tracker {
	if newT, ok := trackers[name]; ok {
		return newT()
	}
	return nil
}

func (tcs *TrackerCounters) SortByAccesses() {
	sort.Slice(*tcs, func(i, j int) bool {
		return (*tcs)[i].Accesses < (*tcs)[j].Accesses
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
