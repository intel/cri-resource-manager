/*
Copyright 2019 Intel Corporation

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package rdt

import (
	"fmt"
	"math/bits"
	"strconv"
	"strings"
)

// Bitmask represents a generic 64 bit wide bitmask
type Bitmask uint64

// MarshalJSON implements the Marshaler interface of "encoding/json"
func (b Bitmask) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("\"%#x\"", b)), nil
}

// ListStr prints the bitmask in human-readable format, similar to e.g. the
// cpuset format of the Linux kernel
func (b Bitmask) ListStr() string {
	str := ""
	sep := ""

	shift := int(0)
	lsbOne := b.lsbOne()

	// Process "ranges of ones"
	for lsbOne != -1 {
		b >>= uint(lsbOne)

		// Get range lenght from the position of the first zero
		numOnes := b.lsbZero()

		if numOnes == 1 {
			str += sep + strconv.Itoa(lsbOne+shift)
		} else {
			str += sep + strconv.Itoa(lsbOne+shift) + "-" + strconv.Itoa(lsbOne+numOnes-1+shift)
		}

		// Shift away the bits that have been processed
		b >>= uint(numOnes)
		shift += lsbOne + numOnes

		// Get next bit that is set (if any)
		lsbOne = b.lsbOne()

		sep = ","
	}

	return str
}

// ListStrToBitmask parses a string containing a human-readable list of bit
// numbers into a bitmask
func ListStrToBitmask(str string) (Bitmask, error) {
	b := Bitmask(0)

	// Empty bitmask
	if len(str) == 0 {
		return b, nil
	}

	ranges := strings.Split(str, ",")
	for _, ran := range ranges {
		split := strings.SplitN(ran, "-", 2)

		bitNum, err := strconv.ParseUint(split[0], 10, 6)
		if err != nil {
			return b, rdtError("invalid bitmask %q: %v", str, err)
		}

		if len(split) == 1 {
			b |= 1 << bitNum
		} else {
			endNum, err := strconv.ParseUint(split[1], 10, 6)
			if err != nil {
				return b, rdtError("invalid bitmask %q: %v", str, err)
			}
			if endNum <= bitNum {
				return b, rdtError("invalid range %q in bitmask %q", ran, str)
			}
			b |= (1<<(endNum-bitNum+1) - 1) << bitNum
		}
	}
	return b, nil
}

func (b Bitmask) lsbOne() int {
	if b == 0 {
		return -1
	}
	return bits.TrailingZeros64(uint64(b))
}

func (b Bitmask) msbOne() int {
	// Returns -1 for b == 0
	return 63 - bits.LeadingZeros64(uint64(b))
}

func (b Bitmask) lsbZero() int {
	return bits.TrailingZeros64(^uint64(b))
}
