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
)

func TestUpdateCounters(t *testing.T) {
	hm := NewCounterHeatmap()
	PS := uint64(constPagesize)
	tcs := TrackerCounters{
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
	hm.UpdateFromCounters(&tcs, timestamp)

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

	hr0 := hm.HeatRangeAt(2000, 100*PS)
	if hr0.heat != 0.0 {
		t.Errorf("heat 0.0 expected at start address of the first range without accesses")
	}

}
