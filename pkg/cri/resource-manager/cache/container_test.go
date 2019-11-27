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
	"testing"
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
			expectedLen:   0,
		},
		{
			name:          "host /etc",
			hostPath:      "/etc/something",
			containerPath: "/data/something",
			readOnly:      false,
			expectedLen:   0,
		},
		{
			name:          "container /etc",
			hostPath:      "/var/lib/kubelet/pods/0c9bcfc4-c51b-11e9-ac9a-b8aeed7c7427/etc-hosts",
			containerPath: "/etc/hosts",
			readOnly:      false,
			expectedLen:   0,
		},
		{
			name:          "ConfigMap",
			containerPath: "/var/lib/kube-proxy",
			hostPath:      "/var/lib/kubelet/pods/0c9bcfc4-c51b-11e9-ac9a-b8aeed7c7427/volumes/kubernetes.io~configmap/kube-proxy",
			readOnly:      false,
			expectedLen:   0,
		},
		{
			name:          "secret",
			containerPath: "/var/run/secrets/kubernetes.io/serviceaccount",
			hostPath:      "/var/lib/kubelet/pods/0c9bcfc4-c51b-11e9-ac9a-b8aeed7c7427/volumes/kubernetes.io~secret/kube-proxy-token-d9slz",
			readOnly:      false,
			expectedLen:   0,
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
