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

func bWriter(b []byte, count int, interval int) {
	if count > len(b) {
		count = len(b)
	}
	for {
		if interval > 0 {
			time.Sleep(time.Duration(interval) * time.Microsecond)
		}
		for i := 0; i < count; i++ {
			b[i] = bValue
		}
	}
}

func bReader(b []byte, count int, interval int) {
	if count > len(b) {
		count = len(b)
	}
	for {
		if interval > 0 {
			time.Sleep(time.Duration(interval) * time.Microsecond)
		}
		for i := 0; i < count; i++ {
			bValue += b[i]
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

func main() {
	fmt.Printf("memory exerciser\npid: %d\n", os.Getpid())
	optBCount := flag.Int("bc", 1, "number of byte arrays")
	optBSize := flag.Int("bs", 1024*1024, "size of each byte array [kB]")
	optBReaderCount := flag.Int("brc", 1, "number of byte arrays to be read")
	optBWriterCount := flag.Int("bwc", 1, "number of byte arrays to be written")
	optBReadSize := flag.Int("brs", 1024*1024, "size of read on each byte array [kB]")
	optBWriteSize := flag.Int("bws", 1024*1024, "size of write on each byte array [kB]")
	optBReadOffset := flag.Int("bro", 0, "offset of read on each byte array [kB]")
	optBWriteOffset := flag.Int("bwo", 0, "offset of read on each byte array [kB]")
	optBReadInterval := flag.Int("bri", 0, "read interval on each byte array [us]")
	optBWriteInterval := flag.Int("bwi", 0, "write interval on each byte array [us]")
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
		go bReader(ab[i][*optBReadOffset*1024:], *optBReadSize*1024, *optBReadInterval)
		fmt.Printf("    reader %d: %s\n", i,
			bAddrRange(ab[i][(*optBReadOffset)*1024:], (*optBReadSize)*1024))
	}

	// create writers
	for i := 0; i < *optBWriterCount; i++ {
		go bWriter(ab[i][*optBWriteOffset*1024:], *optBWriteSize*1024, *optBWriteInterval)
		fmt.Printf("    writer %d: %s\n", i,
			bAddrRange(ab[i][(*optBWriteOffset)*1024:], (*optBWriteSize)*1024))
	}

	// wait
	fmt.Printf("press enter to exit...")
	bufio.NewReader(os.Stdin).ReadString('\n')
}
