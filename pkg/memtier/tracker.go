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
	ResetCounters()
	GetCounters() *TrackerCounters
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
