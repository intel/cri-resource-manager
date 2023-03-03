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
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

type AddrRanges struct {
	pid   int
	addrs []AddrRange
}

type AddrRange struct {
	addr   uint64
	length uint64
}

func NewAddrRangeFromString(s string) (*AddrRange, error) {
	// Syntax:
	// STARTADDR[-ENDADDR]
	// STARTADDR[+SIZE[FACTOR]]
	var startAddr, endAddr uint64
	var err error
	switch {
	case strings.Contains(s, "-"):
		startEndSlice := strings.Split(s, "-")
		if len(startEndSlice) != 2 {
			return nil, fmt.Errorf("invalid START-END address range %q", s)
		}
		startAddr, err = strconv.ParseUint(startEndSlice[0], 16, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid start address %q", startEndSlice[0])
		}
		endAddr, err = strconv.ParseUint(startEndSlice[1], 16, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid end address %q", startEndSlice[1])
		}
	case strings.Contains(s, "+"):
		startSizeSlice := strings.Split(s, "+")
		if len(startSizeSlice) != 2 {
			return nil, fmt.Errorf("invalid START+SIZE address range %q", s)
		}
		startAddr, err = strconv.ParseUint(startSizeSlice[0], 16, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid start address %q", startSizeSlice[0])
		}
		size, err := parseMemBytes(startSizeSlice[1])
		if err != nil || size < 0 {
			return nil, fmt.Errorf("invalid size %q after address %q", startSizeSlice[1], startSizeSlice[0])
		}
		endAddr = startAddr + uint64(size)
	default:
		startAddr, err = strconv.ParseUint(s, 16, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid address %q", s)
		}
		endAddr = startAddr + constUPagesize
	}
	return NewAddrRange(startAddr, endAddr), nil
}

func NewAddrRange(startAddr, stopAddr uint64) *AddrRange {
	if stopAddr < startAddr {
		startAddr, stopAddr = stopAddr, startAddr
	}
	return &AddrRange{addr: startAddr, length: (stopAddr - startAddr) / constUPagesize}
}

func (r AddrRange) String() string {
	return fmt.Sprintf("%x-%x (%d B)", r.addr, r.addr+(r.length*constUPagesize), r.length*constUPagesize)
}

func (r *AddrRange) Addr() uint64 {
	return r.addr
}

func (r *AddrRange) EndAddr() uint64 {
	return r.addr + r.length*constUPagesize
}

func (r *AddrRange) Length() uint64 {
	return r.length
}

func (r *AddrRange) Equals(other *AddrRange) bool {
	return r.addr == other.addr && r.length == other.length
}

func NewAddrRanges(pid int, arParams ...AddrRange) *AddrRanges {
	ar := &AddrRanges{
		pid:   pid,
		addrs: make([]AddrRange, 0, len(arParams)),
	}
	for _, r := range arParams {
		ar.addrs = append(ar.addrs, r)
	}
	return ar
}

func (ar *AddrRanges) Pid() int {
	return ar.pid
}

func (ar *AddrRanges) Ranges() []AddrRange {
	return ar.addrs
}

func (ar *AddrRanges) String() string {
	rs := []string{}
	for _, r := range ar.addrs {
		rs = append(rs, r.String())
	}
	s := fmt.Sprintf("AddrRanges{pid=%d ranges=%s}",
		ar.pid, strings.Join(rs, ","))
	return s
}

// Flatten returns AddrRanges where each item includes only one address range
func (ar *AddrRanges) Flatten() []*AddrRanges {
	rv := []*AddrRanges{}
	for _, r := range ar.addrs {
		newAr := &AddrRanges{
			pid:   ar.pid,
			addrs: []AddrRange{r},
		}
		rv = append(rv, newAr)
	}
	return rv
}

func (ar *AddrRanges) Filter(accept func(ar AddrRange) bool) *AddrRanges {
	newAr := &AddrRanges{
		pid:   ar.pid,
		addrs: []AddrRange{},
	}
	for _, r := range ar.addrs {
		if accept(r) {
			newAr.addrs = append(newAr.addrs, r)
		}
	}
	return newAr
}

func (ar *AddrRanges) SplitLength(maxLength uint64) *AddrRanges {
	newAr := &AddrRanges{
		pid:   ar.pid,
		addrs: make([]AddrRange, 0, len(ar.addrs)),
	}
	for _, r := range ar.addrs {
		addr := r.addr
		length := r.length
		for length > maxLength {
			newAr.addrs = append(newAr.addrs, AddrRange{addr, maxLength})
			length -= maxLength
			addr += maxLength * constUPagesize
		}
		if length > 0 {
			newAr.addrs = append(newAr.addrs, AddrRange{addr, length})
		}
	}
	return newAr
}

func (ar *AddrRanges) SwapOut() error {
	return ar.ProcessMadvise(unix.MADV_PAGEOUT)
}

func (ar *AddrRanges) ProcessMadvise(advise int) error {
	pidfd, err := PidfdOpenSyscall(ar.Pid(), 0)
	if pidfd < 0 || err != nil {
		return fmt.Errorf("pidfd_open error: %s", err)
	}
	defer PidfdCloseSyscall(pidfd)
	sysRet, errno, err := ProcessMadviseSyscall(pidfd, ar.Ranges(), advise, 0)
	if stats != nil {
		stats.Store(StatsMadvised{
			pid:       ar.Pid(),
			sysRet:    sysRet,
			errno:     int(errno),
			advise:    advise,
			pageCount: ar.PageCount(),
		})
	}
	if err != nil {
		stats.Store(StatsHeartbeat{fmt.Sprintf("process_madvise(...) error: %s", err)})
		return fmt.Errorf("process_madvise error: %s", err)
	}
	return nil
}

func (ar *AddrRanges) PageCount() uint64 {
	pageCount := uint64(0)
	for _, r := range ar.Ranges() {
		pageCount += r.length
	}
	return pageCount
}

// PagesMatching returns pages with pagetable attributes.
func (ar *AddrRanges) PagesMatching(pageAttributes uint64) (*Pages, error) {
	pmFile, err := ProcPagemapOpen(ar.pid)
	if err != nil {
		return nil, err
	}
	defer pmFile.Close()

	pp := &Pages{pid: ar.pid, pages: []Page{}}

	err = pmFile.ForEachPage(ar.Ranges(), pageAttributes,
		func(pagemapBits, addr uint64) int {
			pp.pages = append(pp.pages, Page{addr: addr})
			return 0
		})

	if err != nil {
		return nil, err
	}

	return pp, nil
}

func (ar *AddrRanges) Intersection(intRanges []AddrRange) {
	newAddrs := []AddrRange{}
	for _, oldRange := range ar.addrs {
		for _, cutRange := range intRanges {
			start := oldRange.addr
			stop := oldRange.addr + oldRange.length*constUPagesize
			if cutRange.addr >= oldRange.addr &&
				cutRange.addr <= stop {
				if cutRange.addr > start {
					start = cutRange.addr
				}
				cutStop := cutRange.addr + cutRange.length*constUPagesize
				if cutStop < stop {
					stop = cutStop
				}
				if stop-start > 0 {
					newAddrs = append(newAddrs, *NewAddrRange(start, stop))
				}
			}
		}
	}
	ar.addrs = newAddrs
}

func parseMemBytes(s string) (int64, error) {
	factor := int64(1)
	// Syntax: INT[PREFIX[i][B]]
	if len(s) > 0 && s[len(s)-1] == 'B' {
		s = s[:len(s)-1]
	}
	if len(s) > 0 && s[len(s)-1] == 'i' {
		s = s[:len(s)-1]
	}
	if len(s) == 0 {
		return 0, fmt.Errorf("syntax error parsing bytes")
	}
	numpart := s[:len(s)-1]
	switch s[len(s)-1] {
	case 'k':
		factor = 1 << 10
	case 'M':
		factor = 1 << 20
	case 'G':
		factor = 1 << 30
	case 'T':
		factor = 1 << 40
	default:
		numpart = s
	}
	n, err := strconv.ParseInt(numpart, 10, 0)
	if err != nil {
		return n, err
	}
	return n * factor, nil
}
