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
	"math/bits"
	"strconv"
	"strings"
)

//
// RDT cache specific bitmask
//

// CacheBitmask represents a cache bitmask in the system's resctrl kernel
// interface. The "width" i.e. the number of bits available depends on the
// underlying hardware.
type CacheBitmask Bitmask

// UnmarshalJSON is the unmarshaller function for "encoding/json"
func (b *CacheBitmask) UnmarshalJSON(data []byte) error {
	// Number of bits available in CacheBitmask
	cacheBitmaskNumBits := uint64(rdtInfo.l3.cbmMask.lsbZero())

	// Drop string quotes
	str := strings.TrimSpace(string(data[1 : len(data)-1]))

	if strings.HasPrefix(str, "0x") {
		// Hex value
		value, err := strconv.ParseUint(str[2:], 16, int(cacheBitmaskNumBits))
		if err != nil {
			return err
		}
		*b = CacheBitmask(value)

		return nil
	} else if str[len(str)-1] == '%' {
		// Percentages of the max number of bits
		split := strings.SplitN(str[0:len(str)-1], "-", 2)
		var low, high uint64
		var err error

		if len(split) == 1 {
			high, err = strconv.ParseUint(split[0], 10, 7)
			if err != nil {
				return err
			}
		} else {
			low, err = strconv.ParseUint(split[0], 10, 7)
			if err != nil {
				return err
			}
			high, err = strconv.ParseUint(split[1], 10, 7)
			if err != nil {
				return err
			}
		}
		if low == 0 {
			low = 1
		}
		if low > high || low > 100 || high > 100 {
			return rdtError("invalid percentage range %q", str)
		}

		// Convert percentage limits to bit numbers
		// Our effective range is 1%-100%, use substraction (-1) because of
		// arithmetics, so that we don't overflow on 100%
		lsb := (low - 1) * cacheBitmaskNumBits / 100
		msb := (high - 1) * cacheBitmaskNumBits / 100

		*b = ((1 << (msb - lsb + 1)) - 1) << lsb

		return nil
	}

	// Last, try "list" format (i.e. smthg like 0,2,5-9,...)
	value, err := ListStrToBitmask(str)
	if err != nil {
		return err
	}
	*b = CacheBitmask(value)
	return nil
}

//
// Generic bitbmask
//

// Bitmask represents a generic 64 bit wide bitmask
type Bitmask uint64

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

func (b Bitmask) lsbZero() int {
	return bits.TrailingZeros64(^uint64(b))
}
