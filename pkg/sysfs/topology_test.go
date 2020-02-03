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

package sysfs

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func setupTestEnv(t *testing.T) func() {
	pwd, err := os.Getwd()
	if err != nil {
		t.Fatal("unable to get current directory")
	}
	if path, err := filepath.EvalSymlinks(pwd); err == nil {
		pwd = path
	}
	mockRoot = pwd + "/testdata"
	teardown := func() {
		mockRoot = ""
	}
	return teardown
}

func TestMapKeys(t *testing.T) {
	cases := []struct {
		name   string
		input  map[string]bool
		output []string
	}{
		{
			name:   "empty",
			input:  map[string]bool{},
			output: []string{},
		},
		{
			name:   "one",
			input:  map[string]bool{"a": false},
			output: []string{"a"},
		},
		{
			name:   "multiple",
			input:  map[string]bool{"a": false, "b": true, "c": false},
			output: []string{"a", "b", "c"},
		},
	}
	for _, tc := range cases {
		test := tc
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			output := mapKeys(test.input)
			sort.Strings(output)
			if !reflect.DeepEqual(output, test.output) {
				t.Fatalf("expected output: %+v got: %+v", test.output, output)
			}
		})
	}
}

func TestFindSysFsDevice(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}
	teardown := setupTestEnv(t)
	defer teardown()
	cases := []struct {
		name        string
		input       string
		output      string
		expectedErr bool
	}{
		{
			name:        "empty",
			input:       "",
			output:      "",
			expectedErr: false,
		},
		{
			name:        "null",
			input:       "/dev/null",
			output:      "/sys/devices/virtual/mem/null",
			expectedErr: false,
		},
		{
			name:        "proc",
			input:       "/proc/self",
			output:      "",
			expectedErr: true,
		},
	}
	for _, tc := range cases {
		test := tc
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			output, err := FindSysFsDevice(test.input)
			switch {
			case err != nil && !test.expectedErr:
				t.Fatalf("unexpected error returned: %+v", err)
			case err == nil && test.expectedErr:
				t.Fatalf("unexpected success: %+v", output)
			case output != test.output:
				t.Fatalf("expected: %q got: %q", test.output, output)
			}
		})
	}
}

func TestReadFilesInDirectory(t *testing.T) {
	var file, empty string
	fname := "test-a"
	content := []byte(" something\n")
	expectedContent := "something"

	fileMap := map[string]*string{
		fname:          &file,
		"non_existing": &empty,
	}

	dir, err := ioutil.TempDir("", "readFilesInDirectory")
	if err != nil {
		t.Fatalf("unable to create test directory: %+v", err)
	}
	defer os.RemoveAll(dir)
	ioutil.WriteFile(filepath.Join(dir, fname), content, 0644)

	if err = readFilesInDirectory(fileMap, dir); err != nil {
		t.Fatalf("unexpected failure: %v", err)
	}
	if empty != "" {
		t.Fatalf("unexpected content: %q", empty)
	}
	if file != expectedContent {
		t.Fatalf("unexpected content: %q expected: %q", file, expectedContent)
	}
}

func TestGetDevicesFromVirtual(t *testing.T) {
	teardown := setupTestEnv(t)
	defer teardown()

	cases := []struct {
		name        string
		input       string
		output      []string
		expectedErr bool
	}{
		{
			name:        "vfio",
			input:       "/sys/devices/virtual/vfio/42",
			output:      []string{mockRoot + "/sys/devices/pci0000:00/0000:00:02.0"},
			expectedErr: false,
		},
		{
			name:        "misc",
			input:       "/sys/devices/virtual/misc/vfio",
			output:      nil,
			expectedErr: false,
		},
		{
			name:        "missing-iommu-group",
			input:       "/sys/devices/virtual/vfio/84",
			output:      nil,
			expectedErr: true,
		},
		{
			name:        "non-virtual",
			input:       "/sys/devices/pci0000:00/0000:00:02.0",
			output:      nil,
			expectedErr: true,
		},
	}

	for _, tc := range cases {
		test := tc
		t.Run(test.name, func(t *testing.T) {
			output, err := getDevicesFromVirtual(test.input)
			switch {
			case err != nil && !test.expectedErr:
				t.Fatalf("unexpected error returned: %+v", err)
			case err == nil && test.expectedErr:
				t.Fatalf("unexpected success: %+v", output)
			case len(output) != len(test.output):
				t.Fatalf("expected: %q got: %q", len(test.output), len(output))
			}
			for i, p := range test.output {
				if test.output[i] != p {
					t.Fatalf("expected: %q got: %q", test.output[i], p)
				}
			}
		})
	}
}

func TestMergeTopologyHints(t *testing.T) {
	cases := []struct {
		name           string
		inputA         TopologyHints
		inputB         TopologyHints
		expectedOutput TopologyHints
		expectedErr    bool
	}{
		{
			name:           "empty",
			inputA:         nil,
			inputB:         nil,
			expectedOutput: TopologyHints{},
		},
		{
			name:           "one,nil",
			inputA:         TopologyHints{"test": TopologyHint{Provider: "test", CPUs: "0"}},
			inputB:         nil,
			expectedOutput: TopologyHints{"test": TopologyHint{Provider: "test", CPUs: "0"}},
		},
		{
			name:           "nil, one",
			inputA:         nil,
			inputB:         TopologyHints{"test": TopologyHint{Provider: "test", CPUs: "0"}},
			expectedOutput: TopologyHints{"test": TopologyHint{Provider: "test", CPUs: "0"}},
		},
		{
			name:           "duplicate",
			inputA:         TopologyHints{"test": TopologyHint{Provider: "test", CPUs: "0"}},
			inputB:         TopologyHints{"test": TopologyHint{Provider: "test", CPUs: "0"}},
			expectedOutput: TopologyHints{"test": TopologyHint{Provider: "test", CPUs: "0"}},
		},
		{
			name:   "two",
			inputA: TopologyHints{"test1": TopologyHint{Provider: "test1", CPUs: "0"}},
			inputB: TopologyHints{"test2": TopologyHint{Provider: "test2", CPUs: "1"}},
			expectedOutput: TopologyHints{
				"test1": TopologyHint{Provider: "test1", CPUs: "0"},
				"test2": TopologyHint{Provider: "test2", CPUs: "1"},
			},
		},
	}
	for _, tc := range cases {
		test := tc
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			output := MergeTopologyHints(test.inputA, test.inputB)
			if !reflect.DeepEqual(output, test.expectedOutput) {
				t.Fatalf("expected output: %+v got: %+v", test.expectedOutput, output)
			}
		})
	}
}

func TestNewTopologyHints(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}
	teardown := setupTestEnv(t)
	defer teardown()
	cases := []struct {
		name        string
		input       string
		output      TopologyHints
		expectedErr bool
	}{
		{
			name:        "empty",
			input:       "non-existing",
			output:      nil,
			expectedErr: true,
		},
		{
			name:  "pci card1",
			input: mockRoot + "/sys/devices/pci0000:00/0000:00:02.0/drm/card1",
			output: TopologyHints{
				mockRoot + "/sys/devices/pci0000:00/0000:00:02.0": TopologyHint{
					Provider: mockRoot + "/sys/devices/pci0000:00/0000:00:02.0",
					CPUs:     "0-7",
					NUMAs:    "",
					Sockets:  ""},
			},
			expectedErr: false,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			output, err := NewTopologyHints(test.input)
			switch {
			case err != nil && !test.expectedErr:
				t.Fatalf("unexpected error returned: %+v", err)
			case err == nil && test.expectedErr:
				t.Fatalf("unexpected success: %+v", output)
			case !reflect.DeepEqual(output, test.output):
				t.Fatalf("expected: %q got: %q", test.output, output)
			}
		})
	}
}
