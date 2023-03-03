// Copyright 2022 Intel Corporation. All Rights Reserved.
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
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// parseTimeDuration parses time duration string
func parseTimeDuration(s string) (time.Duration, error) {
	factor := float64(time.Second)
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
	f, err := strconv.ParseFloat(strings.TrimSpace(numpart), 64)
	if err != nil {
		return time.Duration(0), fmt.Errorf("syntax error in time duration %s %w, expected [1-9][0-9]*(ns|us|ms|s|m|h)?\n", s, err)
	}
	if math.IsNaN(f) {
		return time.Duration(0), fmt.Errorf("invalid time duration %s, number or inf expected\n", s)
	}
	return time.Duration(f * factor), nil
}

func sliceContainsInt(haystack []int, needle int) bool {
	for _, straw := range haystack {
		if straw == needle {
			return true
		}
	}
	return false
}

func sortedCopyOfInts(orig []int) []int {
	return sortInts(copyInts(orig))
}

func copyInts(orig []int) []int {
	retval := make([]int, len(orig))
	copy(retval, orig)
	return retval
}

func sortInts(orig []int) []int {
	sort.Ints(orig)
	return orig
}

type mapNodeUint64 map[Node]uint64

func (m mapNodeUint64) sortedKeys() []Node {
	keys := make([]Node, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})
	return keys
}

type mapStringPStatsPulse map[string]*StatsPulse

func (m mapStringPStatsPulse) sortedKeys() []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

type mapIntUint64 map[int]uint64

func (m mapIntUint64) sortedKeys() []int {
	keys := make([]int, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Ints(keys)
	return keys
}

type mapIntPStatsPidMadvised map[int]*StatsPidMadvised

func (m mapIntPStatsPidMadvised) sortedKeys() []int {
	keys := make([]int, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Ints(keys)
	return keys
}

type mapIntPStatsPidMoved map[int]*StatsPidMoved

func (m mapIntPStatsPidMoved) sortedKeys() []int {
	keys := make([]int, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Ints(keys)
	return keys
}

type mapIntPStatsPidScanned map[int]*StatsPidScanned

func (m mapIntPStatsPidScanned) sortedKeys() []int {
	keys := make([]int, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Ints(keys)
	return keys
}
