// Copyright 2020 Intel Corporation. All Rights Reserved.
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

package kubernetes

import (
	"testing"

	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
)

func TestShortCPUSet(t *testing.T) {
	tcases := []struct {
		source string
		native string
		short  string
	}{
		{source: "", native: "", short: ""},
		{source: "1", native: "1", short: "1"},
		{source: "1,2", native: "1-2", short: "1,2"},
		{source: "1,2,3,4,5,6,7", native: "1-7", short: "1-7"},
		{source: "1,3,5,7,9,11", native: "1,3,5,7,9,11", short: "1-11:2"},
		{source: "1,3,5,7,8,10,12,14,16", native: "1,3,5,7-8,10,12,14,16", short: "1-7:2,10-16:2"},
		{
			source: "0,2,8,10,12,14,16,18,20,22,24,26,28,30,32,34,36,38,40,42,44,46,48,50,52,54,56,58,64,66,68,70,72,74,76,78,80,82,84,86,88,90,92,94,96,98,100,102,104,106,108,110",
			native: "0,2,8,10,12,14,16,18,20,22,24,26,28,30,32,34,36,38,40,42,44,46,48,50,52,54,56,58,64,66,68,70,72,74,76,78,80,82,84,86,88,90,92,94,96,98,100,102,104,106,108,110",
			short:  "0-110:2",
		},
	}
	for _, tc := range tcases {
		cset := cpuset.MustParse(tc.source)
		native := cset.String()
		if native != tc.native {
			t.Errorf("incorrect native CPUSet for %q, expected %q, got %q",
				tc.source, tc.native, native)
		}
		short := ShortCPUSet(cset)
		if native != tc.native {
			t.Errorf("incorrect shortened CPUSet for %q, expected %q, got %q",
				tc.source, tc.short, short)
		}
	}
}
