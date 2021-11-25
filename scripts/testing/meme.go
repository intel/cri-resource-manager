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
	"strconv"
	"time"
)

var ab [][]byte = [][]byte{} // array of byte arrays

var bValue byte = 0

func bExerciser(read, write bool, b []byte, offset int64, count int64, interval int64, offsetDelta int64, countDelta int64) {
	round := int64(0)
	for {
		roundStartIndex := (offset + (offsetDelta * round)) % int64(len(b))
		roundCount := count + (countDelta * round)
		if roundCount+roundStartIndex >= int64(len(b)) {
			roundCount = int64(len(b)) - roundStartIndex
		}
		if interval > 0 {
			time.Sleep(time.Duration(interval) * time.Microsecond)
			if interval >= 1000000 {
				fmt.Printf("    - round: %d, read: %v, write %v, range %x-%x\n", round, read, write, &b[roundStartIndex], &b[roundStartIndex+roundCount])
			}
		}
		if write {
			for i := roundStartIndex; i < roundStartIndex+roundCount; i++ {
				b[i] = bValue
			}
		}
		if read {
			for i := roundStartIndex; i < roundStartIndex+roundCount; i++ {
				bValue += b[i]
			}
		}
		round += 1
	}
}

func bAddrRange(b []byte, count int) string {
	if count > len(b) {
		count = len(b)
	}
	return fmt.Sprintf("%x-%x (%d bytes)",
		&b[0], &b[count-1], count)
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
		fmt.Printf("syntax error in %s %q: expected [1-9][0-9]*[kMG]?\n")
		os.Exit(1)
	}
	return n * factor
}

func main() {
	fmt.Printf("memory exerciser\npid: %d\n", os.Getpid())
	optTTL := flag.Int("ttl", -1, "do not wait for keypress, terminate after given time (seconds)")
	optBCount := flag.Int("bc", 1, "number of byte arrays")
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
	optBReadInterval := flag.Int64("bri", 0, "read interval on each byte array [us]")
	optBWriteInterval := flag.Int64("bwi", 0, "write interval on each byte array [us]")
	flag.Parse()

	bSize := numBytes("-bs", *optBSize)

	bReadSize := numBytes("-brs", *optBReadSize)
	bReadSizeDelta := numBytes("-brsd", *optBReadSizeDelta)
	bReadOffset := numBytes("-bro", *optBReadOffset)
	bReadOffsetDelta := numBytes("-brod", *optBReadOffsetDelta)

	bWriteSize := numBytes("-bws", *optBWriteSize)
	bWriteSizeDelta := numBytes("-bwsd", *optBWriteSizeDelta)
	bWriteOffset := numBytes("-bwo", *optBWriteOffset)
	bWriteOffsetDelta := numBytes("-bwod", *optBWriteOffsetDelta)

	// create byte arrays
	fmt.Printf("creating %d byte arrays\n", *optBCount)
	for i := 0; i < *optBCount; i++ {
		ab = append(ab, make([]byte, bSize))
		for j := int64(0); j < bSize; j++ {
			ab[i][j] = 0x01
		}
		fmt.Printf("    array %d: %s\n", i, bAddrRange(ab[i], int(bSize)))
	}

	// create readers
	fmt.Printf("creating memory readers and writers\n")
	for i := 0; i < *optBReaderCount; i++ {
		go bExerciser(true, false, ab[i], bReadOffset, bReadSize, *optBReadInterval, bReadOffsetDelta, bReadSizeDelta)
		fmt.Printf("    reader %d: %s\n", i,
			bAddrRange(ab[i][bReadOffset:], int(bReadSize)))
	}

	// create writers
	for i := 0; i < *optBWriterCount; i++ {
		go bExerciser(false, true, ab[i], bWriteOffset, bWriteSize, *optBWriteInterval, bWriteOffsetDelta, bWriteSizeDelta)
		fmt.Printf("    writer %d: %s\n", i,
			bAddrRange(ab[i][bWriteOffset:], int(bWriteSize)))
	}

	// wait
	if *optTTL == -1 {
		fmt.Printf("press enter to exit...")
		bufio.NewReader(os.Stdin).ReadString('\n')
	} else {
		time.Sleep(time.Duration(*optTTL) * time.Second)
	}
}
