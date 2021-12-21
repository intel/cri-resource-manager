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

package memtier

import (
	"fmt"
	"strconv"
	"strings"
)

func ParseBytes(s string) (int64, error) {
	origS := s
	factor := int64(1)
	if len(s) == 0 {
		return 0, fmt.Errorf("syntax error in bytes: string is empty")
	}
	if s[len(s)-1] == 'B' {
		s = s[:len(s)-1]
	}
	numpart := s[:len(s)-1]
	switch c := s[len(s)-1]; {
	case c == 'k':
		factor = 1024
	case c == 'M':
		factor = 1024 * 1024
	case c == 'G':
		factor = 1024 * 1024 * 1024
	case c == 'T':
		factor = 1024 * 1024 * 1024 * 1024
	case '0' <= c && c <= '9':
		numpart = s
	default:
		return 0, fmt.Errorf("syntax error in bytes %q: unexpected unit %q", origS, c)
	}
	n, err := strconv.ParseInt(strings.TrimSpace(numpart), 10, 0)
	if err != nil {
		return 0, fmt.Errorf("syntax error in bytes %q: bad numeric part %q", origS, numpart)
	}
	return n * factor, nil
}

func MustParseBytes(s string) int64 {
	bytes, err := ParseBytes(s)
	if err != nil {
		panic(err)
	}
	return bytes
}
