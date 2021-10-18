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
	"encoding/binary"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
)

// procPagemap returns pages of a process from address ranges
func procPagemap(pid int, addressRanges []AddrRange, pageAttributes uint64) ([]Page, error) {
	pageMustBePresent := (pageAttributes&PagePresent == PagePresent)
	pageMustBeExclusive := (pageAttributes&PageExclusive == PageExclusive)
	pageMustBeDirty := (pageAttributes&PageDirty == PageDirty)
	pageMustNotBeDirty := (pageAttributes&PageNotDirty == PageNotDirty)
	softDirtyBit := uint64(0x1) << 55
	exclusiveBit := uint64(0x1) << 56
	presentBit := uint64(0x1) << 63

	pages := make([]Page, 0)
	path := "/proc/" + strconv.Itoa(pid) + "/pagemap"
	pageMap, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	for _, addressRange := range addressRanges {
		idx := int64(addressRange.addr / uint64(constPagesize) * 8)
		_, err := pageMap.Seek(idx, io.SeekStart)
		if err != nil {
			// Maybe there was a race condition and the maps changed?
			// log.Error("Failed to seek: %v\n", err)
			continue
		}
		readBuf := make([]byte, 8*constPagesize) // read from pagemap, chunks of len(readBuf)
		readData := readBuf[0:0]                 // valid data in readBuf
		for i := uint64(0); i < addressRange.length; i++ {
			if len(readData) == 0 {
				unreadByteCount := 8 * int(addressRange.length-i)
				fillBufUpTo := cap(readBuf)
				if fillBufUpTo > unreadByteCount {
					fillBufUpTo = unreadByteCount
				}
				_, err = io.ReadAtLeast(pageMap, readBuf, fillBufUpTo)
				if err != nil {
					// cannot read address range
					continue
				}
				readData = readBuf[:fillBufUpTo]
			}
			bytes := readData[:8]
			readData = readData[8:]
			data := binary.LittleEndian.Uint64(bytes)

			// Check that the page is present (not swapped), exclusively
			// mapped (not used by any other process), and it has the
			// soft-dirty bit off.

			// Note: there appears to be no way to see from the pagemap entry what the NUMA node is.
			// We could map this back to the physical address ranges if needed. Currently this is handled
			// in movePages() by calling move_pages() first with an empty node array.

			present := (data&presentBit == presentBit)
			exclusive := (data&exclusiveBit == exclusiveBit)
			softDirty := (data&softDirtyBit == softDirtyBit)

			if (!pageMustBePresent || present) &&
				(!pageMustBeExclusive || exclusive) &&
				(!pageMustBeDirty || softDirty) &&
				(!pageMustNotBeDirty || !softDirty) {
				pages = append(pages, Page{addr: addressRange.addr + i*uint64(constPagesize)})
			}
		}
	}
	return pages, nil
}

// procMaps returns address ranges of a process
func procMaps(pid int) ([]AddrRange, error) {
	pageCanBeInAnonymous := true
	pageCanBeInHeap := true

	addressRanges := make([]AddrRange, 0)
	sPid := strconv.Itoa(pid)

	// Read /proc/pid/numa_maps
	numaMapsPath := "/proc/" + sPid + "/numa_maps"
	numaMapsBytes, err := ioutil.ReadFile(numaMapsPath)
	if err != nil {
		return nil, err
	}
	numaMapsLines := strings.Split(string(numaMapsBytes), "\n")

	// Read /proc/pid/maps
	mapsPath := "/proc/" + sPid + "/maps"
	mapsBytes, err := ioutil.ReadFile(mapsPath)
	if err != nil {
		return nil, err
	}
	mapsLines := strings.Split(string(mapsBytes), "\n")

	allAddressRanges := make(map[uint64]AddrRange, len(numaMapsLines))
	for _, mapLine := range mapsLines {
		// Parse start and end addresses. Example of /proc/pid/maps lines:
		// 55d74cf13000-55d74cf14000 rw-p 00003000 fe:03 1194719   /usr/bin/python3.8
		// 55d74e76d000-55d74e968000 rw-p 00000000 00:00 0         [heap]
		// 7f3bcfe69000-7f3c4fe6a000 rw-p 00000000 00:00 0
		dashIndex := strings.Index(mapLine, "-")
		spaceIndex := strings.Index(mapLine, " ")
		if dashIndex > 0 && spaceIndex > dashIndex {
			startAddr, err := strconv.ParseUint(mapLine[0:dashIndex], 16, 64)
			if err != nil {
				continue
			}
			endAddr, err := strconv.ParseUint(mapLine[dashIndex+1:spaceIndex], 16, 64)
			if err != nil || endAddr < startAddr {
				continue
			}
			rangeLength := endAddr - startAddr
			allAddressRanges[startAddr] = AddrRange{startAddr, rangeLength / uint64(constPagesize)}
		}
	}

	for _, line := range numaMapsLines {
		// Example of /proc/pid/numa_maps:
		// 55d74cf13000 default file=/usr/bin/python3.8 anon=1 dirty=1 active=0 N0=1 kernelpagesize_kB=4
		// 55d74e76d000 default heap anon=471 dirty=471 active=0 N0=471 kernelpagesize_kB=4
		// 7f3bcfe69000 default anon=524289 dirty=524289 active=0 N0=257944 N1=266345 kernelpagesize_kB=4
		tokens := strings.Split(line, " ")
		if len(tokens) < 3 {
			continue
		}
		attrs := strings.Join(tokens[2:], " ")

		// Filter out lines which don't have "anonymous", since we are not
		// interested in file-mapped or shared pages. Save the interesting ranges.
		// TODO: consider dropping the "heap" requirement. There are often ranges
		// in the file which don't have any attributes indicating the memory
		// location.
		// TODO: rather than filtering here, consider parsing properties
		// (like on which nodes pages in the range are located, heap/dirty/active...)
		// to AddrRange{} structs so that they can be filtered later on
		// for instance ar.IsDirty().OnNodes(2, 3)
		if !(pageCanBeInHeap && strings.Contains(attrs, "heap") ||
			pageCanBeInAnonymous && strings.Contains(attrs, "anon=")) {
			continue
		}
		startAddr, err := strconv.ParseUint(tokens[0], 16, 64)
		if err != nil {
			continue
		}
		if ar, ok := allAddressRanges[startAddr]; ok {
			addressRanges = append(addressRanges, ar)
		}
	}
	return addressRanges, nil
}
