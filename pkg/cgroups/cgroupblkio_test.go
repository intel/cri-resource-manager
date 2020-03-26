// Copyright 2020 Intel Corporation. All Rights Reserved.
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

package cgroups

import (
	"fmt"
	"testing"

	"github.com/intel/cri-resource-manager/pkg/testutils"
)

// TestSetBlkioParameters: unit test for SetBlkioParameters()
func TestSetBlkioParameters(t *testing.T) {
	tcases := []struct {
		name                    string
		cgroupsDir              string
		blockIO                 OciBlockIOParameters
		writesFail              int
		expectedFsWrites        map[string]string
		expectedErrorCount      int
		expectedErrorSubstrings []string
	}{
		{
			name:       "write full OCI struct",
			cgroupsDir: "/my/full",
			blockIO: OciBlockIOParameters{
				Weight:                  10,
				WeightDevice:            []OciWeightDeviceParameters{{Major: 1, Minor: 2, Weight: 3}},
				ThrottleReadBpsDevice:   []OciRateDeviceParameters{{Major: 11, Minor: 12, Rate: 13}},
				ThrottleWriteBpsDevice:  []OciRateDeviceParameters{{Major: 21, Minor: 22, Rate: 23}},
				ThrottleReadIOPSDevice:  []OciRateDeviceParameters{{Major: 31, Minor: 32, Rate: 33}},
				ThrottleWriteIOPSDevice: []OciRateDeviceParameters{{Major: 41, Minor: 42, Rate: 43}},
			},
			expectedFsWrites: map[string]string{
				"/my/full/blkio.bfq.weight":                 "10",
				"/my/full/blkio.bfq.weight_device":          "1:2 3",
				"/my/full/blkio.throttle.read_bps_device":   "11:12 13",
				"/my/full/blkio.throttle.write_bps_device":  "21:22 23",
				"/my/full/blkio.throttle.read_iops_device":  "31:32 33",
				"/my/full/blkio.throttle.write_iops_device": "41:42 43",
			},
		},
		{
			name:       "write empty struct",
			cgroupsDir: "/my/empty",
			blockIO:    OciBlockIOParameters{},
			expectedFsWrites: map[string]string{
				"/my/empty/blkio.bfq.weight": "0",
			},
		},
		{
			name:       "multidevice weight and throttling, no weight write on -1",
			cgroupsDir: "/my/multidev",
			blockIO: OciBlockIOParameters{
				Weight:                  -1,
				WeightDevice:            []OciWeightDeviceParameters{{1, 2, 3}, {4, 5, 6}},
				ThrottleReadBpsDevice:   []OciRateDeviceParameters{{11, 12, 13}, {111, 112, 113}},
				ThrottleWriteBpsDevice:  []OciRateDeviceParameters{{21, 22, 23}, {221, 222, 223}},
				ThrottleReadIOPSDevice:  []OciRateDeviceParameters{{31, 32, 33}, {331, 332, 333}},
				ThrottleWriteIOPSDevice: []OciRateDeviceParameters{{41, 42, 43}, {441, 442, 443}},
			},
			expectedFsWrites: map[string]string{
				"/my/multidev/blkio.bfq.weight_device":          "1:2 3+4:5 6",
				"/my/multidev/blkio.throttle.read_bps_device":   "11:12 13+111:112 113",
				"/my/multidev/blkio.throttle.write_bps_device":  "21:22 23+221:222 223",
				"/my/multidev/blkio.throttle.read_iops_device":  "31:32 33+331:332 333",
				"/my/multidev/blkio.throttle.write_iops_device": "41:42 43+441:442 443",
			},
		},
		{
			name:             "no bfq.weight",
			cgroupsDir:       "/my/nobfq",
			blockIO:          OciBlockIOParameters{Weight: 100},
			writesFail:       1,
			expectedFsWrites: map[string]string{"/my/nobfq/blkio.weight": "100"},
		},
		{
			name:       "all writes fail",
			cgroupsDir: "/my/writesfail",
			blockIO: OciBlockIOParameters{
				Weight:       -1,
				WeightDevice: []OciWeightDeviceParameters{{1, 0, 100}},
			},
			writesFail:         9999,
			expectedErrorCount: 1,
			expectedErrorSubstrings: []string{
				"could not write content \"1:0 100\" to any of files",
				"\"blkio.bfq.weight_device\"",
				"\"blkio.weight_device\"",
			},
		},
	}

	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			mpf := mockPlatform{
				fsWrites:   make(map[string]string),
				writesFail: tc.writesFail,
			}
			currentPlatform = &mpf
			err := SetBlkioParameters(tc.cgroupsDir, tc.blockIO)
			testutils.VerifyError(t, err, tc.expectedErrorCount, tc.expectedErrorSubstrings)
			if tc.expectedFsWrites != nil {
				testutils.VerifyDeepEqual(t, "filesystem writes", tc.expectedFsWrites, mpf.fsWrites)
			}
		})
	}
}

// mockPlatform implements mock versions of platformInterface functions.
type mockPlatform struct {
	fsWrites   map[string]string
	writesFail int
}

func (mpf *mockPlatform) writeToFile(filename string, content string) error {
	var newContent string
	if mpf.writesFail > 0 {
		mpf.writesFail--
		return fmt.Errorf("mockPlatform: writing to %#v failed", filename)
	}
	if oldContent, ok := mpf.fsWrites[filename]; ok {
		newContent = fmt.Sprintf("%s+%s", oldContent, content)
	} else {
		newContent = content
	}
	mpf.fsWrites[filename] = newContent
	return nil
}
