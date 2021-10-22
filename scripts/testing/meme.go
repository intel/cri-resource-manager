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

var aba [][]byte = [][]byte{} // array of byte arrays

var baValue byte = 0

func baWriter(ba []byte, count int, interval int) {
	for {
		if interval > 0 {
			time.Sleep(time.Duration(interval) * time.Microsecond)
		}
		for i := 0; i < count; i++ {
			ba[i] = baValue
		}
	}
}

func baReader(ba []byte, count int, interval int) {
	for {
		if interval > 0 {
			time.Sleep(time.Duration(interval) * time.Microsecond)
		}
		for i := 0; i < 0; i++ {
			baValue += ba[i]
		}
	}
}

func baAddrRange(ba []byte, count int) string {
	return fmt.Sprintf("%x - %x (%d bytes)",
		&ba[0], &ba[count-1], count)
}

func main() {
	fmt.Printf("memory exerciser\npid: %d\n", os.Getpid())
	optBACount := flag.Int("bac", 1, "number of byte arrays")
	optBASize := flag.Int("bas", 1024*1024, "size of each byte array [kB]")
	optBAReaderCount := flag.Int("barc", 1, "number of byte arrays to be read")
	optBAWriterCount := flag.Int("bawc", 1, "number of byte arrays to be written")
	optBAReadSize := flag.Int("bars", 1024*1024, "size of read on each byte array [kB]")
	optBAWriteSize := flag.Int("baws", 1024*1024, "size of write on each byte array [kB]")
	optBAReadOffset := flag.Int("baro", 0, "offset of read on each byte array [kB]")
	optBAWriteOffset := flag.Int("bawo", 0, "offset of read on each byte array [kB]")
	optBAReadInterval := flag.Int("bari", 0, "read interval on each byte array [us]")
	optBAWriteInterval := flag.Int("bawi", 0, "write interval on each byte array [us]")
	flag.Parse()

	// create byte arrays
	fmt.Printf("creating %d byte arrays\n", *optBACount)
	for i := 0; i < *optBACount; i++ {
		aba = append(aba, make([]byte, *optBASize*1024))
		for j := 0; j < *optBASize*1024; j++ {
			aba[i][j] = 0x01
		}
		fmt.Printf("    array %d: %s\n", i, baAddrRange(aba[i], *optBASize*1024))
	}

	// create readers
	fmt.Printf("creating memory readers and writers\n")
	for i := 0; i < *optBAReaderCount; i++ {
		go baReader(aba[i][*optBAReadOffset*1024:], *optBAReadSize*1024, *optBAReadInterval)
		fmt.Printf("    reader %d: %s\n", i,
			baAddrRange(aba[i][(*optBAReadOffset)*1024:], (*optBAReadSize)*1024))
	}

	// create writers
	for i := 0; i < *optBAWriterCount; i++ {
		go baWriter(aba[i][*optBAWriteOffset*1024:], *optBAWriteSize*1024, *optBAWriteInterval)
		fmt.Printf("    writer %d: %s\n", i,
			baAddrRange(aba[i][(*optBAWriteOffset)*1024:], (*optBAWriteSize)*1024))
	}

	// wait
	fmt.Printf("press enter to exit...")
	bufio.NewReader(os.Stdin).ReadString('\n')
}
