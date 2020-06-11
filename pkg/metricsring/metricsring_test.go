/*
Copyright 2020 Intel Corporation

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

package metricsring

import (
	"reflect"
	"testing"
)

func TestMetricsRing(t *testing.T) {

	cases := []struct {
		name     string
		input    []float64
		output   []float64
		inputlen int
		count    int
	}{
		{
			name:     "get all samples",
			input:    []float64{1.1, 2.2, 3.3, 4.4},
			output:   []float64{1.1, 2.2, 3.3, 4.4},
			inputlen: 4,
			count:    4,
		},
		{
			name:     "get less samples",
			input:    []float64{1.1, 2.2, 3.3, 4.4},
			output:   []float64{3.3, 4.4},
			inputlen: 4,
			count:    2,
		},
		{
			name:     "get excess samples (ask more than ring size)",
			input:    []float64{1.1, 2.2, 3.3, 4.4},
			output:   []float64{1.1, 2.2, 3.3, 4.4},
			inputlen: 4,
			count:    8,
		},
		{
			name:     "get excess samples (ring not yet full)",
			input:    []float64{3.3, 4.4},
			output:   []float64{3.3, 4.4},
			inputlen: 4,
			count:    4,
		},
	}
	for _, tc := range cases {
		test := tc
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			mr := NewMetricsRing(test.inputlen)
			for _, v := range test.input {
				mr.Push(v)
			}
			output := mr.GetLastNSamples(test.count)
			if !reflect.DeepEqual(output, test.output) {
				t.Fatalf("GetLastNSamples: expected output: %+v got: %+v", test.output, output)
			}
			all := mr.GetLastNSamples(mr.GetSize())
			if !reflect.DeepEqual(all, test.input) {
				t.Fatalf("GetAllSamples: expected output: %+v got: %+v", test.input, all)
			}
		})
	}
}
