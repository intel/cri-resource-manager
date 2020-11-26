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
	"testing"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	resapi "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPodIsolationPreference(t *testing.T) {
	tcases := []struct {
		name             string
		pod              *mockPod
		container        *mockContainer
		expectedIsolate  bool
		expectedExplicit bool
		disabled         bool
	}{
		{
			name:     "podIsolationPreference() should handle nil pod arg gracefully",
			disabled: true,
		},
		{
			name:            "return defaults",
			pod:             &mockPod{},
			expectedIsolate: opt.PreferIsolated,
		},
		{
			name: "prefer resmgr's annotation value",
			pod: &mockPod{
				returnValue1FotGetResmgrAnnotation: "true",
				returnValue2FotGetResmgrAnnotation: true,
			},
			expectedIsolate:  true,
			expectedExplicit: true,
		},
		{
			name: "return defaults for unparsable",
			pod: &mockPod{
				returnValue1FotGetResmgrAnnotation: "UNPARSABLE",
				returnValue2FotGetResmgrAnnotation: true,
			},
			expectedIsolate: opt.PreferIsolated,
		},
		{
			name: "podIsolationPreference() should handle nil container arg gracefully",
			pod: &mockPod{
				returnValue1FotGetResmgrAnnotation: "key: true",
				returnValue2FotGetResmgrAnnotation: true,
			},
			disabled: true,
		},
		{
			name: "return defaults for missing preferences",
			pod: &mockPod{
				returnValue1FotGetResmgrAnnotation: "key: true",
				returnValue2FotGetResmgrAnnotation: true,
			},
			container:       &mockContainer{},
			expectedIsolate: opt.PreferIsolated,
		},
		{
			name: "return defined preferences",
			pod: &mockPod{
				returnValue1FotGetResmgrAnnotation: "testcontainer: false",
				returnValue2FotGetResmgrAnnotation: true,
			},
			container: &mockContainer{
				name: "testcontainer",
			},
			expectedExplicit: true,
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.disabled {
				t.Skipf("The case '%s' is skipped", tc.name)
			}
			isolate, explicit := podIsolationPreference(tc.pod, tc.container)
			if isolate != tc.expectedIsolate || explicit != tc.expectedExplicit {
				t.Errorf("Expected (%v, %v), but got (%v, %v)", tc.expectedIsolate, tc.expectedExplicit, isolate, explicit)
			}
		})
	}
}

func TestPodSharedCPUPreference(t *testing.T) {
	tcases := []struct {
		name           string
		pod            *mockPod
		container      *mockContainer
		expectedShared bool
		disabled       bool
	}{
		{
			name:     "podSharedCPUPreference() should handle nil pod arg gracefully",
			disabled: true,
		},
		{
			name:           "return defaults",
			pod:            &mockPod{},
			expectedShared: opt.PreferShared,
		},
		{
			name: "prefer resmgr's annotation value",
			pod: &mockPod{
				returnValue1FotGetResmgrAnnotation: "true",
				returnValue2FotGetResmgrAnnotation: true,
			},
			expectedShared: true,
		},
		{
			name: "return defaults for unparsable",
			pod: &mockPod{
				returnValue1FotGetResmgrAnnotation: "UNPARSABLE",
				returnValue2FotGetResmgrAnnotation: true,
			},
			expectedShared: opt.PreferShared,
		},
		{
			name: "podSharedCPUPreference() should handle nil container arg gracefully",
			pod: &mockPod{
				returnValue1FotGetResmgrAnnotation: "key: true",
				returnValue2FotGetResmgrAnnotation: true,
			},
			disabled: true,
		},
		{
			name: "return defaults for missing preferences",
			pod: &mockPod{
				returnValue1FotGetResmgrAnnotation: "key: true",
				returnValue2FotGetResmgrAnnotation: true,
			},
			container:      &mockContainer{},
			expectedShared: opt.PreferShared,
		},
		{
			name: "return defined preferences",
			pod: &mockPod{
				returnValue1FotGetResmgrAnnotation: "testcontainer: false",
				returnValue2FotGetResmgrAnnotation: true,
			},
			container: &mockContainer{
				name: "testcontainer",
			},
		},
		{
			name: "return defaults for unparsable annotation value",
			pod: &mockPod{
				returnValue1FotGetResmgrAnnotation: "testcontainer: UNPARSABLE",
				returnValue2FotGetResmgrAnnotation: true,
			},
			container: &mockContainer{
				name: "testcontainer",
			},
			expectedShared: opt.PreferShared,
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.disabled {
				t.Skipf("The case '%s' is skipped", tc.name)
			}
			shared := podSharedCPUPreference(tc.pod, tc.container)
			if shared != tc.expectedShared {
				t.Errorf("Expected %v, but got %v", tc.expectedShared, shared)
			}
		})
	}
}

func TestCpuAllocationPreferences(t *testing.T) {
	tcases := []struct {
		name             string
		pod              *mockPod
		container        *mockContainer
		expectedFull     int
		expectedFraction int
		expectedIsolate  bool
		expectedCpuType  cpuClass
		disabled         bool
	}{
		{
			name:     "cpuAllocationPreferences() should handle nil container arg gracefully",
			disabled: true,
		},
		{
			name:      "no resource requirements",
			container: &mockContainer{},
		},
		{
			name: "cpuAllocationPreferences() should handle nil pod arg gracefully",
			container: &mockContainer{
				returnValueForGetResourceRequirements: v1.ResourceRequirements{
					Requests: v1.ResourceList{
						corev1.ResourceCPU: resapi.MustParse("1"),
					},
				},
			},
			disabled: true,
		},
		{
			name: "return defaults",
			container: &mockContainer{
				returnValueForGetResourceRequirements: v1.ResourceRequirements{
					Requests: v1.ResourceList{
						corev1.ResourceCPU: resapi.MustParse("1"),
					},
				},
			},
			pod: &mockPod{},
		},
		{
			name: "return request's value for system container",
			container: &mockContainer{
				namespace: metav1.NamespaceSystem,
				returnValueForGetResourceRequirements: v1.ResourceRequirements{
					Requests: v1.ResourceList{
						corev1.ResourceCPU: resapi.MustParse("2"),
					},
				},
			},
			pod:              &mockPod{},
			expectedFraction: 2000,
			expectedCpuType:  cpuReserved,
		},
		{
			name: "return request's value for burstable QoS",
			container: &mockContainer{
				returnValueForGetResourceRequirements: v1.ResourceRequirements{
					Requests: v1.ResourceList{
						corev1.ResourceCPU: resapi.MustParse("2"),
					},
				},
			},
			pod: &mockPod{
				returnValueFotGetQOSClass: corev1.PodQOSBurstable,
			},
			expectedFraction: 2000,
		},
		{
			name: "return request's value for guaranteed QoS",
			container: &mockContainer{
				returnValueForGetResourceRequirements: v1.ResourceRequirements{
					Requests: v1.ResourceList{
						corev1.ResourceCPU: resapi.MustParse("2"),
					},
				},
			},
			pod: &mockPod{
				returnValueFotGetQOSClass: corev1.PodQOSGuaranteed,
			},
			expectedFull: 2,
		},
		{
			name: "return request's value for guaranteed QoS and isolate",
			container: &mockContainer{
				returnValueForGetResourceRequirements: v1.ResourceRequirements{
					Requests: v1.ResourceList{
						corev1.ResourceCPU: resapi.MustParse("1"),
					},
				},
			},
			pod: &mockPod{
				returnValueFotGetQOSClass: corev1.PodQOSGuaranteed,
			},
			expectedFull:    1,
			expectedIsolate: true,
		},
		{
			name: "return request's value for guaranteed QoS and no isolate",
			container: &mockContainer{
				name: "testcontainer",
				returnValueForGetResourceRequirements: v1.ResourceRequirements{
					Requests: v1.ResourceList{
						corev1.ResourceCPU: resapi.MustParse("1"),
					},
				},
			},
			pod: &mockPod{
				returnValueFotGetQOSClass:          corev1.PodQOSGuaranteed,
				returnValue1FotGetResmgrAnnotation: "testcontainer: false",
				returnValue2FotGetResmgrAnnotation: true,
			},
			expectedFull: 1,
		},
		{
			name: "prefer shared",
			container: &mockContainer{
				name: "testcontainer",
				returnValueForGetResourceRequirements: v1.ResourceRequirements{
					Requests: v1.ResourceList{
						corev1.ResourceCPU: resapi.MustParse("2"),
					},
				},
			},
			pod: &mockPod{
				returnValueFotGetQOSClass:          corev1.PodQOSGuaranteed,
				returnValue1FotGetResmgrAnnotation: "testcontainer: true",
				returnValue2FotGetResmgrAnnotation: true,
			},
			expectedFull:     0,
			expectedFraction: 2000,
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.disabled {
				t.Skipf("The case '%s' is skipped", tc.name)
			}
			full, fraction, isolate, cpuType := cpuAllocationPreferences(tc.pod, tc.container)
			if full != tc.expectedFull || fraction != tc.expectedFraction ||
				isolate != tc.expectedIsolate || cpuType != tc.expectedCpuType {
				t.Errorf("Expected (%v, %v, %v, %s), but got (%v, %v, %v, %s)",
					tc.expectedFull, tc.expectedFraction, tc.expectedIsolate, tc.expectedCpuType,
					full, fraction, isolate, cpuType)
			}
		})
	}
}
