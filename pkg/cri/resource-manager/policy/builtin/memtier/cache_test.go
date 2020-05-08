// Copyright 2019 Intel Corporation. All Rights Reserved.
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
	"bytes"
	"testing"

	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
)

func TestToGrant(t *testing.T) {
	tcases := []struct {
		name          string
		policy        *policy
		cgrant        *cachedGrant
		expectedError bool
	}{
		{
			name:   "unknown node",
			cgrant: &cachedGrant{},
			policy: &policy{
				nodes: map[string]Node{
					"node1": &node{},
				},
			},
			expectedError: true,
		},
		{
			name: "known node but failed lookup",
			cgrant: &cachedGrant{
				Pool: "node1",
			},
			policy: &policy{
				nodes: map[string]Node{
					"node1": &node{},
				},
				cache: &mockCache{},
			},
			expectedError: true,
		},
		{
			name: "known node",
			cgrant: &cachedGrant{
				Pool: "node1",
			},
			policy: &policy{
				nodes: map[string]Node{
					"node1": &node{},
				},
				cache: &mockCache{
					returnValue2ForLookupContainer: true,
				},
			},
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.cgrant.ToGrant(tc.policy)
			if tc.expectedError && err == nil {
				t.Errorf("Expected error, but got success")
			}
			if !tc.expectedError && err != nil {
				t.Errorf("Unxpected error: %+v", err)
			}
		})
	}
}

func TestAllocationMarshalling(t *testing.T) {
	tcases := []struct {
		name                       string
		data                       []byte
		expectedUnmarshallingError bool
		expectedMarshallingError   bool
	}{
		{
			name: "non-zero Exclusive",
			data: []byte(`{"key1":{"Exclusive":"1","Part":1,"Container":"1","Pool":"testnode","MemoryPool":"testnode","MemType":"DRAM,PMEM,HBM","Memset":"","MemoryLimit":{},"ColdStart":0}}`),
		},
		{
			name: "zero Exclusive",
			data: []byte(`{"key1":{"Exclusive":"","Part":1,"Container":"1","Pool":"testnode","MemoryPool":"testnode","MemType":"DRAM,PMEM,HBM","Memset":"","MemoryLimit":{},"ColdStart":0}}`),
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			alloc := &allocations{
				policy: &policy{
					nodes: map[string]Node{
						"testnode": &virtualnode{
							node: node{
								name:    "testnode",
								kind:    UnknownNode,
								noderes: newSupply(&node{}, cpuset.NewCPUSet(), cpuset.NewCPUSet(), 0, createMemoryMap(0, 0, 0), createMemoryMap(0, 0, 0)),
								freeres: newSupply(&node{}, cpuset.NewCPUSet(), cpuset.NewCPUSet(), 0, createMemoryMap(0, 0, 0), createMemoryMap(0, 0, 0)),
							},
						},
					},
					cache: &mockCache{
						returnValue1ForLookupContainer: &mockContainer{
							returnValueForGetCacheID: "1",
						},
						returnValue2ForLookupContainer: true,
					},
				},
			}
			unmarshallingErr := alloc.UnmarshalJSON(tc.data)
			if tc.expectedUnmarshallingError && unmarshallingErr == nil {
				t.Errorf("Expected unmarshalling error, but got success")
			}
			if !tc.expectedUnmarshallingError && unmarshallingErr != nil {
				t.Errorf("Unxpected unmarshalling error: %+v", unmarshallingErr)
			}

			out, marshallingErr := alloc.MarshalJSON()
			if !bytes.Equal(out, tc.data) {
				t.Errorf("Expected\n%q\nBut got\n%q", tc.data, out)
			}
			if tc.expectedMarshallingError && marshallingErr == nil {
				t.Errorf("Expected marshalling error, but got success")
			}
			if !tc.expectedMarshallingError && marshallingErr != nil {
				t.Errorf("Unxpected marshalling error: %+v", marshallingErr)
			}

		})
	}
}
