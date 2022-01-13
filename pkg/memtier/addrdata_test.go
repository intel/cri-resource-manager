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
	"testing"
)

func TestOverwrite(t *testing.T) {
	ads := NewAddrDatas()
	PS := constUPagesize
	expectDataOk := func(addr uint64, expectedData int, expectedOk bool) {
		data, ok := ads.Data(addr)
		if ok != expectedOk {
			t.Errorf("Data(%x (page %d)): expected ok == %v, got %v", addr, addr/constUPagesize, expectedOk, ok)
			return
		}
		if ok {
			if data != expectedData {
				t.Errorf("Data(%x): expected data == %v, got %v",
					addr, expectedData, data)
				return
			}
		}

	}
	expectLen := func(expectedLen int) {
		if len(ads.ads) != expectedLen {
			t.Errorf("expected len == %v, got %v", expectedLen, len(ads.ads))
		}
	}
	AR := func(addr, length uint64) AddrRange {
		return AddrRange{addr, length}
	}
	// No data in the structure, must not crash.
	expectLen(0)
	expectDataOk(0, 0, false)
	expectDataOk(1000*PS, 0, false)

	// Single data block, 10 pages starting from the 10th page.
	ads.SetData(AR(10*PS, 10), 1)
	expectLen(1)
	expectDataOk(0, 0, false)
	expectDataOk(9*PS, 0, false)
	expectDataOk(10*PS-1, 0, false)
	expectDataOk(10*PS, 1, true)
	expectDataOk(10*PS+1, 1, true)
	expectDataOk(11*PS, 1, true)
	expectDataOk(19*PS, 1, true)
	expectDataOk(20*PS-1, 1, true)
	expectDataOk(20*PS, 0, false)
	expectDataOk(20*PS+1, 0, false)
	expectDataOk(21*PS, 0, false)

	// Add non-overlapping data, new first element
	ads.SetData(AR(5*PS, 2), 2)
	expectLen(2)
	expectDataOk(4*PS, 0, false)
	expectDataOk(5*PS, 2, true)
	expectDataOk(6*PS, 2, true)
	expectDataOk(7*PS, 0, false)
	expectDataOk(8*PS, 0, false)
	expectDataOk(9*PS, 0, false)
	expectDataOk(10*PS, 1, true)

	// Add non-overlapping data, new last element
	ads.SetData(AR(25*PS, 4), 3)
	expectLen(3)
	expectDataOk(19*PS, 1, true)
	expectDataOk(20*PS, 0, false)
	expectDataOk(21*PS, 0, false)
	expectDataOk(22*PS, 0, false)
	expectDataOk(23*PS, 0, false)
	expectDataOk(24*PS, 0, false)
	expectDataOk(25*PS, 3, true)
	expectDataOk(26*PS, 3, true)
	expectDataOk(27*PS, 3, true)
	expectDataOk(28*PS, 3, true)
	expectDataOk(29*PS, 0, false)

	// Overwrite data in the middle of existing element
	ads.SetData(AR(26*PS, 1), 4)
	expectLen(5)
	expectDataOk(24*PS, 0, false)
	expectDataOk(25*PS, 3, true)
	expectDataOk(26*PS, 4, true)
	expectDataOk(27*PS, 3, true)
	expectDataOk(28*PS, 3, true)
	expectDataOk(29*PS, 0, false)

	// Overwrite completely over an old element
	ads.SetData(AR(3*PS, 5), 5)
	expectLen(5)
	expectDataOk(2*PS, 0, false)
	expectDataOk(3*PS, 5, true)
	expectDataOk(4*PS, 5, true)
	expectDataOk(5*PS, 5, true)
	expectDataOk(6*PS, 5, true)
	expectDataOk(7*PS, 5, true)
	expectDataOk(8*PS, 0, false)
	expectDataOk(9*PS, 0, false)
	expectDataOk(10*PS, 1, true)

	// Fill a hole with exact match
	ads.SetData(AR(8*PS, 2), 6)
	expectLen(6)
	expectDataOk(7*PS, 5, true)
	expectDataOk(8*PS, 6, true)
	expectDataOk(9*PS, 6, true)
	expectDataOk(10*PS, 1, true)

	// Partial overwrite the beginning of existing data
	ads.SetData(AR(1*PS, 3), 7)
	expectLen(7)
	expectDataOk(0*PS, 0, false)
	expectDataOk(1*PS, 7, true)
	expectDataOk(2*PS, 7, true)
	expectDataOk(3*PS, 7, true)
	expectDataOk(4*PS, 5, true)

	// Partial overwrite the end of existing data
	ads.SetData(AR(28*PS, 2), 8)
	expectLen(8)
	expectDataOk(24*PS, 0, false)
	expectDataOk(25*PS, 3, true)
	expectDataOk(26*PS, 4, true)
	expectDataOk(27*PS, 3, true)
	expectDataOk(28*PS, 8, true)
	expectDataOk(29*PS, 8, true)
	expectDataOk(30*PS, 0, false)

	// Exact overwrite existing data
	ads.SetData(AR(8*PS, 2), 9)
	expectLen(8)
	expectDataOk(7*PS, 5, true)
	expectDataOk(8*PS, 9, true)
	expectDataOk(9*PS, 9, true)
	expectDataOk(10*PS, 1, true)

	// Exact overwrite many existing datas
	ads.SetData(AR(25*PS, 3), 10)
	expectLen(6) // remove 25:3, 26:4, 27:3, add 25-27:10
	expectDataOk(24*PS, 0, false)
	expectDataOk(25*PS, 10, true)
	expectDataOk(26*PS, 10, true)
	expectDataOk(27*PS, 10, true)
	expectDataOk(28*PS, 8, true)
	expectDataOk(29*PS, 8, true)
	expectDataOk(30*PS, 0, false)

	// Overlapping overwrite many existing datas
	ads.SetData(AR(24*PS, 5), 11)
	expectLen(6) // remove 25-27:10, add 24-28:11
	expectDataOk(23*PS, 0, false)
	expectDataOk(24*PS, 11, true)
	expectDataOk(25*PS, 11, true)
	expectDataOk(26*PS, 11, true)
	expectDataOk(27*PS, 11, true)
	expectDataOk(28*PS, 11, true)
	expectDataOk(29*PS, 8, true)
	expectDataOk(30*PS, 0, false)

	// Overwrite from the middle to the middle
	ads.SetData(AR(18*PS, 7), 12)
	expectLen(7)
	expectDataOk(17*PS, 1, true)
	expectDataOk(18*PS, 12, true)
	expectDataOk(19*PS, 12, true)
	expectDataOk(20*PS, 12, true)
	expectDataOk(21*PS, 12, true)
	expectDataOk(22*PS, 12, true)
	expectDataOk(23*PS, 12, true)
	expectDataOk(24*PS, 12, true)
	expectDataOk(25*PS, 11, true)

	// Overwrite everything but the first and the last datas
	ads.SetData(AR(2*PS, 27), 13)
	expectLen(3)
	expectDataOk(0*PS, 0, false)
	expectDataOk(1*PS, 7, true)
	expectDataOk(2*PS, 13, true)
	expectDataOk(27*PS, 13, true)
	expectDataOk(28*PS, 13, true)
	expectDataOk(29*PS, 8, true)

	// Overwrite everything
	ads.SetData(AR(1*PS, 29), 14)
	expectLen(1)
	expectDataOk(0*PS, 0, false)
	expectDataOk(1*PS, 14, true)
	expectDataOk(28*PS, 14, true)
	expectDataOk(29*PS, 14, true)
	expectDataOk(30*PS, 0, false)
}
