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
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"
)

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

// createDirectoryTree creates files and directories with the provided content
// it creates empty directory if file content is set to nil
func createDirectoryTree(root string, files map[string][]byte) error {
	for filePath, content := range files {
		fullPath := path.Join(root, filePath)
		// create directory
		dirPath := path.Dir(fullPath)
		err := os.MkdirAll(dirPath, 0755)
		if err != nil {
			return err
		}
		// create file if content is provided
		if content != nil {
			err := ioutil.WriteFile(fullPath, content, 0644)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func TestNewTopologyHints(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	// prepare test data
	mockRoot = fmt.Sprintf("/tmp/cri-rm-test-%d", time.Now().Unix())
	sysFSTree := map[string][]byte{
		"sys/devices/pci0000:00/0000:00:02.0/local_cpulist":      []byte("0-7"),
		"sys/devices/pci0000:00/0000:00:02.0/device":             []byte("0x5912"),
		"sys/devices/pci0000:00/0000:00:02.0/local_cpus":         []byte("ff"),
		"sys/devices/pci0000:00/0000:00:02.0/numa_node":          []byte("-1"),
		"sys/devices/pci0000:00/0000:00:02.0/drm/renderD129/dev": []byte("226:129"),
		"sys/devices/pci0000:00/0000:00:02.0/drm/card1/dev":      []byte("226:1"),
		"sys/devices/pci0000:00/0000:00:02.0/vendor":             []byte("0x8086"),
		"sys/devices/pci0000:00/0000:00:02.0/class":              []byte("0x030000"),
	}
	err := createDirectoryTree(mockRoot, sysFSTree)
	if err != nil {
		t.Fatalf("Failed to create tmp mockRoot sysfs tree %s: %+v", mockRoot, err)
	}

	// defer test data cleanup
	defer func() {
		err := os.RemoveAll(mockRoot)
		if err != nil {
			t.Fatalf("Failed to remove tmp mockRoot sysfs tree %s: %+v", mockRoot, err)
		}
		mockRoot = ""
	}()

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
