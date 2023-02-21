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

package blockio

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/intel/cri-resource-manager/pkg/cgroups"
	"github.com/intel/cri-resource-manager/pkg/testutils"
)

var knownIOSchedulers = map[string]bool{
	"bfq":         true,
	"cfq":         true,
	"deadline":    true,
	"kyber":       true,
	"mq-deadline": true,
	"none":        true,
	"noop":        true,
}

// TestGetCurrentIOSchedulers: unit test for getCurrentIOSchedulers()
func TestGetCurrentIOSchedulers(t *testing.T) {
	currentIOSchedulers, err := getCurrentIOSchedulers()
	testutils.VerifyError(t, err, 0, nil)
	for blockDev, ioScheduler := range currentIOSchedulers {
		s, ok := knownIOSchedulers[ioScheduler]
		if !ok || !s {
			t.Errorf("unknown io scheduler %#v on block device %#v", ioScheduler, blockDev)
		}
	}
}

// TestConfigurableBlockDevices: unit tests for configurableBlockDevices()
func TestConfigurableBlockDevices(t *testing.T) {
	sysfsBlockDevs, err := filepath.Glob("/sys/block/*")
	if err != nil {
		sysfsBlockDevs = []string{}
	}
	devBlockDevs := []string{}
	for _, sysfsBlockDev := range sysfsBlockDevs {
		if strings.HasPrefix(sysfsBlockDev, "/sys/block/sd") || strings.HasPrefix(sysfsBlockDev, "/sys/block/vd") {
			devBlockDevs = append(devBlockDevs, strings.Replace(sysfsBlockDev, "/sys/block/", "/dev/", 1))
		}
	}
	devPartitions := []string{}
	for _, devBlockDev := range devBlockDevs {
		devPartitions, _ = filepath.Glob(devBlockDev + "[0-9]")
		if len(devPartitions) > 0 {
			break
		}
	}
	t.Logf("test real block devices: %v", devBlockDevs)
	t.Logf("test partitions: %v", devPartitions)
	tcases := []struct {
		name                    string
		devWildcards            []string
		expectedErrorCount      int
		expectedErrorSubstrings []string
		expectedMatches         int
		disabled                bool
		disabledReason          string
	}{
		{
			name:               "no device wildcards",
			devWildcards:       nil,
			expectedErrorCount: 0,
		},
		{
			name:                    "bad wildcard",
			devWildcards:            []string{"/[-/verybadwildcard]"},
			expectedErrorCount:      1,
			expectedErrorSubstrings: []string{"verybadwildcard", "syntax error"},
		},
		{
			name:                    "not matching wildcard",
			devWildcards:            []string{"/dev/path that should not exist/*"},
			expectedErrorCount:      1,
			expectedErrorSubstrings: []string{"does not match any"},
		},
		{
			name:                    "two wildcards: empty string and a character device",
			devWildcards:            []string{"/dev/null", ""},
			expectedErrorCount:      2,
			expectedErrorSubstrings: []string{"\"/dev/null\" is a character device", "\"\" does not match any"},
		},
		{
			name:                    "not a device or even a file",
			devWildcards:            []string{"/proc", "/proc/meminfo", "/proc/notexistingfile"},
			expectedErrorCount:      3,
			expectedErrorSubstrings: []string{"\"/proc\" is not a device", "\"/proc/meminfo\" is not a device"},
		},
		{
			name:            "real block devices",
			devWildcards:    devBlockDevs,
			expectedMatches: len(devBlockDevs),
		},
		{
			name:                    "partition",
			devWildcards:            devPartitions,
			expectedErrorCount:      len(devPartitions),
			expectedErrorSubstrings: []string{"cannot weight/throttle partitions"},
			disabled:                len(devPartitions) == 0,
			disabledReason:          "no block device partitions found",
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.disabled {
				t.Skip(tc.disabledReason)
			}
			realPlatform := defaultPlatform{}
			bdis, err := realPlatform.configurableBlockDevices(tc.devWildcards)
			testutils.VerifyError(t, err, tc.expectedErrorCount, tc.expectedErrorSubstrings)
			if len(bdis) != tc.expectedMatches {
				t.Errorf("expected %d matching block devices, got %d", tc.expectedMatches, len(bdis))
			}
		})
	}
}

// TestDevicesParametersToOci: unit tests for devicesParametersToOci
func TestDevicesParametersToOci(t *testing.T) {
	// switch real devicesParametersToOci to call mockPlatform.configurableBlockDevices
	currentPlatform = mockPlatform{}
	tcases := []struct {
		name                    string
		dps                     []DevicesParameters
		iosched                 map[string]string
		expectedOci             *cgroups.OciBlockIOParameters
		expectedErrorCount      int
		expectedErrorSubstrings []string
	}{
		{
			name: "all OCI fields",
			dps: []DevicesParameters{
				{
					Weight: "144",
				},
				{
					Devices:           []string{"/dev/sda"},
					ThrottleReadBps:   "1G",
					ThrottleWriteBps:  "2M",
					ThrottleReadIOPS:  "3k",
					ThrottleWriteIOPS: "4",
					Weight:            "50",
				},
			},
			iosched: map[string]string{"/dev/sda": "bfq"},
			expectedOci: &cgroups.OciBlockIOParameters{
				Weight: 144,
				WeightDevice: cgroups.OciDeviceWeights{
					{Major: 11, Minor: 12, Weight: 50},
				},
				ThrottleReadBpsDevice: cgroups.OciDeviceRates{
					{Major: 11, Minor: 12, Rate: 1000000000},
				},
				ThrottleWriteBpsDevice: cgroups.OciDeviceRates{
					{Major: 11, Minor: 12, Rate: 2000000},
				},
				ThrottleReadIOPSDevice: cgroups.OciDeviceRates{
					{Major: 11, Minor: 12, Rate: 3000},
				},
				ThrottleWriteIOPSDevice: cgroups.OciDeviceRates{
					{Major: 11, Minor: 12, Rate: 4},
				},
			},
		},
		{
			name: "later match overrides value",
			dps: []DevicesParameters{
				{
					Devices:         []string{"/dev/sda", "/dev/sdb", "/dev/sdc"},
					ThrottleReadBps: "100",
					Weight:          "110",
				},
				{
					Devices:         []string{"/dev/sdb", "/dev/sdc"},
					ThrottleReadBps: "300",
					Weight:          "330",
				},
				{
					Devices:         []string{"/dev/sdb"},
					ThrottleReadBps: "200",
					Weight:          "220",
				},
			},
			iosched: map[string]string{"/dev/sda": "bfq", "/dev/sdb": "bfq", "/dev/sdc": "cfq"},
			expectedOci: &cgroups.OciBlockIOParameters{
				Weight: -1,
				WeightDevice: cgroups.OciDeviceWeights{
					{Major: 11, Minor: 12, Weight: 110},
					{Major: 21, Minor: 22, Weight: 220},
					{Major: 31, Minor: 32, Weight: 330},
				},
				ThrottleReadBpsDevice: cgroups.OciDeviceRates{
					{Major: 11, Minor: 12, Rate: 100},
					{Major: 21, Minor: 22, Rate: 200},
					{Major: 31, Minor: 32, Rate: 300},
				},
			},
		},
		{
			name: "invalid weights, many errors in different parameter sets",
			dps: []DevicesParameters{
				{
					Weight: "99999",
				},
				{
					Devices: []string{"/dev/sda"},
					Weight:  "1",
				},
				{
					Devices: []string{"/dev/sdb"},
					Weight:  "-2",
				},
			},
			expectedErrorCount: 3,
			expectedErrorSubstrings: []string{
				"(99999) bigger than maximum",
				"(1) smaller than minimum",
				"(-2) smaller than minimum",
			},
		},
		{
			name: "throttling without listing Devices",
			dps: []DevicesParameters{
				{
					ThrottleReadBps:   "100M",
					ThrottleWriteIOPS: "20k",
				},
			},
			expectedErrorCount: 1,
			expectedErrorSubstrings: []string{
				"Devices not listed",
				"\"100M\"",
				"\"20k\"",
			},
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			oci, err := devicesParametersToOci(tc.dps, tc.iosched)
			testutils.VerifyError(t, err, tc.expectedErrorCount, tc.expectedErrorSubstrings)
			if tc.expectedOci != nil {
				testutils.VerifyDeepEqual(t, "OCI parameters", *tc.expectedOci, oci)
			}
		})
	}
}

// mockPlatform implements mock versions of platformInterface functions.
type mockPlatform struct{}

// configurableBlockDevices mock always returns a set of block devices.
func (mpf mockPlatform) configurableBlockDevices(devWildcards []string) ([]BlockDeviceInfo, error) {
	blockDevices := []BlockDeviceInfo{}
	for _, devWildcard := range devWildcards {
		if devWildcard == "/dev/sda" {
			blockDevices = append(blockDevices, BlockDeviceInfo{
				Major:   11,
				Minor:   12,
				DevNode: devWildcard,
				Origin:  fmt.Sprintf("from wildcards %v", devWildcard),
			})
		} else if devWildcard == "/dev/sdb" {
			blockDevices = append(blockDevices, BlockDeviceInfo{
				Major:   21,
				Minor:   22,
				DevNode: devWildcard,
				Origin:  fmt.Sprintf("from wildcards %v", devWildcard),
			})
		} else if devWildcard == "/dev/sdc" {
			blockDevices = append(blockDevices, BlockDeviceInfo{
				Major:   31,
				Minor:   32,
				DevNode: devWildcard,
				Origin:  fmt.Sprintf("from wildcards %v", devWildcard),
			})
		}
	}
	return blockDevices, nil
}
