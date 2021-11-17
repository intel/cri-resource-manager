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

func main() {
	fmt.Printf("memory exerciser\npid: %d\n", os.Getpid())
	optTTL := flag.Int("ttl", -1, "do not wait for keypress, terminate after given time (seconds)")
	optBCount := flag.Int("bc", 1, "number of byte arrays")
	optBSize := flag.Int("bs", 1024*1024, "size of each byte array [kB]")
	optBReaderCount := flag.Int("brc", 1, "number of byte arrays to be read")
	optBWriterCount := flag.Int("bwc", 1, "number of byte arrays to be written")
	optBReadSize := flag.Int64("brs", 1024*1024, "size of read on each byte array [kB]")
	optBWriteSize := flag.Int64("bws", 1024*1024, "size of write on each byte array [kB]")
	optBReadSizeDelta := flag.Int64("brsd", 0, "size change on each iteration [kB]")
	optBWriteSizeDelta := flag.Int64("bwsd", 0, "size change on each iteration [kB]")
	optBReadOffset := flag.Int64("bro", 0, "offset of read on each byte array [kB]")
	optBWriteOffset := flag.Int64("bwo", 0, "offset of write on each byte array [kB]")
	optBReadOffsetDelta := flag.Int64("brod", 0, "offset change on each iteration [kB]")
	optBWriteOffsetDelta := flag.Int64("bwod", 0, "offset change on each iteration [kB]")
	optBReadInterval := flag.Int64("bri", 0, "read interval on each byte array [us]")
	optBWriteInterval := flag.Int64("bwi", 0, "write interval on each byte array [us]")
	flag.Parse()

	// create byte arrays
	fmt.Printf("creating %d byte arrays\n", *optBCount)
	for i := 0; i < *optBCount; i++ {
		ab = append(ab, make([]byte, *optBSize*1024))
		for j := 0; j < *optBSize*1024; j++ {
			ab[i][j] = 0x01
		}
		fmt.Printf("    array %d: %s\n", i, bAddrRange(ab[i], *optBSize*1024))
	}

	// create readers
	fmt.Printf("creating memory readers and writers\n")
	for i := 0; i < *optBReaderCount; i++ {
		go bExerciser(true, false, ab[i], *optBReadOffset*1024, *optBReadSize*1024, *optBReadInterval, *optBReadOffsetDelta*1024, *optBReadSizeDelta*1024)
		fmt.Printf("    reader %d: %s\n", i,
			bAddrRange(ab[i][(*optBReadOffset)*1024:], int(*optBReadSize)*1024))
	}

	// create writers
	for i := 0; i < *optBWriterCount; i++ {
		go bExerciser(false, true, ab[i], *optBWriteOffset*1024, *optBWriteSize*1024, *optBWriteInterval, *optBWriteOffsetDelta*1024, *optBWriteSizeDelta*1024)
		fmt.Printf("    writer %d: %s\n", i,
			bAddrRange(ab[i][(*optBWriteOffset)*1024:], int(*optBWriteSize)*1024))
	}

	// wait
	if *optTTL == -1 {
		fmt.Printf("press enter to exit...")
		bufio.NewReader(os.Stdin).ReadString('\n')
	} else {
		time.Sleep(time.Duration(*optTTL) * time.Second)
	}
}
