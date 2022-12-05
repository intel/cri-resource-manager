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
	"strings"
	"testing"
)

func TestNewAddrRangeFromString(t *testing.T) {
	tcases := []struct {
		name           string
		input          string
		expectedOutput *AddrRange
		expectedError  string
	}{
		{
			name:          "empty string",
			input:         "",
			expectedError: "invalid",
		}, {
			name:          "missing start-end",
			input:         "-",
			expectedError: "invalid",
		}, {
			name:          "missing end",
			input:         "42-",
			expectedError: "invalid",
		}, {
			name:          "missing start+size",
			input:         "+",
			expectedError: "invalid",
		}, {
			name:          "missing size",
			input:         "42+",
			expectedError: "invalid",
		}, {
			name:           "zero",
			input:          "0",
			expectedOutput: NewAddrRange(0, constUPagesize),
		}, {
			name:           "single number",
			input:          "4",
			expectedOutput: NewAddrRange(4, 4+constUPagesize),
		}, {
			name:           "64-bit number",
			input:          "deadbeefcafebabe",
			expectedOutput: NewAddrRange(0xdeadbeefcafebabe, 0xdeadbeefcafebabe+constUPagesize),
		}, {
			name:           "single number range",
			input:          "4-6",
			expectedOutput: NewAddrRange(4, 6),
		}, {
			name:           "64-bit range",
			input:          "deadbeefcafebabe-deadcafebeefbabe",
			expectedOutput: NewAddrRange(0xdeadbeefcafebabe, 0xdeadcafebeefbabe),
		}, {
			name:           "64-bit start>end range",
			input:          "deadcafebeefbabe-deadbeefcafebabe",
			expectedOutput: NewAddrRange(0xdeadbeefcafebabe, 0xdeadcafebeefbabe),
		}, {
			name:           "single number size with bytes",
			input:          "4+1MB",
			expectedOutput: NewAddrRange(4, 4+1024*1024),
		}, {
			name:           "64-bit size without bytes",
			input:          "deadbeefcafebabe+1G",
			expectedOutput: NewAddrRange(0xdeadbeefcafebabe, 0xdeadbeefcafebabe+1024*1024*1024),
		}, {
			name:           "first kibibyte",
			input:          "0+1kiB",
			expectedOutput: NewAddrRange(0, 1024),
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			output, err := NewAddrRangeFromString(tc.input)
			seenError := fmt.Sprintf("%s", err)
			if tc.expectedError != "" {
				if !strings.Contains(seenError, tc.expectedError) {
					t.Errorf("expected error containing %q, got %q",
						tc.expectedError, seenError)
				}
			} else if err != nil {
				t.Errorf("got unexpected error: %q", seenError)
			}
			if tc.expectedOutput != nil && !tc.expectedOutput.Equals(output) {
				t.Errorf("expected output %q got %q",
					tc.expectedOutput, output)
			}
		})
	}
}
