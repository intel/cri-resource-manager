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

package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/intel/cri-resource-manager/pkg/memtier"
)

func exit(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, fmt.Sprintf("memtierd: "+format+"\n", a...))
	os.Exit(1)
}

func parseOptPages(pagesStr string) (uint64, error) {
	if pagesStr == "" {
		return (memtier.PagePresent |
			memtier.PageExclusive |
			memtier.PageDirty), nil
	}
	var pageAttributes uint64 = 0
	for _, pageAttrStr := range strings.Split(pagesStr, ",") {
		switch pageAttrStr {
		case "Present":
			pageAttributes |= memtier.PagePresent
		case "Exclusive":
			pageAttributes |= memtier.PageExclusive
		case "Dirty":
			pageAttributes |= memtier.PageDirty
		case "NotDirty":
			pageAttributes |= memtier.PageNotDirty
		default:
			return 0, fmt.Errorf("invalid -page: %q", pageAttrStr)
		}
		if pageAttributes&memtier.PageDirty == memtier.PageDirty &&
			pageAttributes&memtier.PageNotDirty == memtier.PageNotDirty {
			return 0, fmt.Errorf("contradicting page requirements: Dirty,NotDirty")
		}
	}
	return pageAttributes, nil
}

func parseOptRanges(rangeStr string) []memtier.AddrRange {
	addrRanges := []memtier.AddrRange{}
	for _, startStopStr := range strings.Split(rangeStr, ",") {
		startStopSlice := strings.Split(startStopStr, "-")
		if len(startStopSlice) != 2 {
			exit("invalid addresss range %q, expected STARTADDR-STOPADDR")
		}
		startAddr, err := strconv.ParseUint(startStopSlice[0], 16, 64)
		if err != nil {
			exit("invalid start address %q", startStopSlice[0])
		}
		stopAddr, err := strconv.ParseUint(startStopSlice[1], 16, 64)
		if err != nil {
			exit("invalid stop address %q", startStopSlice[1])
		}
		addrRanges = append(addrRanges, *memtier.NewAddrRange(startAddr, stopAddr))
	}
	return addrRanges
}

func main() {
	optPid := flag.Int("pid", 0, "-pid=PID operate on this process")
	optPages := flag.String("pages", "", "-pages=[Exclusive,Dirty,NotDirty,InHeap,InAnonymous]")
	optMover := flag.String("mover", "oneshot", "-mover=<oneshot|{'Interval':100,'Bandwidth':20}>")
	optRanges := flag.String("ranges", "", "-ranges=START-STOP[,START-STOP...] include only given ranges")
	optMoveFrom := flag.Int("move-from", -1, "-move-from=NUMA source memory node")
	optMoveTo := flag.Int("move-to", -1, "-move-to=NUMA target memory node")
	optCount := flag.Int("count", 100, "-count=PAGECOUNT number of pages to move at a time")

	flag.Parse()

	// Get pages of a PID
	if *optPid == 0 {
		exit("missing -pid=PID")
	}

	// Parse -pages=...
	pageAttributes, err := parseOptPages(*optPages)
	if err != nil {
		exit(fmt.Sprintf("invalid -pages: %v", err))
	}

	// Parse -ranges=...
	selectedRanges := []memtier.AddrRange{}
	if *optRanges != "" {
		selectedRanges = parseOptRanges(*optRanges)
	}

	p := memtier.NewProcess(*optPid)
	ar, err := p.AddressRanges()
	fmt.Printf("found %d address ranges\n", len(ar.Ranges()))
	if err != nil {
		exit("%v", err)
	}
	if len(selectedRanges) > 0 {
		ar.Intersection(selectedRanges)
	}
	fmt.Printf("using %d address ranges\n", len(ar.Ranges()))

	// Find pages with wanted attributes from the address ranges
	pgs, err := ar.PagesMatching(pageAttributes)
	fmt.Printf("found total %d pages\n", len(pgs.Pages()))
	if *optMover == "" {
		fmt.Printf("missing --mover, doing nothing\n")
	} else if *optMover == "oneshot" {
		// Move pages if --move-to is given
		if *optMoveTo != -1 {
			if *optMoveFrom == -1 {
				// source node not defined, move from any
				// other node to dstNode
				dstNode := memtier.Node(*optMoveTo)
				pgs.NotOnNode(dstNode).MoveTo(dstNode, *optCount)
			} else {
				srcNode := memtier.Node(*optMoveFrom)
				dstNode := memtier.Node(*optMoveTo)
				pgs.OnNode(srcNode).MoveTo(dstNode, *optCount)
			}
		} else {
			fmt.Printf("oneshot: nothing to do without --move-to\n")
		}
	} else if strings.HasPrefix(*optMover, "{") {
		mover := memtier.NewMover()
		if err := mover.SetConfigJson(*optMover); err != nil {
			exit("invalid mover configuration: %v", err)
		}
		mover.Start()
		if *optMoveTo != -1 {
			mover.AddTask(memtier.NewMoverTask(pgs, memtier.Node(*optMoveTo)))
		} else {
			fmt.Printf("mover: nothing to do without --move-to\n")
		}
		bufio.NewReader(os.Stdin).ReadBytes('\n')
		fmt.Printf(memtier.Stats().Dump() + "\n")

	} else {
		exit("invalid --mover, expected \"oneshot\" or MoverConfig JSON")
	}

	// Print node/page status
	for node, pageCount := range pgs.NodePageCount() {
		fmt.Printf("pages found in node %d: %d\n", node, pageCount)
	}
}
