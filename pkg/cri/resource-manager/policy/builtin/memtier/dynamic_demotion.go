// Copyright 2020 Intel Corporation. All Rights Reserved.
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
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/events"
	system "github.com/intel/cri-resource-manager/pkg/sysfs"
	"github.com/intel/cri-resource-manager/pkg/utils"
)

// Support dynamic pushing of unused pages from DRAM to PMEM.
//
// The algorithm is be (roughly) this:
//
// Find out which processes belong to the container. For every process in the
// container, find out which pages the process uses. Using move_pages(), push a
// number of pages not in the working set, which are present in DRAM, from DRAM
// to PMEM. This may need to be done for many times with a delay in between,
// because the process will be "stuck" when the pages are moved. Repeat this
// process.
//
// How to figure out which pages are not part of the working set:
//
// 1. Clear soft-dirty bits on the PTEs:
//    https://www.kernel.org/doc/html/latest/admin-guide/mm/soft-dirty.html
// 2. Wait for a while.
// 3. Read out the process page maps:
//    https://www.kernel.org/doc/html/latest/admin-guide/mm/pagemap.html The pages
//    which don't have the soft-dirty bit are considered to be outside of the
//    working set.

type page struct {
	pid  int
	addr uint64
}

type addrRange struct {
	addr   uint64
	length uint64
}

type demoter struct {
	policy *policy // Policy backpointer, needed for sending events

	// Finding pages
	dirtyBitReset    time.Ticker      // Ticker for resetting the dirty bits.
	dirtyBitStop     chan interface{} // Channel for stopping the ticker.
	dirtyBitDuration time.Duration    // How often should we analyze the working set size.

	// Moving pages
	pageMover         PageMover
	containerDemoters map[string]chan interface{} // Channel for sending pagemap updates to demoters.
	pageMoveDuration  time.Duration               // How often should we move pages for a container.
	pageMoveCount     uint                        // How many pages to move at once.
}

// Demoter dynamically demotes pages from DRAM to PMEM.
type Demoter interface {
	// StartDirtyBitResetTimer starts the timer for getting memory moving events.
	StartDirtyBitResetTimer(policy *policy, timeout time.Duration)
	// StopDirtyBitResetTimer stops the memory moving.
	StopDirtyBitResetTimer()
	// ResetDirtyBit resets soft-dirty bits for all processes in container c.
	ResetDirtyBit(c cache.Container) error
	// GetPagesForContainer gets pages which could be potentially moved from container c.
	GetPagesForContainer(c cache.Container, sourceNodes system.IDSet) (pagePool, error)
	// MovePages moves at most 'count' pages in page pool to a memory node.
	MovePages(p pagePool, count uint, targetNodes system.IDSet) error

	UpdateDemoter(cid string, p pagePool, targetNodes system.IDSet)
	StopDemoter(cid string)
	UnusedDemoters(cs []cache.Container) []string
}

type pagePool struct {
	pages        map[int][]page
	longestRange uint
}

type demotion struct {
	pagePool    pagePool
	targetNodes system.IDSet
}

func copyPagePool(p pagePool) pagePool {
	c := pagePool{
		longestRange: p.longestRange,
		pages:        make(map[int][]page, 0),
	}
	for pid, pages := range p.pages {
		c.pages[pid] = make([]page, len(pages))
		copy(c.pages[pid], pages)
	}
	return c
}

func (d *demoter) UpdateDemoter(cid string, p pagePool, targetNodes system.IDSet) {
	channel, found := d.containerDemoters[cid]
	if !found {
		channel := make(chan interface{})
		go func() {
			moveTimer := time.NewTicker(d.pageMoveDuration)
			moveTimerChan := moveTimer.C
			pagePool := p
			nodes := targetNodes
			count := d.pageMoveCount
			for {
				select {
				case msg := <-channel:
					demotion, ok := msg.(demotion)
					if ok {
						pagePool = demotion.pagePool
						targetNodes = demotion.targetNodes
						if p.longestRange > d.pageMoveCount {
							// The number of pages moved needs to be at least as large as a range in numa_maps
							// file so that we know that all pages will be moved (even if some of them were
							// already on the PMEM node).

							// TODO: adjust the timer if we have a larger-than-usual range of pages to move.
							count = p.longestRange
						} else {
							count = d.pageMoveCount
						}
					} else {
						// A stop request.
						if moveTimer != nil {
							moveTimer.Stop()
						}
						return
					}
				case _ = <-moveTimerChan:
					err := d.MovePages(pagePool, count, nodes)
					if err != nil {
						log.Error("Error demoting pages: %s", err)
					}
				}
			}
		}()
		d.containerDemoters[cid] = channel
		// TODO: trigger instant update when run the first time?
	} else {
		channel <- demotion{pagePool: p, targetNodes: targetNodes}
	}
}

func (d *demoter) StopDemoter(cid string) {
	channel, found := d.containerDemoters[cid]
	if found {
		channel <- "stop"
		delete(d.containerDemoters, cid)
	}
}

func (d *demoter) UnusedDemoters(cs []cache.Container) []string {
	unused := make([]string, 0)
	for key := range d.containerDemoters {
		found := false
		for _, c := range cs {
			if c.GetCacheID() == key {
				found = true
				break
			}
		}
		if !found {
			unused = append(unused, key)
		}
	}
	return unused
}

func (d *demoter) StopDirtyBitResetTimer() {
	if d.dirtyBitStop != nil {
		close(d.dirtyBitStop)
		d.dirtyBitStop = nil
	}
}

func (d *demoter) StartDirtyBitResetTimer(policy *policy, timeout time.Duration) {
	if d.dirtyBitStop != nil {
		return
	}

	d.policy = policy

	stop := make(chan interface{})
	go func() {
		dirtyBitResetTimer := time.NewTicker(timeout)
		dirtyBitResetChan := dirtyBitResetTimer.C
		for {
			select {
			case _ = <-stop:
				if dirtyBitResetTimer != nil {
					dirtyBitResetTimer.Stop()
				}
				return
			case _ = <-dirtyBitResetChan:
				e := &events.Policy{
					Type:   DirtyBitReset,
					Source: PolicyName,
					Data:   "tick",
				}
				if err := d.policy.options.SendEvent(e); err != nil {
					log.Error("Failed to send event.")
				}
			}
		}
	}()
	d.dirtyBitStop = stop
}

func resetDirtyBit(pid string) error {
	// Write magic value "4" to the clear_refs file. This resets the dirty bit.
	path := "/proc/" + pid + "/clear_refs"
	err := ioutil.WriteFile(path, []byte("4"), 0600)
	return err
}

// ResetDirtyBit unsets soft-dirty bits for all processes in a container.
func (d *demoter) ResetDirtyBit(c cache.Container) error {
	parentDir := ""
	pod, isPod := c.GetPod()
	if isPod {
		parentDir = pod.GetCgroupParentDir()
	}

	pids, err := utils.GetProcessesInContainer(parentDir, c.GetID())
	if err != nil {
		return err
	}

	for _, pid := range pids {
		err = resetDirtyBit(pid)
		if err != nil {
			log.Error("Failed to reset dirty bit for process %s: %v", pid, err)
			return err
		}
	}
	return nil
}

func (d *demoter) GetPagesForContainer(c cache.Container, sourceNodes system.IDSet) (pagePool, error) {
	pool := pagePool{
		pages:        make(map[int][]page, 0),
		longestRange: 0,
	}
	parentDir := ""
	pod, isPod := c.GetPod()
	if isPod {
		parentDir = pod.GetCgroupParentDir()
	}

	pids, err := utils.GetProcessesInContainer(parentDir, c.GetID())
	if err != nil {
		return pagePool{}, err
	}

	for _, pid := range pids {
		addressRanges := make([]addrRange, 0)
		pidNumber64, err := strconv.ParseInt(pid, 10, 32)
		if err != nil {
			log.Error("Failed to parse addr to int: %v", err)
			continue
		}
		pidNumber := int(pidNumber64)
		// Read /proc/pid/numa_maps and /proc/pid/maps
		numaMapsPath := "/proc/" + pid + "/numa_maps"
		numaMapsBytes, err := ioutil.ReadFile(numaMapsPath)
		if err != nil {
			log.Error("Could not read numa_maps: %v", err)
			continue
		}
		mapsPath := "/proc/" + pid + "/maps"
		mapsBytes, err := ioutil.ReadFile(mapsPath)
		if err != nil {
			log.Error("Could not read maps: %v\n", err)
			os.Exit(1)
		}
		mapsLines := strings.Split(string(mapsBytes), "\n")

		for _, line := range strings.Split(string(numaMapsBytes), "\n") {
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
			if !strings.Contains(attrs, "heap") || !strings.Contains(attrs, "anon=") {
				continue
			}
			// We only find out if *any* pages in the range are in a DRAM node. The
			// more fine-grained analysis is done later by running the move_pages()
			// system call twice.
			locatedOnDRAMNode := false
			for node := range sourceNodes {
				number := strconv.FormatInt(int64(node), 10)
				str := "N" + number + "="
				if strings.Contains(attrs, str) {
					locatedOnDRAMNode = true
					break
				}
			}
			if !locatedOnDRAMNode {
				continue
			}

			for _, mapLine := range mapsLines {
				if strings.HasPrefix(mapLine, tokens[0]+"-") {
					spaceIndex := strings.Index(mapLine, " ")
					if spaceIndex > len(tokens[0]+"-") {
						endAddrStr := mapLine[len(tokens[0]+"-"):spaceIndex]
						startAddr, err := strconv.ParseInt(tokens[0], 16, 64)
						if err != nil {
							log.Error("Failed to parse addr to int: %v\n", err)
							break
						}
						endAddr, err := strconv.ParseInt(endAddrStr, 16, 64)
						if err != nil {
							log.Error("Failed to parse addr to int: %v\n", err)
							break
						}
						rangeLength := endAddr - startAddr
						addressRanges = append(addressRanges, addrRange{uint64(startAddr), uint64(rangeLength / int64(os.Getpagesize()))})
						// log.Debug("found interesting page range for pid %s: %v", pid, addressRanges[len(addressRanges)-1])
						break
					}
				}
			}
		}

		// Read /proc/pid/pagemap and process only interesting page ranges. For
		// every read-only page and for every page with the soft-dirty bit on, mark
		// them as candidates to be moved by adding them to pagePool.

		if len(addressRanges) > 0 {
			// log.Debug("Getting pages for PID %s for ranges %v", pid, addressRanges)
			pages := make([]page, 0)
			path := "/proc/" + pid + "/pagemap"
			pageMap, err := os.OpenFile(path, os.O_RDONLY, 0)
			if err != nil {
				// Probably the process just died?
				fmt.Printf("Could not read pagemaps: %v\n", err)
				break
			}
			for _, addressRange := range addressRanges {
				idx := int64(addressRange.addr / uint64(os.Getpagesize()) * 8)
				offset, err := pageMap.Seek(idx, io.SeekStart)
				if err != nil {
					// Maybe there was a race condition and the maps changed?
					log.Error("Failed to seek: %v\n", err)
					continue
				}
				for i := uint64(0); i < addressRange.length; i++ {
					bytes := make([]byte, 8)
					// Read exactly 8 bytes (because the file interface breaks otherwise).
					_, err = io.ReadAtLeast(pageMap, bytes, 8)
					if err != nil {
						// Possibly the maps changed.
						log.Error("Could not read data from pagemaps(%v)(page size: %d, seek offset: %d): %v\n", idx, os.Getpagesize(), offset, err)
						break
					}
					data := binary.LittleEndian.Uint64(bytes)

					// Check that the page is present (not swapped), exclusively
					// mapped (not used by any other process), and it has the
					// soft-dirty bit off.

					// Note: there appears to be no way to see from the pagemap entry what the NUMA node is.
					// We could map this back to the physical address ranges if needed. Currently this is handled
					// in MovePages() by calling move_pages() first with an empty node array.

					softDirtyBit := uint64(0x1) << 55
					exclusiveBit := uint64(0x1) << 56
					presentBit := uint64(0x1) << 63
					present := (data&presentBit == presentBit)
					exclusive := (data&exclusiveBit == exclusiveBit)
					softDirty := (data&softDirtyBit == softDirtyBit)

					if present && exclusive && !softDirty {
						// log.Debug("page a candidate for moving: 0x%08x", addressRange.addr+i*uint64(os.Getpagesize()))
						pages = append(pages, page{addr: addressRange.addr + i*uint64(os.Getpagesize()), pid: pidNumber})
					}
				}
			}
			if _, found := pool.pages[pidNumber]; found {
				pool.pages[pidNumber] = append(pool.pages[pidNumber], pages...)
			} else {
				pool.pages[pidNumber] = pages
			}
			if uint(len(addressRanges)) > pool.longestRange {
				pool.longestRange = uint(len(addressRanges))
			}
		}
	}

	return pool, nil
}

func pickClosestPMEMNode(currentNode system.ID, targetNodes system.IDSet) system.ID {
	// TODO: analyze the topology information (and possibly the amount of free memory) and choose the "best"
	// PMEM node to demote the page to. The array targetNodes already contains only the subset of PMEM nodes
	// available in this topology subtree. Right now just pick a random controller.
	nodes := targetNodes.Members()
	return nodes[rand.Intn(len(nodes))]
}

func (d *demoter) movePagesForPid(p []page, count uint, pid int, targetNodes system.IDSet) (uint, error) {
	// We move at max count pages, but there might not be that much.
	nPages := count
	if uint(len(p)) < count {
		nPages = uint(len(p))
	}

	// Gather memory page pointers.
	pages := make([]uintptr, nPages)
	var i uint
	for i = 0; i < nPages; i++ {
		pages[i] = uintptr(p[i].addr)
	}

	// MPOL_MF_MOVE - only move pages exclusive to this process. There will be
	// permission denied errors for pages which couldn't be moved. FIXME: find
	// out if the whole move_pages() syscall failed or if just the non-exclusive
	// pages were not moved.
	flags := 1 << 1

	// Call move_pages() first with nil nodes array to find out the current controllers.
	_, currentStatus, err := d.pageMover.MovePagesSyscall(pid, nPages, pages, nil, flags)
	if err != nil {
		log.Error("Failed to find out the current status of the pages: %v.", err)
		return 0, err
	}

	dramPages := make([]uintptr, 0)
	nodes := make([]int, 0)
	// Choose a target node for every page. Drop the pages which already are on the right controller from the list.
	for i, pageStatus := range currentStatus {
		if pageStatus < 0 {
			// There was an error regarding this page.
			continue
		}
		// log.Debug("page 0x%08X: old status %d", pages[i], pageStatus)
		if !targetNodes.Has(system.ID(pageStatus)) {
			// In case of many PMEM controllers choose the one that is the closest.
			dramPages = append(dramPages, pages[i])
			nodes = append(nodes, int(pickClosestPMEMNode(system.ID(pageStatus), targetNodes)))
		} // else no need to move.
	}

	// Call move_pages() to actually move the pages.
	_, _, err = d.pageMover.MovePagesSyscall(pid, uint(len(dramPages)), dramPages, nodes, flags)

	// We processed (moved or ignored) at least nPages.
	return nPages, err
}

func (d *demoter) MovePages(p pagePool, count uint, targetNodes system.IDSet) error {
	// Select pid for moving the pages so that the process with the largest number
	// of non-dirty pages gets the pages moved first.
	processedPids := make(map[int]bool, 0)

	for count > 0 {
		mostPagesPid := 0
		nPagesForPid := uint(0)
		for pid, pages := range p.pages {
			_, alreadyProcessed := processedPids[pid]
			if alreadyProcessed {
				continue
			}
			if uint(len(pages)) > nPagesForPid {
				mostPagesPid = pid
				nPagesForPid = uint(len(pages))
			}
		}

		if nPagesForPid == 0 {
			return nil
		}

		processedPids[mostPagesPid] = true

		nMovePages := nPagesForPid
		if count < nPagesForPid {
			nMovePages = count
			count = 0
		} else {
			count -= nPagesForPid
		}

		log.Debug("moving %d pages for pid %d", nMovePages, mostPagesPid)
		nPages, err := d.movePagesForPid(p.pages[mostPagesPid], nMovePages, mostPagesPid, targetNodes)
		if err != nil {
			log.Error("Failed to move pages: %v", err)
			return err
		}
		// Remove processed pages from the pagemap.
		p.pages[mostPagesPid] = p.pages[mostPagesPid][nPages:]
	}
	return nil
}
