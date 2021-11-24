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
	"testing"
	"time"
)

func TestUpdateCountersBoundaryCheck(t *testing.T) {
	hm := NewCounterHeatmap()

	PS := constUPagesize
	tcs0 := TrackerCounters{
		// Memory regions have a hole.
		// [100..150][150..200][200..250]<hole>[500..600]
		TrackerCounter{
			AR: NewAddrRanges(2000,
				AddrRange{100 * PS, 50},
				AddrRange{150 * PS, 50}),
		},
		TrackerCounter{
			Accesses: 1,
			AR: NewAddrRanges(2000,
				AddrRange{200 * PS, 50}),
		},
		TrackerCounter{
			Writes: 100,
			AR: NewAddrRanges(2000,
				AddrRange{500 * PS, 100}),
		},
	}
	timestamp := int64(0)
	hm.UpdateFromCounters(&tcs0, timestamp)

	fmt.Println(hm.Dump())

	// Boundary value check: nil/non-nil
	if hm.HeatRangeAt(4040, 0) != nil {
		t.Errorf("nil expected when requesting heat for non-existing pid")
	}
	if hm.HeatRangeAt(2000, 99*PS) != nil {
		t.Errorf("nil expected when requesting heat for address before any range")
	}
	if hm.HeatRangeAt(2000, 100*PS) == nil {
		t.Errorf("non-nil expected when requesting heat for the start address in the first range")
	}
	if hm.HeatRangeAt(2000, 150*PS) == nil {
		t.Errorf("non-nil expected when requesting heat for the start address in the second range")
	}
	if hm.HeatRangeAt(2000, 199*PS) == nil {
		t.Errorf("non-nil expected when requesting heat for the last address in the second range")
	}
	if hm.HeatRangeAt(2000, 200*PS) == nil {
		t.Errorf("non-nil expected when requesting heat for the first address in the third range")
	}
	if hm.HeatRangeAt(2000, 249*PS) == nil {
		t.Errorf("non-nil expected when requesting heat for the last address in the third range")
	}
	if hm.HeatRangeAt(2000, 250*PS) != nil {
		t.Errorf("nil expected when requesting heat for the first address in the hole")
	}
	if hm.HeatRangeAt(2000, 499*PS) != nil {
		t.Errorf("nil expected when requesting heat for the last address in the hole")
	}
	if hm.HeatRangeAt(2000, 500*PS) == nil {
		t.Errorf("non-nil expected when requesting heat for the first address after the hole")
	}
	if hm.HeatRangeAt(2000, 599*PS) == nil {
		t.Errorf("non-nil expected when requesting heat for the last address after the hole")
	}
	if hm.HeatRangeAt(2000, 600*PS) != nil {
		t.Errorf("nil expected when requesting heat after the last range")
	}

	// heat on each region after first non-overlapping counters
	hr0 := hm.HeatRangeAt(2000, 100*PS)
	if hr0.heat != 0.0 {
		t.Errorf("heat 0.0 expected at start address of the first range without accesses")
	}
	hr1 := hm.HeatRangeAt(2000, 150*PS)
	if hr1.heat != 0.0 {
		t.Errorf("heat 0.0 expected at start address of the second range without accesses")
	}
	hr2 := hm.HeatRangeAt(2000, 200*PS)
	if hr2.heat != 0.02 {
		t.Errorf("heat 0.02 expected at start address of the range with 1 access for in 50 pages")
	}
	hr3 := hm.HeatRangeAt(2000, 599*PS)
	if hr3.heat != 1.0 {
		t.Errorf("heat 1.0 expected at the last address of the range with 100 writes in 100 pages")
	}

	tcs1 := TrackerCounters{
		// There are four ranges and a hole. Add a region that overlaps with three:
		// now: [100..150][150..200][200..250]<hole>[500..600]
		// add:                [180.....................580]
		TrackerCounter{
			AR: NewAddrRanges(2000,
				AddrRange{180 * PS, 400}),
		},
	}

	timestamp = 1 * int64(time.Second)
	hm.UpdateFromCounters(&tcs1, timestamp)

	fmt.Println(hm.Dump())
}

func TestUpdateCountersOverlappingRanges(t *testing.T) {
	PS := constUPagesize
	sec := int64(time.Second)

	tcases := []struct {
		name        string
		config      string
		orig        TrackerCounters
		origT       int64
		update      TrackerCounters
		updateT     int64
		addrHeat    map[uint64]float64
		addrUpdated map[uint64]int64
	}{
		{
			name:        "disjoint ranges",
			orig:        TrackerCounters{{Accesses: 1, AR: NewAddrRanges(1000, AddrRange{100 * PS, 50})}},
			update:      TrackerCounters{{Accesses: 2, AR: NewAddrRanges(1000, AddrRange{10 * PS, 80})}},
			addrHeat:    map[uint64]float64{110 * PS: 0.02, 20 * PS: 0.025},
			addrUpdated: map[uint64]int64{110 * PS: 0, 20 * PS: sec},
		},
		{
			name:        "touching ranges at orig start",
			orig:        TrackerCounters{{Accesses: 1, AR: NewAddrRanges(1000, AddrRange{100 * PS, 50})}},
			update:      TrackerCounters{{Accesses: 2, AR: NewAddrRanges(1000, AddrRange{20 * PS, 80})}},
			addrHeat:    map[uint64]float64{100 * PS: 0.02, 99 * PS: 0.025},
			addrUpdated: map[uint64]int64{100 * PS: 0, 99 * PS: sec},
		},
		{
			name:        "touching ranges at orig end",
			orig:        TrackerCounters{{Writes: 1, AR: NewAddrRanges(1000, AddrRange{100 * PS, 50})}},
			update:      TrackerCounters{{Writes: 2, AR: NewAddrRanges(1000, AddrRange{150 * PS, 80})}},
			addrHeat:    map[uint64]float64{149 * PS: 0.02, 150 * PS: 0.025},
			addrUpdated: map[uint64]int64{149 * PS: 0, 150 * PS: sec},
		},
		{
			name:        "total eclipse, no cool down",
			config:      `{"HeatMax":1.0,"HeatRetention":1.0}`,
			orig:        TrackerCounters{{Writes: 1, AR: NewAddrRanges(1000, AddrRange{100 * PS, 50})}},
			update:      TrackerCounters{{Writes: 1, AR: NewAddrRanges(1000, AddrRange{100 * PS, 50})}},
			addrHeat:    map[uint64]float64{100 * PS: 0.04, 149 * PS: 0.04},
			addrUpdated: map[uint64]int64{100 * PS: sec, 149 * PS: sec},
		},
		{
			name:     "total eclipse, total cool down",
			config:   `{"HeatMax":1.0,"HeatRetention":0.0}`,
			orig:     TrackerCounters{{Writes: 1, AR: NewAddrRanges(1000, AddrRange{100 * PS, 50})}},
			update:   TrackerCounters{{Writes: 1, AR: NewAddrRanges(1000, AddrRange{100 * PS, 50})}},
			addrHeat: map[uint64]float64{100 * PS: 0.02, 149 * PS: 0.02},
		},
		{
			name:     "total eclipse, cool down to half in each second",
			config:   `{"HeatMax":1.0,"HeatRetention":0.5}`,
			orig:     TrackerCounters{{Writes: 1, AR: NewAddrRanges(1000, AddrRange{100 * PS, 50})}},
			update:   TrackerCounters{{Writes: 1, AR: NewAddrRanges(1000, AddrRange{100 * PS, 50})}},
			addrHeat: map[uint64]float64{100 * PS: 0.03, 149 * PS: 0.03},
		},
		{
			name:     "total eclipse, cool down to half in each second, hit max heat on both updates",
			config:   `{"HeatMax":1.0,"HeatRetention":0.5}`,
			orig:     TrackerCounters{{Writes: 200, AR: NewAddrRanges(1000, AddrRange{100 * PS, 50})}},
			update:   TrackerCounters{{Writes: 200, AR: NewAddrRanges(1000, AddrRange{100 * PS, 50})}},
			addrHeat: map[uint64]float64{100 * PS: 1.0, 149 * PS: 1.0},
		},
		{
			name:     "total eclipse, cool down to half in each second, 2s update interval",
			config:   `{"HeatMax":1.0,"HeatRetention":0.5}`,
			orig:     TrackerCounters{{Writes: 200, AR: NewAddrRanges(1000, AddrRange{100 * PS, 50})}},
			update:   TrackerCounters{{Writes: 1, AR: NewAddrRanges(1000, AddrRange{100 * PS, 50})}},
			updateT:  2 * sec,
			addrHeat: map[uint64]float64{100 * PS: 0.27, 149 * PS: 0.27},
		},
		{
			name:        "overlap at start",
			config:      `{"HeatMax":1.0,"HeatRetention":0.5}`,
			orig:        TrackerCounters{{Writes: 25, AR: NewAddrRanges(1000, AddrRange{100 * PS, 50})}},
			origT:       sec,
			update:      TrackerCounters{{Writes: 25, AR: NewAddrRanges(1000, AddrRange{75 * PS, 50})}},
			updateT:     2 * sec,
			addrHeat:    map[uint64]float64{75 * PS: 0.5, 100 * PS: 0.75, 125 * PS: 0.5},
			addrUpdated: map[uint64]int64{75 * PS: 2 * sec, 100 * PS: 2 * sec, 125 * PS: 1 * sec},
		},
		{
			name:        "overlap at end",
			config:      `{"HeatMax":1.0,"HeatRetention":0.5}`,
			orig:        TrackerCounters{{Writes: 25, AR: NewAddrRanges(1000, AddrRange{100 * PS, 50})}},
			origT:       sec,
			update:      TrackerCounters{{Writes: 25, AR: NewAddrRanges(1000, AddrRange{125 * PS, 50})}},
			updateT:     2 * sec,
			addrHeat:    map[uint64]float64{100 * PS: 0.5, 149 * PS: 0.75, 150 * PS: 0.5, 174 * PS: 0.5},
			addrUpdated: map[uint64]int64{100 * PS: sec, 124 * PS: sec, 125 * PS: 2 * sec, 150 * PS: 2 * sec, 174 * PS: 2 * sec},
		},
		{
			name:        "overlap middle, total cool down",
			config:      `{"HeatMax":1.0,"HeatRetention":0.0}`,
			orig:        TrackerCounters{{Writes: 25, AR: NewAddrRanges(1000, AddrRange{100 * PS, 50})}},
			origT:       2 * sec,
			update:      TrackerCounters{{Writes: 1, AR: NewAddrRanges(1000, AddrRange{110 * PS, 10})}},
			updateT:     4 * sec,
			addrHeat:    map[uint64]float64{100 * PS: 0.5, 110 * PS: 0.1, 119 * PS: 0.1, 120 * PS: 0.5},
			addrUpdated: map[uint64]int64{100 * PS: 2 * sec, 110 * PS: 4 * sec, 119 * PS: 4 * sec, 120 * PS: 2 * sec},
		},
		{
			name:        "overlap both sides, no cool down",
			config:      `{"HeatMax":1.0,"HeatRetention":1.0}`,
			orig:        TrackerCounters{{Writes: 25, AR: NewAddrRanges(1000, AddrRange{100 * PS, 50})}},
			origT:       1 * sec,
			update:      TrackerCounters{{Writes: 20, AR: NewAddrRanges(1000, AddrRange{0 * PS, 200})}},
			updateT:     3 * sec,
			addrHeat:    map[uint64]float64{0 * PS: 0.1, 99 * PS: 0.1, 100 * PS: 0.6, 149 * PS: 0.6, 150 * PS: 0.1, 199 * PS: 0.1},
			addrUpdated: map[uint64]int64{0 * PS: 3 * sec, 99 * PS: 3 * sec, 100 * PS: 3 * sec, 149 * PS: 3 * sec, 150 * PS: 3 * sec, 199 * PS: 3 * sec},
		},
		{
			name:   "overlap contiguous multirange",
			config: `{"HeatMax":1.0,"HeatRetention":0.5}`,
			orig: TrackerCounters{
				{Writes: 1, AR: NewAddrRanges(1000, AddrRange{50 * PS, 50})},     // heat 0.02
				{Writes: 1, AR: NewAddrRanges(1000, AddrRange{100 * PS, 100})},   // heat 0.01
				{Writes: 10, AR: NewAddrRanges(1000, AddrRange{200 * PS, 100})},  // heat 0.1
				{Writes: 100, AR: NewAddrRanges(1000, AddrRange{300 * PS, 100})}, // heat 1.0
				{Writes: 90, AR: NewAddrRanges(1000, AddrRange{400 * PS, 100})},  // heat 0.9
			},
			origT:       3 * sec,
			update:      TrackerCounters{{Writes: 20, AR: NewAddrRanges(1000, AddrRange{150 * PS, 200})}}, // heat: 0.1
			updateT:     4 * sec,
			addrHeat:    map[uint64]float64{50 * PS: 0.02, 100 * PS: 0.01, 150 * PS: 0.105, 200 * PS: 0.15, 300 * PS: 0.6, 350 * PS: 1.0, 400 * PS: 0.9},
			addrUpdated: map[uint64]int64{50 * PS: 3 * sec, 100 * PS: 3 * sec, 150 * PS: 4 * sec, 200 * PS: 4 * sec, 300 * PS: 4 * sec, 350 * PS: 3 * sec, 400 * PS: 3 * sec},
		},
		{
			name:   "overlap non-contiguous multirange",
			config: `{"HeatMax":1.0,"HeatRetention":0.5}`,
			orig: TrackerCounters{
				{Writes: 0, AR: NewAddrRanges(1000, AddrRange{50 * PS, 50})},  // heat 0.00
				{Writes: 1, AR: NewAddrRanges(1000, AddrRange{100 * PS, 50})}, // heat 0.02
				// hole 150..200
				{Writes: 40, AR: NewAddrRanges(1000, AddrRange{200 * PS, 50})}, // heat 0.8
				// hole 250..300
				{Writes: 4, AR: NewAddrRanges(1000, AddrRange{300 * PS, 40})}, // heat 0.1
				// hole 340..400
				{Writes: 90, AR: NewAddrRanges(1000, AddrRange{400 * PS, 100})}, // heat 0.9
			},
			origT:   3 * sec,
			update:  TrackerCounters{{Writes: 20, AR: NewAddrRanges(1000, AddrRange{120 * PS, 200})}}, // heat: 0.1
			updateT: 4 * sec,
			addrHeat: map[uint64]float64{50 * PS: 0.00, 100 * PS: 0.02, 119 * PS: 0.02, 120 * PS: 0.11, 149 * PS: 0.11, 150 * PS: 0.1, 199 * PS: 0.1,
				200 * PS: 0.5, 249 * PS: 0.5, 250 * PS: 0.1, 299 * PS: 0.1, 300 * PS: 0.15, 319 * PS: 0.15, 320 * PS: 0.1, 339 * PS: 0.1, 400 * PS: 0.9, 499 * PS: 0.9},
			addrUpdated: map[uint64]int64{50 * PS: 3 * sec, 100 * PS: 3 * sec, 119 * PS: 3 * sec, 120 * PS: 4 * sec, 149 * PS: 4 * sec, 150 * PS: 4 * sec, 199 * PS: 4 * sec,
				200 * PS: 4 * sec, 249 * PS: 4 * sec, 250 * PS: 4 * sec, 299 * PS: 4 * sec, 300 * PS: 4 * sec, 319 * PS: 4 * sec, 320 * PS: 3 * sec, 339 * PS: 3 * sec, 400 * PS: 3 * sec, 499 * PS: 3 * sec},
		},
		{
			name:   "overlapping counters on multirange",
			config: `{"HeatMax":1.0,"HeatRetention":0.5}`,
			orig: TrackerCounters{
				{Writes: 0, AR: NewAddrRanges(1000, AddrRange{50 * PS, 50})},  // heat 0.00
				{Writes: 1, AR: NewAddrRanges(1000, AddrRange{100 * PS, 50})}, // heat 0.02
				// hole 150..200
				{Writes: 40, AR: NewAddrRanges(1000, AddrRange{200 * PS, 50})}, // heat 0.8
				// hole 250..300
				{Writes: 4, AR: NewAddrRanges(1000, AddrRange{300 * PS, 40})}, // heat 0.1
			},
			origT: 3 * sec,
			update: TrackerCounters{
				{Writes: 0, AR: NewAddrRanges(1000, AddrRange{50 * PS, 100})},    // heat: 0.0
				{Writes: 10, AR: NewAddrRanges(1000, AddrRange{120 * PS, 100})}}, // heat: 0.1
			updateT:     4 * sec,
			addrHeat:    map[uint64]float64{50 * PS: 0.0, 100 * PS: 0.01, 120 * PS: 0.11, 150 * PS: 0.1, 199 * PS: 0.1, 200 * PS: 0.5, 219 * PS: 0.5},
			addrUpdated: map[uint64]int64{50 * PS: 4 * sec, 100 * PS: 4 * sec, 120 * PS: 4 * sec, 150 * PS: 4 * sec, 199 * PS: 4 * sec, 200 * PS: 4 * sec, 219 * PS: 4 * sec, 300 * PS: 3 * sec},
		},
	}
	isEqualEnough := func(f, g float64) bool {
		acceptableDelta := float64(0.000000000001)
		return f+acceptableDelta > g && f-acceptableDelta < g
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			hm := NewCounterHeatmap()
			if tc.config != "" {
				hm.SetConfigJson(tc.config)
			}
			timestamp := int64(tc.origT)
			hm.UpdateFromCounters(&tc.orig, timestamp)
			// If updateT is not defined, the default interval between updates is 1 s
			if tc.updateT == 0 {
				timestamp += sec - tc.origT
			} else {
				timestamp += tc.updateT - tc.origT
			}
			hm.UpdateFromCounters(&tc.update, timestamp)
			fmt.Printf("%s:\n%s\n\n", tc.name, hm.Dump())
			for addr, expHeat := range tc.addrHeat {
				obsHeat := hm.HeatRangeAt(1000, addr).heat
				if !isEqualEnough(obsHeat, expHeat) {
					t.Errorf("unexpected heat at %x: expected %f, observed %f", addr, expHeat, obsHeat)
				}
			}
			for addr, expUpdated := range tc.addrUpdated {
				obsUpdated := hm.HeatRangeAt(1000, addr).updated
				if obsUpdated != expUpdated {
					fmt.Printf("%s:\n%s\n\n", tc.name, hm.Dump())
					t.Errorf("unexpected updated at %x: expected %d, observed %d", addr, expUpdated, obsUpdated)
				}
			}
		})
	}
}
