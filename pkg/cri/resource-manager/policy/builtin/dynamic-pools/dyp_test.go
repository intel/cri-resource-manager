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

	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
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

func TestIsNeedReallocate(t *testing.T) {
	p := &dynamicPools{
		dynamicPools: []*DynamicPool{
			{
				Def: &DynamicPoolDef{
					Name: reservedDynamicPoolDefName,
				},
				Cpus: cpuset.NewCPUSet(1, 2),
			},
			{
				Def: &DynamicPoolDef{
					Name: sharedDynamicPoolDefName,
				},
				Cpus: cpuset.NewCPUSet(3, 4, 5, 6),
			},
			{
				Def: &DynamicPoolDef{
					Name: "poo1",
				},
				Cpus: cpuset.NewCPUSet(7, 8, 9, 10, 11, 12),
			},
			{
				Def: &DynamicPoolDef{
					Name: "poo2",
				},
				Cpus: cpuset.NewCPUSet(0),
			},
		},
	}
	tcases := []struct {
		name          string
		newPoolCpu    map[*DynamicPool]int
		expectedValue bool
	}{
		{
			name: "no need to reallocate",
			newPoolCpu: map[*DynamicPool]int{
				p.dynamicPools[0]: 2,
				p.dynamicPools[1]: 4,
				p.dynamicPools[2]: 6,
				p.dynamicPools[3]: 1,
			},
			expectedValue: false,
		},
		{
			name: "need to reallocate",
			newPoolCpu: map[*DynamicPool]int{
				p.dynamicPools[0]: 2,
				p.dynamicPools[1]: 6,
				p.dynamicPools[2]: 4,
				p.dynamicPools[3]: 1,
			},
			expectedValue: true,
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			value := p.isNeedReallocate(tc.newPoolCpu)
			if value != tc.expectedValue {
				t.Errorf("Expected return value %v but got %v", tc.expectedValue, value)
			}
		})
	}
}

func TestCalculatePoolCpuset(t *testing.T) {
	p := &dynamicPools{
		allowed:  cpuset.NewCPUSet(0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13),
		reserved: cpuset.NewCPUSet(1, 2),
		dynamicPools: []*DynamicPool{
			{
				Def: &DynamicPoolDef{
					Name: reservedDynamicPoolDefName,
				},
				Cpus: cpuset.NewCPUSet(1, 2),
			},
			{
				Def: &DynamicPoolDef{
					Name: sharedDynamicPoolDefName,
				},
				Cpus: cpuset.NewCPUSet(3, 4, 5, 6),
			},
			{
				Def: &DynamicPoolDef{
					Name: "poo1",
				},
				Cpus: cpuset.NewCPUSet(7, 8, 9, 10, 11, 12, 13),
			},
			{
				Def: &DynamicPoolDef{
					Name: "poo2",
				},
				Cpus: cpuset.NewCPUSet(0),
			},
		},
	}
	tcases := []struct {
		name          string
		requestCpu    map[*DynamicPool]int
		remainFree    int
		weight        map[*DynamicPool]float64
		sumWeight     float64
		expectedValue map[*DynamicPool]int
	}{
		{
			name:       "The requests and weight of the dynamic pools are both nil",
			requestCpu: map[*DynamicPool]int{},
			remainFree: 12,
			weight:     map[*DynamicPool]float64{},
			sumWeight:  0.0,
			expectedValue: map[*DynamicPool]int{
				p.dynamicPools[0]: 2,
				p.dynamicPools[1]: 12,
				p.dynamicPools[2]: 0,
				p.dynamicPools[3]: 0,
			},
		},
		{
			name: "The requests of the dynamic pools is not nil, and the requests of the shared dynamic pools is 0",
			requestCpu: map[*DynamicPool]int{
				p.dynamicPools[0]: 1,
				p.dynamicPools[1]: 0,
				p.dynamicPools[2]: 2,
				p.dynamicPools[3]: 2,
			},
			remainFree: 8,
			weight:     map[*DynamicPool]float64{},
			sumWeight:  0.0,
			expectedValue: map[*DynamicPool]int{
				p.dynamicPools[0]: 2,
				p.dynamicPools[1]: 8,
				p.dynamicPools[2]: 2,
				p.dynamicPools[3]: 2,
			},
		},
		{
			name: "The requests of the dynamic pools is not nil, and the requests of the shared dynamic pools is not 0",
			requestCpu: map[*DynamicPool]int{
				p.dynamicPools[0]: 1,
				p.dynamicPools[1]: 2,
				p.dynamicPools[2]: 2,
				p.dynamicPools[3]: 2,
			},
			remainFree: 6,
			weight:     map[*DynamicPool]float64{},
			sumWeight:  0.0,
			expectedValue: map[*DynamicPool]int{
				p.dynamicPools[0]: 2,
				p.dynamicPools[1]: 8,
				p.dynamicPools[2]: 2,
				p.dynamicPools[3]: 2,
			},
		},
		{
			name:       "The weight of the dynamic pools is not nil, and the weight of the shared dynamic pools is not 0",
			requestCpu: map[*DynamicPool]int{},
			remainFree: 12,
			weight: map[*DynamicPool]float64{
				p.dynamicPools[0]: 10.0,
				p.dynamicPools[1]: 100.0,
				p.dynamicPools[2]: 200.0,
				p.dynamicPools[3]: 100.0,
			},
			sumWeight: 400.0,
			expectedValue: map[*DynamicPool]int{
				p.dynamicPools[0]: 2,
				p.dynamicPools[1]: 3,
				p.dynamicPools[2]: 6,
				p.dynamicPools[3]: 3,
			},
		},
		{
			name:       "The weight of the dynamic pools is not nil, and the weight of the shared dynamic pools is 0",
			requestCpu: map[*DynamicPool]int{},
			remainFree: 12,
			weight: map[*DynamicPool]float64{
				p.dynamicPools[0]: 10.0,
				p.dynamicPools[1]: 0.0,
				p.dynamicPools[2]: 200.0,
				p.dynamicPools[3]: 100.0,
			},
			sumWeight: 300.0,
			expectedValue: map[*DynamicPool]int{
				p.dynamicPools[0]: 2,
				p.dynamicPools[1]: 0,
				p.dynamicPools[2]: 8,
				p.dynamicPools[3]: 4,
			},
		},
		{
			name: "The requests and weight of the dynamic pools are not nil, and the requests of the shared dynamic pools is 0",
			requestCpu: map[*DynamicPool]int{
				p.dynamicPools[0]: 1,
				p.dynamicPools[1]: 0,
				p.dynamicPools[2]: 2,
				p.dynamicPools[3]: 2,
			},
			remainFree: 8,
			weight: map[*DynamicPool]float64{
				p.dynamicPools[0]: 10.0,
				p.dynamicPools[1]: 100.0,
				p.dynamicPools[2]: 200.0,
				p.dynamicPools[3]: 100.0,
			},
			sumWeight: 400.0,
			expectedValue: map[*DynamicPool]int{
				p.dynamicPools[0]: 2,
				p.dynamicPools[1]: 2,
				p.dynamicPools[2]: 6,
				p.dynamicPools[3]: 4,
			},
		},
		{
			name: "The requests and weight of the dynamic pools are not nil, and the weight of the shared dynamic pools is 0",
			requestCpu: map[*DynamicPool]int{
				p.dynamicPools[0]: 1,
				p.dynamicPools[1]: 1,
				p.dynamicPools[2]: 2,
				p.dynamicPools[3]: 2,
			},
			remainFree: 7,
			weight: map[*DynamicPool]float64{
				p.dynamicPools[0]: 10.0,
				p.dynamicPools[1]: 0.0,
				p.dynamicPools[2]: 200.0,
				p.dynamicPools[3]: 100.0,
			},
			sumWeight: 300.0,
			expectedValue: map[*DynamicPool]int{
				p.dynamicPools[0]: 2,
				p.dynamicPools[1]: 1,
				p.dynamicPools[2]: 7,
				p.dynamicPools[3]: 4,
			},
		},
		{
			name: "The requests and weight of the dynamic pools are not nil, and the requests and weight of the shared dynamic pools are both 0",
			requestCpu: map[*DynamicPool]int{
				p.dynamicPools[0]: 1,
				p.dynamicPools[1]: 0,
				p.dynamicPools[2]: 2,
				p.dynamicPools[3]: 2,
			},
			remainFree: 8,
			weight: map[*DynamicPool]float64{
				p.dynamicPools[0]: 10.0,
				p.dynamicPools[1]: 0.0,
				p.dynamicPools[2]: 200.0,
				p.dynamicPools[3]: 100.0,
			},
			sumWeight: 300.0,
			expectedValue: map[*DynamicPool]int{
				p.dynamicPools[0]: 2,
				p.dynamicPools[1]: 0,
				p.dynamicPools[2]: 8,
				p.dynamicPools[3]: 4,
			},
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			value := p.calculatePoolCpuset(tc.requestCpu, tc.remainFree, tc.weight, tc.sumWeight)
			for k, v := range value {
				if v != tc.expectedValue[k] {
					t.Errorf("dynamic pool %v Expected return value %v but got %v", k, tc.expectedValue[k], v)
				}
			}
		})
	}
}
