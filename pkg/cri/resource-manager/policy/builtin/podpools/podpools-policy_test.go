// Copyright 2020-2021 Intel Corporation. All Rights Reserved.
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

package podpools

import (
	"fmt"
	"strings"
	"testing"

	"github.com/intel/cri-resource-manager/pkg/cpuallocator"
	"github.com/intel/cri-resource-manager/pkg/utils/cpuset"
)

func validateError(t *testing.T, expectedError string, err error) bool {
	if expectedError != "" {
		if err == nil {
			t.Errorf("Expected error containing %q, did not get any error", expectedError)
			return false
		} else if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("Expected error containing %q, but got %q", expectedError, err.Error())
			return false
		}
	} else {
		if err != nil {
			t.Errorf("Unexpected error %s", err)
			return false
		}
	}
	return true
}

func assertEqualPools(t *testing.T, expectedPool, gotPool Pool) bool {
	if expectedPool.String() != gotPool.String() {
		// Compares Def.Name, Def.Instance, .CPUs, .Mems, Def.MaxPods
		// and assigned pods/containers.
		t.Errorf("expected pool %s, got %s", expectedPool, gotPool)
		return false
	}
	if expectedPool.Def.Instances != gotPool.Def.Instances {
		t.Errorf("pools %s: PoolDef.Instances differ: expected %q, got %q", expectedPool, expectedPool.Def.Instances, gotPool.Def.Instances)
		return false
	}
	if expectedPool.Def.FillOrder != gotPool.Def.FillOrder {
		t.Errorf("pools %s: PoolDef.FillOrder differ: expected %s, got %s", expectedPool, expectedPool.Def.FillOrder, gotPool.Def.FillOrder)
		return false
	}
	return true
}

type mockCpuAllocator struct{}

func (mca *mockCpuAllocator) AllocateCpus(from *cpuset.CPUSet, cnt int, dontcare cpuallocator.CPUPriority) (cpuset.CPUSet, error) {
	switch {
	case from.Size() < cnt:
		return cpuset.New(), fmt.Errorf("cpuset %s does not have %d CPUs", from, cnt)
	case from.Size() == cnt:
		result := from.Clone()
		*from = cpuset.New()
		return result, nil
	default:
		result := cpuset.New()
		for _, cpu := range from.List() {
			if result.Size() >= cnt {
				break
			}
			result = result.Union(cpuset.New(cpu))
		}
		*from = from.Difference(result)
		return result, nil
	}
}

func (mca *mockCpuAllocator) ReleaseCpus(*cpuset.CPUSet, int, cpuallocator.CPUPriority) (cpuset.CPUSet, error) {
	return cpuset.New(), nil
}

func TestApplyPoolDef(t *testing.T) {
	reservedCpus1 := cpuset.CPUSet{}
	reservedPoolDef := PoolDef{
		Name: reservedPoolDefName,
	}
	defaultPoolDef := PoolDef{
		Name: defaultPoolDefName,
	}
	reservedPool := Pool{
		Def:  &reservedPoolDef,
		CPUs: reservedCpus1,
	}
	defaultPool := Pool{
		Def:  &defaultPoolDef,
		CPUs: reservedCpus1,
	}
	normalPoolsAtStart := []Pool{reservedPool, defaultPool}
	singlecpuSingleInstance := PoolDef{
		Name: "singlecpu",
		CPU:  "1",
	}
	quadcpuDualInstance := PoolDef{
		Name:      "quadcpu",
		CPU:       "4",
		Instances: "8 CPUs",
	}
	quadcpuMultiInstance := PoolDef{
		Name:      "quadcpu",
		CPU:       "4",
		Instances: "100%",
	}
	tcases := []struct {
		name             string
		pools            *[]Pool
		poolDef          PoolDef
		freeCpus         string // example: "0-2"
		expectedFreeCpus string // "": no check, "-": assert empty
		expectedError    string // "": error is not allowed, otherwise expected error substring
		expectedPools    *[]Pool
	}{
		// negative tests
		{
			name:          "call apply without built-in pools",
			pools:         &([]Pool{}),
			poolDef:       singlecpuSingleInstance,
			freeCpus:      "0-3",
			expectedError: "pools missing",
		},
		{
			name: "bad reserved CPUs",
			poolDef: PoolDef{
				Name: "reserved",
				CPU:  "two",
			},
			expectedError: "invalid CPUs",
		},
		{
			name: "bad reserved Instances",
			poolDef: PoolDef{
				Name:      "reserved",
				CPU:       "1",
				Instances: "0x",
			},
			expectedError: "invalid Instances",
		},
		{
			name: "bad default CPUs",
			poolDef: PoolDef{
				Name: "default",
				CPU:  "2500m",
			},
			freeCpus:      "0-8",
			expectedError: "invalid CPUs",
		},
		{
			name: "bad default Instances",
			poolDef: PoolDef{
				Name:      "default",
				CPU:       "0xf",
				Instances: "100 % CPUs",
			},
			freeCpus:      "0-95",
			expectedError: "invalid Instances",
		},
		{
			name: "bad user-defined CPUs",
			poolDef: PoolDef{
				Name: "mypool",
			},
			freeCpus:      "0-8",
			expectedError: "missing CPUs",
		},
		{
			name: "too many CPUs on user-defined Instances",
			poolDef: PoolDef{
				Name:      "user pool",
				CPU:       "1",
				Instances: "100 CPUs",
			},
			freeCpus:      "0-95",
			expectedError: "insufficient CPUs",
		},
		{
			name: "unnamed pool",
			poolDef: PoolDef{
				CPU:     "1",
				MaxPods: 1,
			},
			freeCpus:      "0-3",
			expectedError: "undefined or empty pool name",
		},
		{
			name: "unreachable pools",
			poolDef: PoolDef{
				Name:      "unlimited capacity",
				CPU:       "3",
				MaxPods:   0,
				FillOrder: FillPacked,
				Instances: "3",
			},
			freeCpus:      "0-95",
			expectedError: "2 pool(s) unreachable",
		},
		// redefine the reserved pool
		{
			name: "redefine reserved CPUs",
			poolDef: PoolDef{
				Name: "reserved",
				CPU:  "2",
			},
			freeCpus:      "0-3",
			expectedError: "conflicting ReservedResources CPUs",
		},
		{
			name: "redefine reserved instances",
			poolDef: PoolDef{
				Name:      "reserved",
				CPU:       "1",
				Instances: "2",
			},
			freeCpus:      "0-3",
			expectedError: "cannot change the number of instances",
		},
		{
			name: "redefine reserved MaxPods",
			poolDef: PoolDef{
				Name:    "reserved",
				MaxPods: 42,
			},
			freeCpus: "0-3",
			expectedPools: &[]Pool{
				{
					Def: &PoolDef{
						Name:    reservedPoolDefName,
						MaxPods: 42,
					},
					CPUs: reservedPool.CPUs,
				},
				defaultPool,
			},
		},
		// redefine the default pool
		{
			name: "redefine default CPUs",
			poolDef: PoolDef{
				Name: "default",
				CPU:  "2",
			},
			freeCpus:         "0-3",
			expectedFreeCpus: "2-3",
			expectedPools: &[]Pool{
				reservedPool,
				{
					Def: &PoolDef{
						Name: defaultPoolDefName,
					},
					CPUs: cpuset.MustParse("0-1"),
				},
			},
		},
		{
			name: "redefine default instances",
			poolDef: PoolDef{
				Name:      "default",
				CPU:       "1",
				Instances: "2",
			},
			freeCpus:      "0-3",
			expectedError: "cannot change the number of instances",
		},
		{
			name: "redefine default MaxPods",
			poolDef: PoolDef{
				Name:    "default",
				MaxPods: 52,
			},
			freeCpus: "0-3",
			expectedPools: &[]Pool{
				reservedPool,
				{
					Def: &PoolDef{
						Name:    defaultPoolDefName,
						MaxPods: 52,
					},
					CPUs: defaultPool.CPUs,
				},
			},
		},
		// user-defined pools
		{
			name:          "use one CPUs - insufficient",
			poolDef:       singlecpuSingleInstance,
			expectedError: "insufficient CPUs",
		},
		{
			name:             "use one CPU",
			freeCpus:         "0-3",
			poolDef:          singlecpuSingleInstance,
			expectedFreeCpus: "1-3",
			expectedPools: &[]Pool{
				reservedPool,
				defaultPool,
				{
					Def:      &singlecpuSingleInstance,
					Instance: 0,
					CPUs:     cpuset.MustParse("0"),
				},
			},
		},
		{
			name:             "use the only CPU",
			freeCpus:         "0",
			poolDef:          singlecpuSingleInstance,
			expectedFreeCpus: "-",
		},
		{
			name:          "use 2x4 CPUs - insufficient",
			freeCpus:      "0-6",
			poolDef:       quadcpuDualInstance,
			expectedError: "insufficient CPUs",
		},
		{
			name:             "use 2x4 CPUs - consume all",
			freeCpus:         "0-7",
			poolDef:          quadcpuDualInstance,
			expectedFreeCpus: "-",
		},
		{
			name:             "use 2x4 CPUs - CPUs left",
			freeCpus:         "0-8",
			poolDef:          quadcpuDualInstance,
			expectedFreeCpus: "8",
		},
		{
			name:          "use all cpus - but insufficient",
			freeCpus:      "0-2",
			poolDef:       quadcpuMultiInstance,
			expectedError: "insufficient CPUs",
		},
		{
			name:             "use all cpus - partial",
			freeCpus:         "0-6",
			poolDef:          quadcpuMultiInstance,
			expectedFreeCpus: "4-6",
			expectedPools: &[]Pool{
				reservedPool,
				defaultPool,
				{
					Def:      &quadcpuMultiInstance,
					Instance: 0,
					CPUs:     cpuset.MustParse("0-3"),
				},
			},
		},
		{
			name:             "use all cpus - every single one",
			freeCpus:         "0-7",
			poolDef:          quadcpuMultiInstance,
			expectedFreeCpus: "-",
			expectedPools: &[]Pool{
				reservedPool,
				defaultPool,
				{
					Def:      &quadcpuMultiInstance,
					Instance: 0,
					CPUs:     cpuset.MustParse("0-3"),
				}, {
					Def:      &quadcpuMultiInstance,
					Instance: 1,
					CPUs:     cpuset.MustParse("4-7"),
				},
			},
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			// Tests should not change original pools/pooldefs/freeCpus
			// Create copies before calling the function.
			pools := []*Pool{}
			if tc.pools == nil {
				tc.pools = &normalPoolsAtStart
			}
			for i := range *tc.pools {
				copyOfPool := (*tc.pools)[i]
				pools = append(pools, &copyOfPool)
			}
			freeCpus := cpuset.New()
			if tc.freeCpus != "" {
				freeCpus = cpuset.MustParse(tc.freeCpus)
			}
			p := &podpools{
				cpuAllocator: &mockCpuAllocator{},
			}
			err := p.applyPoolDef(&pools, &tc.poolDef, &freeCpus, freeCpus.Size())
			if ok := validateError(t, tc.expectedError, err); ok {
				// check freeCpus modified by applyPoolDef
				if tc.expectedFreeCpus != "" {
					expectedFreeCpus := cpuset.New()
					if tc.expectedFreeCpus != "-" {
						expectedFreeCpus = cpuset.MustParse(tc.expectedFreeCpus)
					}
					if expectedFreeCpus.Size() != freeCpus.Size() {
						t.Errorf("unexpected number of free CPUs left, expected %d, got %d", expectedFreeCpus.Size(), freeCpus.Size())
					}
				}
				// check pools modified by applyPoolDef
				if tc.expectedPools != nil {
					if len(pools) != len(*tc.expectedPools) {
						t.Errorf("unexpected number of new pools, expected %d got %d", len(pools), len(*tc.expectedPools))
						return
					}
					for i := 0; i < len(pools); i++ {
						if !assertEqualPools(t, (*tc.expectedPools)[i], *pools[i]) {
							return
						}
					}
				}
			}
		})
	}
}

func TestParseInstancesCPUs(t *testing.T) {
	tcases := []struct {
		name              string
		instances         string
		cpus              string
		freeCpus          int
		expectedInstances int
		expectedCPUs      int
		expectedError     string
	}{
		{
			name:          "empty CPUs",
			expectedError: "missing CPUs",
		},
		{
			name:          "bad CPUs",
			cpus:          "55%",
			expectedError: "> 1 expected",
		},
		{
			name:          "zero CPUs",
			cpus:          "0",
			expectedError: "> 1 expected",
		},
		{
			name:          "negative CPUs",
			cpus:          "-1",
			expectedError: "> 1 expected",
		},
		{
			name:              "42 CPUs, empty instances defaults to 1",
			cpus:              "42",
			expectedCPUs:      42,
			expectedInstances: 1,
		},
		{
			name:              "instances: 0",
			instances:         "0",
			cpus:              "2",
			freeCpus:          100,
			expectedInstances: 0,
			expectedCPUs:      2,
		},
		{
			name:              "instances: N",
			instances:         "10",
			cpus:              "2",
			freeCpus:          100,
			expectedInstances: 10,
			expectedCPUs:      2,
		},
		{
			name:              "instances: N CPUs",
			instances:         "10 CPUs",
			cpus:              "2",
			freeCpus:          100,
			expectedInstances: 10 / 2,
			expectedCPUs:      2,
		},
		{
			name:              "instances: 1 CPUS",
			instances:         "1 CPUS",
			cpus:              "1",
			freeCpus:          1,
			expectedInstances: 1,
			expectedCPUs:      1,
		},
		{
			name:              "instances: 1 cpu",
			instances:         "1 cpu",
			cpus:              "1",
			freeCpus:          2,
			expectedInstances: 1,
			expectedCPUs:      1,
		},
		{
			name:              "instances: 8cpu",
			instances:         "8cpu",
			cpus:              "2",
			freeCpus:          9,
			expectedInstances: 4,
			expectedCPUs:      2,
		},
		{
			name:              "instances: N %",
			instances:         "90 %",
			cpus:              "2",
			freeCpus:          10,
			expectedInstances: 4, // 10 * (90/100) / 2
			expectedCPUs:      2,
		},
		{
			name:              "instances: N%",
			instances:         "90%",
			cpus:              "90",
			freeCpus:          100,
			expectedInstances: 1,
			expectedCPUs:      90,
		},
		{
			name:              "instances: N %, not enough for any pools",
			instances:         "10 %",
			cpus:              "2",
			freeCpus:          10,
			expectedInstances: 0, // 10 * (10/100) / 2
			expectedCPUs:      2,
		},
		{
			name:          "instances: -N",
			instances:     "-10",
			cpus:          "2",
			expectedError: "invalid Instances",
		},
		{
			name:          "instances: -N CPUs",
			instances:     "-10 CPUs",
			cpus:          "2",
			expectedError: "invalid Instances",
		},
		{
			name:          "instances: N CPUs CPU",
			instances:     "2 CPUs CPU",
			cpus:          "2",
			expectedError: "invalid Instances",
		},
		{
			name:          "instances: -N %",
			instances:     "-10 %",
			cpus:          "2",
			expectedError: "invalid Instances",
		},
		{
			name:          "instances: N CPUs, N < cpus",
			instances:     "3 CPUs",
			cpus:          "4",
			expectedError: "insufficient CPUs",
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			instances, cpus, err := parseInstancesCPUs(tc.instances, tc.cpus, tc.freeCpus)
			if ok := validateError(t, tc.expectedError, err); ok {
				if instances != tc.expectedInstances || cpus != tc.expectedCPUs {
					t.Errorf("Expected (instances, cpus) (%v, %v), but got (%v, %v)", tc.expectedInstances, tc.expectedCPUs, instances, cpus)
				}
			}
		})
	}
}
