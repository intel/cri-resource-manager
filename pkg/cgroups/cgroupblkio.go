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
	"os"
	"path/filepath"
	"strconv"

	"github.com/hashicorp/go-multierror"

	logger "github.com/intel/cri-resource-manager/pkg/log"
)

const (
	blkioCgroupDir = "/sys/fs/cgroup/blkio/"
)

// logger
var log logger.Logger = logger.NewLogger("cgroupblkio")

// cgroups blkio parameter filenames.
var blkioWeightFiles = []string{"blkio.bfq.weight", "blkio.weight"}
var blkioWeightDeviceFiles = []string{"blkio.bfq.weight_device", "blkio.weight_device"}
var blkioThrottleReadBpsFiles = []string{"blkio.throttle.read_bps_device"}
var blkioThrottleWriteBpsFiles = []string{"blkio.throttle.write_bps_device"}
var blkioThrottleReadIOPSFiles = []string{"blkio.throttle.read_iops_device"}
var blkioThrottleWriteIOPSFiles = []string{"blkio.throttle.write_iops_device"}

// OciBlockIOParameters contains OCI standard configuration of cgroups blkio parameters.
//
// Effects of Weight and Rate values in SetBlkioParameters():
// Value  |  Effect
// -------+-------------------------------------------------------------------
//    -1  |  Do not write to cgroups, value is missing
//     0  |  Write to cgroups, will remove the setting as specified in cgroups blkio interface
//  other |  Write to cgroups, sets the value
type OciBlockIOParameters struct {
	Weight                  int64
	WeightDevice            []OciWeightDeviceParameters
	ThrottleReadBpsDevice   []OciRateDeviceParameters
	ThrottleWriteBpsDevice  []OciRateDeviceParameters
	ThrottleReadIOPSDevice  []OciRateDeviceParameters
	ThrottleWriteIOPSDevice []OciRateDeviceParameters
}

// OciWeightDeviceParameters contains values for
// - blkio.[io-scheduler].weight
type OciWeightDeviceParameters struct {
	Major  int64
	Minor  int64
	Weight int64
}

// OciRateDeviceParameters contains values for
// - blkio.throttle.read_bps_device
// - blkio.throttle.write_bps_device
// - blkio.throttle.read_iops_device
// - blkio.throttle.write_iops_device
type OciRateDeviceParameters struct {
	Major int64
	Minor int64
	Rate  int64
}

// NewOciBlockIOParameters creates new OciBlockIOParameters instance.
func NewOciBlockIOParameters() OciBlockIOParameters {
	return OciBlockIOParameters{
		Weight: -1,
	}
}

// NewOciWeightDeviceParametes creates new OciWeightDeviceParameters instance.
func NewOciWeightDeviceParametes() OciWeightDeviceParameters {
	return OciWeightDeviceParameters{
		Major:  -1,
		Minor:  -1,
		Weight: -1,
	}
}

// NewOciRateDeviceParameters creates new OciRateDeviceParameters instance.
func NewOciRateDeviceParameters() OciRateDeviceParameters {
	return OciRateDeviceParameters{
		Major: -1,
		Minor: -1,
		Rate:  -1,
	}
}

// GetBlkioDir returns the cgroups blkio controller directory.
func GetBlkioDir() string {
	return blkioCgroupDir
}

// SetBlkioParameters writes OCI BlockIO parameters to files in cgroups blkio contoller directory.
func SetBlkioParameters(cgroupsDir string, blockIO OciBlockIOParameters) error {
	log.Debug("configuring cgroups blkio controller in directory %#v with parameters %+v", cgroupsDir, blockIO)
	var errors *multierror.Error
	if blockIO.Weight >= 0 {
		errors = multierror.Append(errors, writeToFileInDir(cgroupsDir, blkioWeightFiles, strconv.FormatInt(blockIO.Weight, 10)))
	}
	for _, weightDevice := range blockIO.WeightDevice {
		errors = multierror.Append(errors, writeDevValueToFileInDir(cgroupsDir, blkioWeightDeviceFiles, weightDevice.Major, weightDevice.Minor, weightDevice.Weight))
	}
	for _, rateDevice := range blockIO.ThrottleReadBpsDevice {
		errors = multierror.Append(errors, writeDevValueToFileInDir(cgroupsDir, blkioThrottleReadBpsFiles, rateDevice.Major, rateDevice.Minor, rateDevice.Rate))
	}
	for _, rateDevice := range blockIO.ThrottleWriteBpsDevice {
		errors = multierror.Append(errors, writeDevValueToFileInDir(cgroupsDir, blkioThrottleWriteBpsFiles, rateDevice.Major, rateDevice.Minor, rateDevice.Rate))
	}
	for _, rateDevice := range blockIO.ThrottleReadIOPSDevice {
		errors = multierror.Append(errors, writeDevValueToFileInDir(cgroupsDir, blkioThrottleReadIOPSFiles, rateDevice.Major, rateDevice.Minor, rateDevice.Rate))
	}
	for _, rateDevice := range blockIO.ThrottleWriteIOPSDevice {
		errors = multierror.Append(errors, writeDevValueToFileInDir(cgroupsDir, blkioThrottleWriteIOPSFiles, rateDevice.Major, rateDevice.Minor, rateDevice.Rate))
	}
	return errors.ErrorOrNil()
}

// writeDevValueToFileInDir writes MAJOR:MINOR VALUE to the first existing file under baseDir
func writeDevValueToFileInDir(baseDir string, filenames []string, major, minor, value int64) error {
	content := fmt.Sprintf("%d:%d %d", major, minor, value)
	return writeToFileInDir(baseDir, filenames, content)
}

// writeToFileInDir writes content to the first existing file in the list under baseDir.
func writeToFileInDir(baseDir string, filenames []string, content string) error {
	var errors *multierror.Error
	// Returns list of errors from writes, list of single error due to all filenames missing or nil on success.
	for _, filename := range filenames {
		filepath := filepath.Join(baseDir, filename)
		err := currentPlatform.writeToFile(filepath, content)
		if err == nil {
			return nil
		}
		errors = multierror.Append(errors, err)
	}
	err := errors.ErrorOrNil()
	if err != nil {
		return fmt.Errorf("could not write content %#v to any of files %q: %w", content, filenames, err)
	}
	return nil
}

// platformInterface includes functions that access the system. Enables mocking the platform.
type platformInterface interface {
	writeToFile(filename string, content string) error
}

// defaultPlatform versions of platformInterface functions access the underlying system.
type defaultPlatform struct{}

// currentPlatform defines which platformInterface is used: defaultPlatform or a mock, for instance.
var currentPlatform platformInterface = defaultPlatform{}

// writeToFile writes content to an existing file.
func (dpm defaultPlatform) writeToFile(filename string, content string) error {
	f, err := os.OpenFile(filename, os.O_WRONLY, 0666)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, content)
	return err
}
