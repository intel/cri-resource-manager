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

package cache

import (
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestGetKubeletHint(t *testing.T) {
	type T struct {
		name        string
		cpus        string
		mems        string
		expectedLen int
	}

	cases := []T{
		{
			name:        "empty",
			cpus:        "",
			mems:        "",
			expectedLen: 0,
		},
		{
			name:        "cpus",
			cpus:        "0-9",
			mems:        "",
			expectedLen: 1,
		},
		{
			name:        "mems",
			cpus:        "",
			mems:        "0,1",
			expectedLen: 1,
		},
		{
			name:        "both",
			cpus:        "0-9",
			mems:        "0,1",
			expectedLen: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			output := getKubeletHint(tc.cpus, tc.mems)
			if len(output) != tc.expectedLen {
				t.Errorf("expected len of hints: %d, got: %d, hints: %+v", tc.expectedLen, len(output), output)
			}
		})
	}
}

func TestGetTopologyHints(t *testing.T) {
	type T struct {
		name          string
		hostPath      string
		containerPath string
		readOnly      bool
		expectedLen   int
	}

	cases := []T{
		{
			name:          "read-only",
			hostPath:      "/something",
			containerPath: "/something",
			readOnly:      true,
		},
		{
			name:          "host /etc",
			hostPath:      "/etc/something",
			containerPath: "/data/something",
		},
		{
			name:          "container /etc",
			hostPath:      "/var/lib/kubelet/pods/0c9bcfc4-c51b-11e9-ac9a-b8aeed7c7427/etc-hosts",
			containerPath: "/etc/hosts",
		},
		{
			name:          "ConfigMap",
			containerPath: "/var/lib/kube-proxy",
			hostPath:      "/var/lib/kubelet/pods/0c9bcfc4-c51b-11e9-ac9a-b8aeed7c7427/volumes/kubernetes.io~configmap/kube-proxy",
		},
		{
			name:          "secret",
			containerPath: "/var/run/secrets/kubernetes.io/serviceaccount",
			hostPath:      "/var/lib/kubelet/pods/0c9bcfc4-c51b-11e9-ac9a-b8aeed7c7427/volumes/kubernetes.io~secret/kube-proxy-token-d9slz",
		},
		{
			name:          "dev null",
			hostPath:      "/dev/null",
			containerPath: "/dev/null",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			output := getTopologyHints(tc.hostPath, tc.containerPath, tc.readOnly)
			if len(output) != tc.expectedLen {
				t.Errorf("expected len of hints: %d, got: %d, hints: %+v", tc.expectedLen, len(output), output)
			}
		})
	}
}

func TestKeysInNamespace(t *testing.T) {
	testMap := map[string]string{
		"no-namespace":               "",
		"my.name.space":              "",
		"my.name.space/key-1":        "",
		"my.name.space/key-2":        "",
		"other.name.space/other-key": "",
	}
	tcases := []struct {
		name          string
		collectionMap map[string]string
		namespace     string
		expectedKeys  []string
	}{
		{
			name: "empty map should return nothing for empty namespace",
		},
		{
			name:      "empty map should return nothing",
			namespace: "my.name.space",
		},
		{
			name:          "keys with no namespace",
			collectionMap: testMap,
			expectedKeys:  []string{"my.name.space", "no-namespace"},
		},
		{
			name:          "keys in namespace",
			collectionMap: testMap,
			namespace:     "my.name.space",
			expectedKeys:  []string{"key-1", "key-2"},
		},
	}

	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			keys := keysInNamespace(tc.collectionMap, tc.namespace)
			sort.Strings(keys)
			if !cmp.Equal(keys, tc.expectedKeys, cmpopts.EquateEmpty()) {
				t.Errorf("Expected %v, received %v", tc.expectedKeys, keys)
			}
		})
	}
}
