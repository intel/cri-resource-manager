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

// This file implements Memory Exerciser

package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type DeltaCycle struct {
	delta   int
	cycleNs time.Duration
	nextNs  time.Duration
}

type BArray struct {
	readers int
	writers int
	b       []byte
}

var bas []*BArray = []*BArray{} // array of byte arrays

var bValue byte = 0

var readers, writers []func()

var bReaderCount int
var bReadSize int64
var bReadSizeDelta int64
var bReadOffset int64
var bReadOffsetDelta int64
var bReadInterval time.Duration

var bWriterCount int
var bWriteSize int64
var bWriteSizeDelta int64
var bWriteOffset int64
var bWriteOffsetDelta int64
var bWriteInterval time.Duration

var constPagesize int = os.Getpagesize()

func bExerciser(read, write bool, ba *BArray, offset int64, count int64, interval time.Duration, offsetDelta int64, countDelta int64) {
	b := ba.b
	if count >= int64(len(b[offset:])) {
		count = int64(len(b[offset:])) - 1
	}
	fmt.Printf("    exerciser start: r=%t w=%t %s\n", read, write, bAddrRange(ba.b[offset:], int(count)))
	round := int64(0)
	defer func() {
		fmt.Printf("    exerciser stop: read=%t write=%t %s\n", read, write, bAddrRange(ba.b[offset:], int(count)))
		runtime.GC()
	}()
	for ba.readers > 0 || ba.writers > 0 {
		roundStartIndex := (offset + (offsetDelta * round)) % int64(len(b))
		roundCount := count + (countDelta * round)
		if roundCount+roundStartIndex >= int64(len(b)) {
			roundCount = int64(len(b)) - roundStartIndex
		}
		if write {
			for i := roundStartIndex; i < roundStartIndex+roundCount; i++ {
				b[i] = bValue
				bValue += 1
			}
		}
		if read {
			for i := roundStartIndex; i < roundStartIndex+roundCount; i++ {
				bValue += b[i]
			}
		}
		if interval > 0 {
			time.Sleep(interval)
			if interval >= 1000000 {
				fmt.Printf("    - round: %d, read=%v, write=%v, range %x-%x\n", round, read, write, &b[roundStartIndex], &b[roundStartIndex+roundCount-1])
			}
		}
		round += 1
	}
}

func bReallocators(bSize int64, deltaCycles []DeltaCycle) {
	nowNs := time.Duration(time.Now().UnixNano())
	// Calculate next (dc.nextNs) wake up times for all deltas.
	for i, dc := range deltaCycles {
		deltaCycles[i].nextNs = nowNs + dc.cycleNs
	}
	for {
		nowNs := time.Duration(time.Now().UnixNano())
		minNextNs := nowNs + time.Duration(24*time.Hour)
		for _, dc := range deltaCycles {
			if dc.nextNs < minNextNs {
				minNextNs = dc.nextNs
			}
		}
		time.Sleep(minNextNs - nowNs)
		nowNs = minNextNs
		for i, dc := range deltaCycles {
			if nowNs >= dc.nextNs {
				deltaCycles[i].nextNs += dc.cycleNs
				fmt.Printf("    allocate %d arrays (every %d s)\n", dc.delta, dc.cycleNs/1000/1000/1000)
				if dc.delta > 0 {
					for n := 0; n < dc.delta; n++ {
						allocateBArray(bSize)
					}
				} else {
					for n := 0; n > dc.delta; n-- {
						freeBArray()
					}
				}
			}
		}
	}
}

func bAddrRange(b []byte, count int) string {
	if count > len(b) {
		count = len(b)
	}
	return fmt.Sprintf("%x-%x (%d bytes)",
		&b[0], &b[count-1], count)
}

func numNs(arg, s string) time.Duration {
	factor := time.Duration(1)
	suffixLen := 0
	switch {
	case strings.HasSuffix(s, "ns"):
		factor = 1
		suffixLen = 2
	case strings.HasSuffix(s, "us"):
		factor = 1000
		suffixLen = 2
	case strings.HasSuffix(s, "ms"):
		factor = 1000 * 1000
		suffixLen = 2
	case strings.HasSuffix(s, "s"):
		factor = 1000 * 1000 * 1000
		suffixLen = 1
	case strings.HasSuffix(s, "m"):
		factor = 1000 * 1000 * 1000 * 60
		suffixLen = 1
	case strings.HasSuffix(s, "h"):
		factor = 1000 * 1000 * 1000 * 60 * 60
		suffixLen = 1
	}
	numpart := s[0 : len(s)-suffixLen]
	n, err := strconv.ParseInt(numpart, 10, 0)
	if err != nil {
		fmt.Printf("syntax error in %s %q: expected [1-9][0-9]*(ns|us|ms|s|m|h)?\n", arg, s)
		os.Exit(1)
	}
	return time.Duration(n) * factor
}

func numBytes(arg, s string) int64 {
	factor := int64(1)
	numpart := s[:len(s)-1]
	switch s[len(s)-1] {
	case 'k':
		factor = 1024
	case 'M':
		factor = 1024 * 1024
	case 'G':
		factor = 1024 * 1024 * 1024
	default:
		numpart = s
	}
	n, err := strconv.ParseInt(numpart, 10, 0)
	if err != nil {
		fmt.Printf("syntax error in %s %q: expected [1-9][0-9]*[kMG]?\n", arg, s)
		os.Exit(1)
	}
	return n * factor
}

func allocateBArray(bSize int64) {
	ba := &BArray{
		b: make([]byte, bSize),
	}
	bas = append(bas, ba)
	for j := int64(0); j < bSize; j += int64(constPagesize) {
		ba.b[j] = 0x01
	}
	fmt.Printf("    array: %s\n", bAddrRange(ba.b, int(bSize)))
	// create reader
	if bReaderCount > 0 {
		ba.readers = 1
		bReaderCount--
		go bExerciser(true, false, ba, bReadOffset, bReadSize, bReadInterval, bReadOffsetDelta, bReadSizeDelta)
	}
	// create writers
	if bWriterCount > 0 {
		ba.writers = 1
		bWriterCount--
		go bExerciser(false, true, ba, bWriteOffset, bWriteSize, bWriteInterval, bWriteOffsetDelta, bWriteSizeDelta)
	}
}

func freeBArray() {
	if len(bas) > 0 {
		i := len(bas) - 1
		fmt.Printf("    free array %d: %s\n", i, bAddrRange(bas[i].b, len(bas[i].b)))
		bReaderCount += bas[i].readers
		bas[i].readers = 0
		bWriterCount += bas[i].writers
		bas[i].writers = 0
		bas = bas[:i]
		runtime.GC()
	}
}

func main() {
	fmt.Printf("memory exerciser\npid: %d\n", os.Getpid())
	optTTL := flag.String("ttl", "", "do not wait for keypress, terminate after given time T(ns|us|s|m|h)")
	optBCount := flag.Int("bc", 1, "number of byte arrays")
	optBCountDelta := flag.String("bcd", "", "array count delta: DELTA/T[,DELTA/T...], \"1/20s,-6/2m\": add 1 array every 20 s, delete 6 arrays every 2 minutes")
	optBSize := flag.String("bs", "1G", "size of each byte array [k, M or G]")
	optBReaderCount := flag.Int("brc", 1, "number of byte arrays to be read")
	optBWriterCount := flag.Int("bwc", 1, "number of byte arrays to be written")
	optBReadSize := flag.String("brs", "1G", "size of read on each byte array")
	optBWriteSize := flag.String("bws", "1G", "size of write on each byte array")
	optBReadSizeDelta := flag.String("brsd", "0k", "size change on each iteration")
	optBWriteSizeDelta := flag.String("bwsd", "0k", "size change on each iteration")
	optBReadOffset := flag.String("bro", "0M", "offset of read on each byte array")
	optBWriteOffset := flag.String("bwo", "0M", "offset of write on each byte array")
	optBReadOffsetDelta := flag.String("brod", "0k", "offset change on each iteration")
	optBWriteOffsetDelta := flag.String("bwod", "0k", "offset change on each iteration")
	optBReadInterval := flag.String("bri", "0", "read interval on each byte array, T(ns|us|ms|s|m|h)")
	optBWriteInterval := flag.String("bwi", "0", "write interval on each byte array")
	flag.Parse()

	bSize := numBytes("-bs", *optBSize)

	bReaderCount = *optBReaderCount
	bReadSize = numBytes("-brs", *optBReadSize)
	bReadSizeDelta = numBytes("-brsd", *optBReadSizeDelta)
	bReadOffset = numBytes("-bro", *optBReadOffset)
	bReadOffsetDelta = numBytes("-brod", *optBReadOffsetDelta)
	bReadInterval = numNs("-bri", *optBReadInterval)

	bWriterCount = *optBWriterCount
	bWriteSize = numBytes("-bws", *optBWriteSize)
	bWriteSizeDelta = numBytes("-bwsd", *optBWriteSizeDelta)
	bWriteOffset = numBytes("-bwo", *optBWriteOffset)
	bWriteOffsetDelta = numBytes("-bwod", *optBWriteOffsetDelta)
	bWriteInterval = numNs("-bwi", *optBWriteInterval)

	// create byte arrays
	fmt.Printf("creating %d byte arrays\n", *optBCount)
	for i := 0; i < *optBCount; i++ {
		allocateBArray(bSize)
	}

	// create byte array reallocators
	if *optBCountDelta != "" {
		deltaCyclesSpec := *optBCountDelta
		deltaCycles := []DeltaCycle{}
		for _, deltaCycleSpec := range strings.Split(deltaCyclesSpec, ",") {
			deltaCycle := strings.Split(deltaCycleSpec, "/")
			if len(deltaCycle) != 2 {
				fmt.Printf("syntax error in %q (%q): expected DELTA/TIME\n", deltaCyclesSpec, deltaCycleSpec)
				os.Exit(1)
			}
			delta, err := strconv.Atoi(deltaCycle[0])
			if err != nil {
				fmt.Printf("bad delta in %q (%q): expected INT\n", deltaCyclesSpec, deltaCycle[0])
				os.Exit(1)
			}
			cycle := numNs("-bcd", deltaCycle[1])
			deltaCycles = append(deltaCycles, DeltaCycle{delta, cycle, 0})
		}
		fmt.Printf("creating byte array reallocators\n")
		go bReallocators(bSize, deltaCycles)
	}

	// wait
	if *optTTL == "" {
		fmt.Printf("press enter to exit...\n")
		bufio.NewReader(os.Stdin).ReadString('\n')
	} else {
		time.Sleep(numNs("-ttl", *optTTL))
	}
}
