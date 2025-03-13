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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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
//
//	  -1  |  Do not write to cgroups, value is missing
//	   0  |  Write to cgroups, will remove the setting as specified in cgroups blkio interface
//	other |  Write to cgroups, sets the value
type OciBlockIOParameters struct {
	Weight                  int64
	WeightDevice            OciDeviceWeights
	ThrottleReadBpsDevice   OciDeviceRates
	ThrottleWriteBpsDevice  OciDeviceRates
	ThrottleReadIOPSDevice  OciDeviceRates
	ThrottleWriteIOPSDevice OciDeviceRates
}

// OciDeviceWeight contains values for
// - blkio.[io-scheduler].weight
type OciDeviceWeight struct {
	Major  int64
	Minor  int64
	Weight int64
}

// OciDeviceRate contains values for
// - blkio.throttle.read_bps_device
// - blkio.throttle.write_bps_device
// - blkio.throttle.read_iops_device
// - blkio.throttle.write_iops_device
type OciDeviceRate struct {
	Major int64
	Minor int64
	Rate  int64
}

// OciDeviceWeights contains weights for devices
type OciDeviceWeights []OciDeviceWeight

// OciDeviceRates contains throttling rates for devices
type OciDeviceRates []OciDeviceRate

// OciDeviceParameters interface provides functions common to OciDeviceWeights and OciDeviceRates
type OciDeviceParameters interface {
	Append(maj, min, val int64)
	Update(maj, min, val int64)
}

// Append appends (major, minor, value) to OciDeviceWeights slice.
func (w *OciDeviceWeights) Append(maj, min, val int64) {
	*w = append(*w, OciDeviceWeight{Major: maj, Minor: min, Weight: val})
}

// Append appends (major, minor, value) to OciDeviceRates slice.
func (r *OciDeviceRates) Append(maj, min, val int64) {
	*r = append(*r, OciDeviceRate{Major: maj, Minor: min, Rate: val})
}

// Update updates device weight in OciDeviceWeights slice, or appends it if not found.
func (w *OciDeviceWeights) Update(maj, min, val int64) {
	for index, devWeight := range *w {
		if devWeight.Major == maj && devWeight.Minor == min {
			(*w)[index].Weight = val
			return
		}
	}
	w.Append(maj, min, val)
}

// Update updates device rate in OciDeviceRates slice, or appends it if not found.
func (r *OciDeviceRates) Update(maj, min, val int64) {
	for index, devRate := range *r {
		if devRate.Major == maj && devRate.Minor == min {
			(*r)[index].Rate = val
			return
		}
	}
	r.Append(maj, min, val)
}

// NewOciBlockIOParameters creates new OciBlockIOParameters instance.
func NewOciBlockIOParameters() OciBlockIOParameters {
	return OciBlockIOParameters{
		Weight: -1,
	}
}

// NewOciDeviceWeight creates new OciDeviceWeight instance.
func NewOciDeviceWeight() OciDeviceWeight {
	return OciDeviceWeight{
		Major:  -1,
		Minor:  -1,
		Weight: -1,
	}
}

// NewOciDeviceRate creates new OciDeviceRate instance.
func NewOciDeviceRate() OciDeviceRate {
	return OciDeviceRate{
		Major: -1,
		Minor: -1,
		Rate:  -1,
	}
}

// GetBlkioDir returns the cgroups blkio controller directory.
func GetBlkioDir() string {
	return blkioCgroupDir
}

type devMajMin struct {
	Major int64
	Minor int64
}

// ResetBlkioParameters adds new, changes existing and removes missing blockIO parameters in cgroupsDir
func ResetBlkioParameters(cgroupsDir string, blockIO OciBlockIOParameters) error {
	errs := []error{}
	oldBlockIO, getErr := GetBlkioParameters(cgroupsDir)
	errs = append(errs, getErr)
	newBlockIO := NewOciBlockIOParameters()
	newBlockIO.Weight = blockIO.Weight
	// Set new device weights
	seenDev := map[devMajMin]bool{}
	for _, ociWDP := range blockIO.WeightDevice {
		seenDev[devMajMin{ociWDP.Major, ociWDP.Minor}] = true
		newBlockIO.WeightDevice = append(newBlockIO.WeightDevice, ociWDP)
	}
	// Reset old device weights that were missing from blockIO.WeightDevice
	for _, ociWDP := range oldBlockIO.WeightDevice {
		if !seenDev[devMajMin{ociWDP.Major, ociWDP.Minor}] {
			newBlockIO.WeightDevice = append(newBlockIO.WeightDevice, OciDeviceWeight{ociWDP.Major, ociWDP.Minor, 0})
		}
	}
	newBlockIO.ThrottleReadBpsDevice = resetDevRates(oldBlockIO.ThrottleReadBpsDevice, blockIO.ThrottleReadBpsDevice)
	newBlockIO.ThrottleWriteBpsDevice = resetDevRates(oldBlockIO.ThrottleWriteBpsDevice, blockIO.ThrottleWriteBpsDevice)
	newBlockIO.ThrottleReadIOPSDevice = resetDevRates(oldBlockIO.ThrottleReadIOPSDevice, blockIO.ThrottleReadIOPSDevice)
	newBlockIO.ThrottleWriteIOPSDevice = resetDevRates(oldBlockIO.ThrottleWriteIOPSDevice, blockIO.ThrottleWriteIOPSDevice)
	errs = append(errs, SetBlkioParameters(cgroupsDir, newBlockIO))
	return errors.Join(errs...)
}

// resetDevRates adds wanted rate parameters to new and resets unwated rates
func resetDevRates(old, wanted []OciDeviceRate) []OciDeviceRate {
	rates := []OciDeviceRate{}
	seenDev := map[devMajMin]bool{}
	for _, rdp := range wanted {
		rates = append(rates, rdp)
		seenDev[devMajMin{rdp.Major, rdp.Minor}] = true
	}
	for _, rdp := range old {
		if !seenDev[devMajMin{rdp.Major, rdp.Minor}] {
			rates = append(rates, OciDeviceRate{rdp.Major, rdp.Minor, 0})
		}
	}
	return rates
}

// GetBlkioParameters returns OCI BlockIO parameters from files in cgroups blkio controller directory.
func GetBlkioParameters(cgroupsDir string) (OciBlockIOParameters, error) {
	errs := []error{}
	blockIO := NewOciBlockIOParameters()
	content, err := readFromFileInDir(cgroupsDir, blkioWeightFiles)
	if err == nil {
		weight, err := strconv.ParseInt(strings.TrimSuffix(content, "\n"), 10, 64)
		if err == nil {
			blockIO.Weight = weight
		} else {
			errs = append(errs, fmt.Errorf("parsing weight from %#v failed: %w", content, err))
		}
	} else {
		errs = append(errs, err)
	}
	errs = append(errs, readOciDeviceParameters(cgroupsDir, blkioWeightDeviceFiles, &blockIO.WeightDevice))
	errs = append(errs, readOciDeviceParameters(cgroupsDir, blkioThrottleReadBpsFiles, &blockIO.ThrottleReadBpsDevice))
	errs = append(errs, readOciDeviceParameters(cgroupsDir, blkioThrottleWriteBpsFiles, &blockIO.ThrottleWriteBpsDevice))
	errs = append(errs, readOciDeviceParameters(cgroupsDir, blkioThrottleReadIOPSFiles, &blockIO.ThrottleReadIOPSDevice))
	errs = append(errs, readOciDeviceParameters(cgroupsDir, blkioThrottleWriteIOPSFiles, &blockIO.ThrottleWriteIOPSDevice))
	return blockIO, errors.Join(errs...)
}

// readOciDeviceParameters parses device lines used for weights and throttling rates
func readOciDeviceParameters(baseDir string, filenames []string, params OciDeviceParameters) error {
	errs := []error{}
	contents, err := readFromFileInDir(baseDir, filenames)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(contents, "\n") {
		// Device weight files may have "default NNN" line at the beginning. Skip it.
		if line == "" || strings.HasPrefix(line, "default ") {
			continue
		}
		// Expect syntax MAJOR:MINOR VALUE
		devVal := strings.Split(line, " ")
		if len(devVal) != 2 {
			errs = append(errs, fmt.Errorf("invalid line %q, single space expected", line))
			continue
		}
		majMin := strings.Split(devVal[0], ":")
		if len(majMin) != 2 {
			errs = append(errs, fmt.Errorf("invalid line %q, single colon expected before space", line))
			continue
		}
		major, majErr := strconv.ParseInt(majMin[0], 10, 64)
		minor, minErr := strconv.ParseInt(majMin[1], 10, 64)
		value, valErr := strconv.ParseInt(devVal[1], 10, 64)
		if majErr != nil || minErr != nil || valErr != nil {
			errs = append(errs, fmt.Errorf("invalid number when parsing \"major:minor value\" from \"%s:%s %s\"", majMin[0], majMin[1], devVal[1]))
			continue
		}
		params.Append(major, minor, value)
	}
	return errors.Join(errs...)
}

// readFromFileInDir returns content from the first successfully read file.
func readFromFileInDir(baseDir string, filenames []string) (string, error) {
	errs := []error{}
	// If reading all the files fails, return list of read errors.
	for _, filename := range filenames {
		filepath := filepath.Join(baseDir, filename)
		content, err := currentPlatform.readFromFile(filepath)
		if err == nil {
			return content, nil
		}
		errs = append(errs, err)
	}
	err := errors.Join(errs...)
	if err != nil {
		return "", fmt.Errorf("could not read any of files %q: %w", filenames, err)
	}
	return "", nil
}

// SetBlkioParameters writes OCI BlockIO parameters to files in cgroups blkio contoller directory.
func SetBlkioParameters(cgroupsDir string, blockIO OciBlockIOParameters) error {
	log.Debug("configuring cgroups blkio controller in directory %#v with parameters %+v", cgroupsDir, blockIO)
	errs := []error{}
	if blockIO.Weight >= 0 {
		errs = append(errs, writeToFileInDir(cgroupsDir, blkioWeightFiles, strconv.FormatInt(blockIO.Weight, 10)))
	}
	for _, weightDevice := range blockIO.WeightDevice {
		errs = append(errs, writeDevValueToFileInDir(cgroupsDir, blkioWeightDeviceFiles, weightDevice.Major, weightDevice.Minor, weightDevice.Weight))
	}
	for _, rateDevice := range blockIO.ThrottleReadBpsDevice {
		errs = append(errs, writeDevValueToFileInDir(cgroupsDir, blkioThrottleReadBpsFiles, rateDevice.Major, rateDevice.Minor, rateDevice.Rate))
	}
	for _, rateDevice := range blockIO.ThrottleWriteBpsDevice {
		errs = append(errs, writeDevValueToFileInDir(cgroupsDir, blkioThrottleWriteBpsFiles, rateDevice.Major, rateDevice.Minor, rateDevice.Rate))
	}
	for _, rateDevice := range blockIO.ThrottleReadIOPSDevice {
		errs = append(errs, writeDevValueToFileInDir(cgroupsDir, blkioThrottleReadIOPSFiles, rateDevice.Major, rateDevice.Minor, rateDevice.Rate))
	}
	for _, rateDevice := range blockIO.ThrottleWriteIOPSDevice {
		errs = append(errs, writeDevValueToFileInDir(cgroupsDir, blkioThrottleWriteIOPSFiles, rateDevice.Major, rateDevice.Minor, rateDevice.Rate))
	}
	return errors.Join(errs...)
}

// writeDevValueToFileInDir writes MAJOR:MINOR VALUE to the first existing file under baseDir
func writeDevValueToFileInDir(baseDir string, filenames []string, major, minor, value int64) error {
	content := fmt.Sprintf("%d:%d %d", major, minor, value)
	return writeToFileInDir(baseDir, filenames, content)
}

// writeToFileInDir writes content to the first existing file in the list under baseDir.
func writeToFileInDir(baseDir string, filenames []string, content string) error {
	errs := []error{}
	// Returns list of errors from writes, list of single error due to all filenames missing or nil on success.
	for _, filename := range filenames {
		filepath := filepath.Join(baseDir, filename)
		err := currentPlatform.writeToFile(filepath, content)
		if err == nil {
			return nil
		}
		errs = append(errs, err)
	}
	err := errors.Join(errs...)
	if err != nil {
		return fmt.Errorf("could not write content %#v to any of files %q: %w", content, filenames, err)
	}
	return nil
}

// platformInterface includes functions that access the system. Enables mocking the platform.
type platformInterface interface {
	readFromFile(filename string) (string, error)
	writeToFile(filename string, content string) error
}

// defaultPlatform versions of platformInterface functions access the underlying system.
type defaultPlatform struct{}

// currentPlatform defines which platformInterface is used: defaultPlatform or a mock, for instance.
var currentPlatform platformInterface = defaultPlatform{}

// readFromFile returns file contents as a string.
func (dpm defaultPlatform) readFromFile(filename string) (string, error) {
	content, err := os.ReadFile(filename)
	return string(content), err
}

// writeToFile writes content to an existing file.
func (dpm defaultPlatform) writeToFile(filename string, content string) error {
	f, err := os.OpenFile(filename, os.O_WRONLY, 0666)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write([]byte(content))
	return err
}
