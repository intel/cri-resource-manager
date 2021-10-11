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

func NewAddrRange(startAddr, stopAddr uint64) *AddrRange {
	if stopAddr < startAddr {
		startAddr, stopAddr = stopAddr, startAddr
	}
	return &AddrRange{addr: startAddr, length: (stopAddr - startAddr) / uint64(constPagesize)}
}

func (ar *AddrRanges) Ranges() []AddrRange {
	return ar.addrs
}

// PagesMatching returns pages with selected pagetable attributes
func (ar *AddrRanges) PagesMatching(pageAttributes uint64) (*Pages, error) {
	pages, err := procPagemap(ar.pid, ar.addrs, pageAttributes)
	if err != nil {
		return nil, err
	}
	return &Pages{pid: ar.pid, pages: pages}, nil
}

func (ar *AddrRanges) Intersection(intRanges []AddrRange) {
	newAddrs := []AddrRange{}
	for _, oldRange := range ar.addrs {
		for _, cutRange := range intRanges {
			start := oldRange.addr
			stop := oldRange.addr + oldRange.length*uint64(constPagesize)
			if cutRange.addr >= oldRange.addr &&
				cutRange.addr <= stop {
				if cutRange.addr > start {
					start = cutRange.addr
				}
				cutStop := cutRange.addr + cutRange.length*uint64(constPagesize)
				if cutStop < stop {
					stop = cutStop
				}
				newAddrs = append(newAddrs, *NewAddrRange(start, stop))
			}
		}
	}
	ar.addrs = newAddrs
}
