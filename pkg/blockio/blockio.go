/*
Copyright 2020 Intel Corporation

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package blockio

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/hashicorp/go-multierror"

	"github.com/intel/cri-resource-manager/pkg/cgroups"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	logger "github.com/intel/cri-resource-manager/pkg/log"
	"github.com/intel/cri-resource-manager/pkg/utils"
)

const (
	// ConfigModuleName is the configuration section of blockio class definitions
	ConfigModuleName = "blockio"

	// sysfsBlockDeviceIOSchedulerPaths expands (with glob) to block device scheduler files.
	// If modified, check how to parse device node from expanded paths.
	sysfsBlockDeviceIOSchedulerPaths = "/sys/block/*/queue/scheduler"
)

// BlockDeviceInfo holds information on a block device to be configured.
// As users can specify block devices using wildcards ("/dev/disk/by-id/*SSD*")
// BlockDeviceInfo.Origin is maintained for traceability: why this
// block device is included in configuration.
// BlockDeviceInfo.DevNode contains resolved device node, like "/dev/sda".
type BlockDeviceInfo struct {
	Major   int64
	Minor   int64
	DevNode string
	Origin  string
}

// Our logger instance.
var log logger.Logger = logger.NewLogger("blockio")

// staticOciBlockIO connects user-defined block I/O classes to
// corresponding OCI BlockIO parameters. "Static" means that
// new/current block devices matching device wildcards in these
// classes are not expanded every time new containers are assigned to
// these classes. Devices are scanned on only at the beginning and on
// blockio configuration changes.
var staticOciBlockIO = map[string]cgroups.OciBlockIOParameters{}

// currentIOSchedulers contains io-schedulers (found in
// sysfsBlockDeviceIOSchedulerPaths) of device nodes:
// {"/dev/sda": "bfq"}
var currentIOSchedulers map[string]string

// UpdateOciConfig converts the configuration in the opt variable into staticOciBlockIO
func UpdateOciConfig(ignoreErrors bool) error {
	currentIOSchedulers, ioSchedulerDetectionError := getCurrentIOSchedulers()
	if ioSchedulerDetectionError != nil {
		log.Warn("configuration validation partly disabled due to IO scheduler detection error %#v", ioSchedulerDetectionError.Error())
	}

	staticOciBlockIO = map[string]cgroups.OciBlockIOParameters{}
	// Create static OCI BlockIO structures for each blockio class
	for class := range opt.Classes {
		ociBlockIO, err := devicesParametersToOci(opt.Classes[class], currentIOSchedulers)
		if err != nil {
			if ignoreErrors {
				log.Error("ignoring: %v", err)
			} else {
				return err
			}
		}
		// Handle all configurations as static for now. That
		// is, the list of block devices matching Devices
		// wildcards will not be updated without new
		// configNotify(). class.DynamicDevices not supported
		// yet.
		staticOciBlockIO[class] = ociBlockIO
	}
	return nil
}

// SetContainerClass assigns the pod in a container to a blockio class.
func SetContainerClass(c cache.Container, class string) error {
	pod, ok := c.GetPod()
	if !ok {
		return blockioError("failed to get Pod for %s", c.PrettyName())
	}

	ociBlockIO, classIsStatic := staticOciBlockIO[class]
	if !classIsStatic {
		return blockioError("no OCI BlockIO parameters for class %#v", class)
	}

	blkioCgroupPodDir := cgroups.GetBlkioDir() + "/" + pod.GetCgroupParentDir()
	containerCgroupDir := utils.GetContainerCgroupDir(blkioCgroupPodDir, c.GetID())
	if containerCgroupDir == "" {
		return blockioError("failed to find cgroup directory for container %s under %#v, container id %#v", c.PrettyName(), blkioCgroupPodDir, c.GetID())
	}

	err := cgroups.ResetBlkioParameters(containerCgroupDir, ociBlockIO)
	if err != nil {
		return blockioError("assigning container %v to class %#v failed: %w", c.PrettyName(), class, err)
	}

	return nil
}

// getCurrentIOSchedulers returns currently active io-scheduler used for each block device in the system.
func getCurrentIOSchedulers() (map[string]string, error) {
	var ios = map[string]string{}
	schedulerFiles, err := filepath.Glob(sysfsBlockDeviceIOSchedulerPaths)
	if err != nil {
		return ios, blockioError("error in IO scheduler wildcards %#v: %w", sysfsBlockDeviceIOSchedulerPaths, err)
	}
	for _, schedulerFile := range schedulerFiles {
		devName := strings.SplitN(schedulerFile, "/", 5)[3]
		schedulerDataB, err := ioutil.ReadFile(schedulerFile)
		if err != nil {
			// A block device may be disconnected. Continue without error.
			log.Error("failed to read current IO scheduler %#v: %v\n", schedulerFile, err)
			continue
		}
		schedulerData := strings.Trim(string(schedulerDataB), "\n")
		currentScheduler := ""
		if strings.IndexByte(schedulerData, ' ') == -1 {
			currentScheduler = schedulerData
		} else {
			openB := strings.Index(schedulerData, "[")
			closeB := strings.Index(schedulerData, "]")
			if -1 < openB && openB < closeB {
				currentScheduler = schedulerData[openB+1 : closeB]
			}
		}
		if currentScheduler == "" {
			return ios, blockioError("could not parse current scheduler in %#v\n", schedulerFile)
		}

		ios["/dev/"+devName] = currentScheduler
	}
	return ios, nil
}

// deviceParametersToOci converts single blockio class parameters into OCI BlockIO structure.
func devicesParametersToOci(dps []DevicesParameters, currentIOSchedulers map[string]string) (cgroups.OciBlockIOParameters, error) {
	var errors *multierror.Error
	oci := cgroups.NewOciBlockIOParameters()
	for _, dp := range dps {
		var err error
		var weight, throttleReadBps, throttleWriteBps, throttleReadIOPS, throttleWriteIOPS int64
		weight, err = parseAndValidateInt64("Weight", dp.Weight, -1, 10, 1000)
		errors = multierror.Append(errors, err)
		throttleReadBps, err = parseAndValidateInt64("ThrottleReadBps", dp.ThrottleReadBps, -1, 0, -1)
		errors = multierror.Append(errors, err)
		throttleWriteBps, err = parseAndValidateInt64("ThrottleWriteBps", dp.ThrottleWriteBps, -1, 0, -1)
		errors = multierror.Append(errors, err)
		throttleReadIOPS, err = parseAndValidateInt64("ThrottleReadIOPS", dp.ThrottleReadIOPS, -1, 0, -1)
		errors = multierror.Append(errors, err)
		throttleWriteIOPS, err = parseAndValidateInt64("ThrottleWriteIOPS", dp.ThrottleWriteIOPS, -1, 0, -1)
		errors = multierror.Append(errors, err)
		if dp.Devices == nil {
			if weight > -1 {
				oci.Weight = weight
			}
			if throttleReadBps > -1 || throttleWriteBps > -1 || throttleReadIOPS > -1 || throttleWriteIOPS > -1 {
				errors = multierror.Append(errors, fmt.Errorf("ignoring throttling (rbps=%#v wbps=%#v riops=%#v wiops=%#v): Devices not listed",
					dp.ThrottleReadBps, dp.ThrottleWriteBps, dp.ThrottleReadIOPS, dp.ThrottleWriteIOPS))
			}
		} else {
			blockDevices, err := currentPlatform.configurableBlockDevices(dp.Devices)
			if err != nil {
				// Problems in matching block device wildcards and resolving symlinks
				// are worth reporting, but must not block configuring blkio where possible.
				log.Error(err.Error())
			}
			if len(blockDevices) == 0 {
				log.Warn("no matches on any of Devices: %v, parameters ignored", dp.Devices)
			}
			for _, blockDeviceInfo := range blockDevices {
				if weight != -1 {
					if ios, found := currentIOSchedulers[blockDeviceInfo.DevNode]; found {
						if ios != "bfq" && ios != "cfq" {
							log.Warn("weight has no effect on device %#v due to "+
								"incompatible io-scheduler %#v (bfq or cfq required)", blockDeviceInfo.DevNode, ios)
						}
					}
					oci.WeightDevice.Update(blockDeviceInfo.Major, blockDeviceInfo.Minor, weight)
				}
				if throttleReadBps != -1 {
					oci.ThrottleReadBpsDevice.Update(blockDeviceInfo.Major, blockDeviceInfo.Minor, throttleReadBps)
				}
				if throttleWriteBps != -1 {
					oci.ThrottleWriteBpsDevice.Update(blockDeviceInfo.Major, blockDeviceInfo.Minor, throttleWriteBps)
				}
				if throttleReadIOPS != -1 {
					oci.ThrottleReadIOPSDevice.Update(blockDeviceInfo.Major, blockDeviceInfo.Minor, throttleReadIOPS)
				}
				if throttleWriteIOPS != -1 {
					oci.ThrottleWriteIOPSDevice.Update(blockDeviceInfo.Major, blockDeviceInfo.Minor, throttleWriteIOPS)
				}
			}
		}
	}
	return oci, errors.ErrorOrNil()
}

// parseAndValidateInt64 parses quantities, like "64 M", and validates that they are in given range.
func parseAndValidateInt64(fieldName string, fieldContent string,
	defaultValue int64, min int64, max int64) (int64, error) {
	// Returns field content
	if fieldContent == "" {
		return defaultValue, nil
	}
	qty, err := resource.ParseQuantity(fieldContent)
	if err != nil {
		return defaultValue, fmt.Errorf("syntax error in %#v (%#v)", fieldName, fieldContent)
	}
	value := qty.Value()
	if min != -1 && min > value {
		return defaultValue, fmt.Errorf("value of %#v (%#v) smaller than minimum (%#v)", fieldName, value, min)
	}
	if max != -1 && value > max {
		return defaultValue, fmt.Errorf("value of %#v (%#v) bigger than maximum (%#v)", fieldName, value, max)
	}
	return value, nil
}

// platformInterface includes functions that access the system. Enables mocking the system.
type platformInterface interface {
	configurableBlockDevices(devWildcards []string) ([]BlockDeviceInfo, error)
}

// defaultPlatform versions of platformInterface functions access the underlying system.
type defaultPlatform struct{}

// currentPlatform defines which platformInterface is used: defaultPlatform or a mock, for instance.
var currentPlatform platformInterface = defaultPlatform{}

// configurableBlockDevices finds major:minor numbers for device filenames (wildcards allowed)
func (dpm defaultPlatform) configurableBlockDevices(devWildcards []string) ([]BlockDeviceInfo, error) {
	// Return map {devNode: BlockDeviceInfo}
	// Example: {"/dev/sda": {Major:8, Minor:0, Origin:"from symlink /dev/disk/by-id/ata-VendorXSSD from wildcard /dev/disk/by-id/*SSD*"}}
	var errors *multierror.Error
	blockDevices := []BlockDeviceInfo{}
	var origin string

	// 1. Expand wildcards to device filenames (may be symlinks)
	// Example: devMatches["/dev/disk/by-id/ata-VendorSSD"] == "from wildcard \"dev/disk/by-id/*SSD*\""
	devMatches := map[string]string{} // {devNodeOrSymlink: origin}
	for _, devWildcard := range devWildcards {
		devWildcardMatches, err := filepath.Glob(devWildcard)
		if err != nil {
			errors = multierror.Append(errors, fmt.Errorf("bad device wildcard %#v: %w", devWildcard, err))
			continue
		}
		if len(devWildcardMatches) == 0 {
			errors = multierror.Append(errors, fmt.Errorf("device wildcard %#v does not match any device nodes", devWildcard))
			continue
		}
		for _, devMatch := range devWildcardMatches {
			if devMatch != devWildcard {
				origin = fmt.Sprintf("from wildcard %#v", devWildcard)
			} else {
				origin = ""
			}
			devMatches[devMatch] = strings.TrimSpace(fmt.Sprintf("%v %v", devMatches[devMatch], origin))
		}
	}

	// 2. Find out real device nodes behind symlinks
	// Example: devRealPaths["/dev/sda"] == "from symlink \"/dev/disk/by-id/ata-VendorSSD\""
	devRealpaths := map[string]string{} // {devNode: origin}
	for devMatch, devOrigin := range devMatches {
		realDevNode, err := filepath.EvalSymlinks(devMatch)
		if err != nil {
			errors = multierror.Append(errors, fmt.Errorf("cannot filepath.EvalSymlinks(%#v): %w", devMatch, err))
			continue
		}
		if realDevNode != devMatch {
			origin = fmt.Sprintf("from symlink %#v %v", devMatch, devOrigin)
		} else {
			origin = devOrigin
		}
		devRealpaths[realDevNode] = strings.TrimSpace(fmt.Sprintf("%v %v", devRealpaths[realDevNode], origin))
	}

	// 3. Filter out everything but block devices that are not partitions
	// Example: blockDevices[0] == {Major: 8, Minor: 0, DevNode: "/dev/sda", Origin: "..."}
	for devRealpath, devOrigin := range devRealpaths {
		origin := ""
		if devOrigin != "" {
			origin = fmt.Sprintf(" (origin: %s)", devOrigin)
		}
		fileInfo, err := os.Stat(devRealpath)
		if err != nil {
			errors = multierror.Append(errors, fmt.Errorf("cannot os.Stat(%#v): %w%s", devRealpath, err, origin))
			continue
		}
		fileMode := fileInfo.Mode()
		if fileMode&os.ModeDevice == 0 {
			errors = multierror.Append(errors, fmt.Errorf("file %#v is not a device%s", devRealpath, origin))
			continue
		}
		if fileMode&os.ModeCharDevice != 0 {
			errors = multierror.Append(errors, fmt.Errorf("file %#v is a character device%s", devRealpath, origin))
			continue
		}
		sys, ok := fileInfo.Sys().(*syscall.Stat_t)
		major := unix.Major(sys.Rdev)
		minor := unix.Minor(sys.Rdev)
		if !ok {
			errors = multierror.Append(errors, fmt.Errorf("cannot get syscall stat_t from %#v: %w%s", devRealpath, err, origin))
			continue
		}
		if minor&0xf != 0 {
			errors = multierror.Append(errors, fmt.Errorf("skipping %#v: cannot weight/throttle partitions%s", devRealpath, origin))
			continue
		}
		blockDevices = append(blockDevices, BlockDeviceInfo{
			Major:   int64(major),
			Minor:   int64(minor),
			DevNode: devRealpath,
			Origin:  devOrigin,
		})
	}
	return blockDevices, errors.ErrorOrNil()
}

// blockioError creates a formatted error message.
func blockioError(format string, args ...interface{}) error {
	return fmt.Errorf(format, args...)
}
