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

package dyp

import (
	"testing"
)

func TestChangesDynamicPools(t *testing.T) {
	tcases := []struct {
		name          string
		opts1         *DynamicPoolsOptions
		opts2         *DynamicPoolsOptions
		expectedValue bool
	}{
		{
			name:          "both options are nil",
			expectedValue: false,
		},
		{
			name:          "one option is nil",
			opts2:         &DynamicPoolsOptions{},
			expectedValue: true,
		},
		{
			name: "reserved pool namespaces differ by len",
			opts1: &DynamicPoolsOptions{
				ReservedPoolNamespaces: []string{"ns0"},
			},
			opts2: &DynamicPoolsOptions{
				ReservedPoolNamespaces: []string{},
			},
			expectedValue: true,
		},
		{
			name: "reserved pool namespaces differ by content",
			opts1: &DynamicPoolsOptions{
				ReservedPoolNamespaces: []string{"ns0"},
			},
			opts2: &DynamicPoolsOptions{
				ReservedPoolNamespaces: []string{"ns1"},
			},
			expectedValue: true,
		},
		{
			name: "dynamic-pool defs differ",
			opts1: &DynamicPoolsOptions{
				ReservedPoolNamespaces: []string{"ns0"},
				DynamicPoolDefs:        []*DynamicPoolDef{},
			},
			opts2: &DynamicPoolsOptions{
				ReservedPoolNamespaces: []string{"ns1"},
				DynamicPoolDefs:        []*DynamicPoolDef{},
			},
			expectedValue: true,
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			value := changesDynamicPools(tc.opts1, tc.opts2)
			if value != tc.expectedValue {
				t.Errorf("Expected return value %v but got %v", tc.expectedValue, value)
			}
		})
	}
}
