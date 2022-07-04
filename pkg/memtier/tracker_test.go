// Copyright 2022 Intel Corporation. All Rights Reserved.
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

func TestFlattened(t *testing.T) {
	cut := func(tc0 TrackerCounter, ar *AddrRange) TrackerCounter {
		startAddr := tc0.AR.Ranges()[0].Addr()
		if ar.Addr() > startAddr {
			startAddr = ar.Addr()
		}
		endAddr := tc0.AR.Ranges()[0].EndAddr()
		if ar.EndAddr() < endAddr {
			endAddr = ar.EndAddr()
		}
		return TrackerCounter{
			Accesses: tc0.Accesses,
			Reads:    tc0.Reads,
			Writes:   tc0.Writes,
			AR:       NewAddrRanges(tc0.AR.Pid(), *NewAddrRange(startAddr, endAddr)),
		}
	}
	union := func(tc0, tc1 TrackerCounter) TrackerCounter {
		if tc0.AR.Pid() != tc1.AR.Pid() {
			t.Errorf("trying to union counters of different processes")
		}
		return TrackerCounter{
			Accesses: tc0.Accesses + tc1.Accesses,
			Reads:    tc0.Reads + tc1.Reads,
			Writes:   tc0.Writes + tc1.Writes,
			AR:       NewAddrRanges(tc0.AR.Pid(), tc0.AR.Ranges()...),
		}
	}
	tcases := []struct {
		name     string
		input    TrackerCounters
		expected TrackerCounters
	}{
		{
			name:     "empty tcs",
			input:    TrackerCounters{},
			expected: TrackerCounters{},
		}, {
			name:     "single entry",
			input:    TrackerCounters{{Accesses: 1, Reads: 2, Writes: 3, AR: NewAddrRanges(4, AddrRange{5, 6})}},
			expected: TrackerCounters{{Accesses: 1, Reads: 2, Writes: 3, AR: NewAddrRanges(4, AddrRange{5, 6})}},
		}, {
			name: "non overlapping entries",
			input: TrackerCounters{
				{Accesses: 1, Reads: 2, Writes: 3, AR: NewAddrRanges(4, AddrRange{5 * constUPagesize, 6})},
				{Accesses: 10, Reads: 20, Writes: 30, AR: NewAddrRanges(4, AddrRange{11 * constUPagesize, 60})},
			},
			expected: TrackerCounters{
				{Accesses: 1, Reads: 2, Writes: 3, AR: NewAddrRanges(4, AddrRange{5 * constUPagesize, 6})},
				{Accesses: 10, Reads: 20, Writes: 30, AR: NewAddrRanges(4, AddrRange{11 * constUPagesize, 60})},
			},
		}, {
			name: "overlapping addresses, different processes",
			input: TrackerCounters{
				{Accesses: 1, Reads: 2, Writes: 3, AR: NewAddrRanges(4, AddrRange{5 * constUPagesize, 6})},
				{Accesses: 10, Reads: 20, Writes: 30, AR: NewAddrRanges(40, AddrRange{10 * constUPagesize, 60})},
			},
			expected: TrackerCounters{
				{Accesses: 1, Reads: 2, Writes: 3, AR: NewAddrRanges(4, AddrRange{5 * constUPagesize, 6})},
				{Accesses: 10, Reads: 20, Writes: 30, AR: NewAddrRanges(40, AddrRange{10 * constUPagesize, 60})},
			},
		}, {
			name: "overlapping addresses",
			input: TrackerCounters{
				{Accesses: 1, Reads: 2, Writes: 3, AR: NewAddrRanges(4, AddrRange{1 * constUPagesize, 2})},
				{Accesses: 11, Reads: 12, Writes: 13, AR: NewAddrRanges(4, AddrRange{5 * constUPagesize, 6})},
				{Accesses: 21, Reads: 22, Writes: 23, AR: NewAddrRanges(4, AddrRange{10 * constUPagesize, 60})},
			},
			expected: TrackerCounters{
				{Accesses: 1, Reads: 2, Writes: 3, AR: NewAddrRanges(4, AddrRange{1 * constUPagesize, 2})},
				{Accesses: 11, Reads: 12, Writes: 13, AR: NewAddrRanges(4, AddrRange{5 * constUPagesize, 5})},
				{Accesses: 32, Reads: 34, Writes: 36, AR: NewAddrRanges(4, AddrRange{10 * constUPagesize, 1})},
				{Accesses: 21, Reads: 22, Writes: 23, AR: NewAddrRanges(4, AddrRange{11 * constUPagesize, 59})},
			},
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			output := tc.input.Flattened(cut, union)
			if len(tc.expected) != len(*output) {
				t.Errorf("expected and observed item counts differ:\n%v\n    vs\n%v)", &tc.expected, output)
			}
			for i := range *output {
				etc := tc.expected[i]
				otc := (*output)[i]
				if etc.Accesses != otc.Accesses ||
					etc.Reads != otc.Reads ||
					etc.Writes != otc.Writes {
					t.Errorf("item %d access/read/write expected: %v, observed: %v", i, etc, otc)
				}
				if etc.AR.Pid() != otc.AR.Pid() ||
					len(etc.AR.Ranges()) != len(otc.AR.Ranges()) {
					t.Errorf("item %d pid/len(ranges) expected: %d/%d, observed %d/%d",
						i,
						etc.AR.Pid(), len(etc.AR.Ranges()),
						otc.AR.Pid(), len(otc.AR.Ranges()))
				}
				for r := range etc.AR.Ranges() {
					if etc.AR.Ranges()[r].addr != otc.AR.Ranges()[r].addr ||
						etc.AR.Ranges()[r].length != otc.AR.Ranges()[r].length {
						t.Errorf("item %d address ranges expected: %v, observed %v",
							i, etc.AR.Ranges(), otc.AR.Ranges())
					}
				}

			}
		})
	}
}
