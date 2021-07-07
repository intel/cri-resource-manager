// Copyright 2021 Intel Corporation. All Rights Reserved.
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
	"os"

	logger "github.com/intel/cri-resource-manager/pkg/log"
)

// SstPackageInfo contains status of Intel Speed Select Technologies (SST)
// for one CPU package
type SstPackageInfo struct {
	// Gereric PP info
	PPSupported    bool
	PPLocked       bool
	PPVersion      int
	PPCurrentLevel int
	PPMaxLevel     int

	// Information about the currently active PP level
	CPSupported bool
	CPEnabled   bool
	CPPriority  CPPriorityType
	BFSupported bool
	BFEnabled   bool
	BFCores     IDSet
	TFSupported bool
	TFEnabled   bool

	ClosInfo [NumClos]SstClosInfo
}

// NumClos is the number of CLOSes suported by SST-CP
const NumClos = 4

// SstClosInfo contains parameters of one CLOS of SST-CP
type SstClosInfo struct {
	EPP                  int
	ProportionalPriority int
	MinFreq              int
	MaxFreq              int
	DesiredFreq          int
}

// CPPriorityType denotes the type CLOS priority ordering used in SST-CP
type CPPriorityType int

const (
	Proportional CPPriorityType = 0
	Ordered      CPPriorityType = 1
)

const isstDevPath = "/dev/isst_interface"

var sstlog = logger.NewLogger("sst")

// SstSupported returns true if Intel Speed Select Technologies (SST) is supported
// by the system and can be interfaced via the Linux kernel device
func SstSupported() bool {
	if _, err := os.Stat(isstDevPath); err != nil {
		if !os.IsNotExist(err) {
			sstlog.Warnf("failed to access sst device %q: %v", isstDevPath, err)
		} else {
			sstlog.Debugf("sst device %q does not exist", isstDevPath)
		}
		return false
	}
	return true
}

func getSstPackageInfo(pkg *cpuPackage) (SstPackageInfo, error) {
	info := SstPackageInfo{}
	ids := pkg.cpus.SortedMembers()
	cpu := ids[0] // We just need to pass one logical cpu from the pkg as an arg

	var rsp uint32
	var err error

	// Read perf-profile feature info
	if rsp, err = sendMboxCmd(cpu, CONFIG_TDP, CONFIG_TDP_GET_LEVELS_INFO, 0); err != nil {
		return info, fmt.Errorf("failed to read SST PP info: %v", err)
	}
	info.PPSupported = getBits(rsp, 31, 31) != 0
	info.PPLocked = getBits(rsp, 24, 24) != 0
	info.PPCurrentLevel = int(getBits(rsp, 16, 23))
	info.PPMaxLevel = int(getBits(rsp, 8, 15))
	info.PPVersion = int(getBits(rsp, 0, 7))

	// Forget about older hw with partial/convoluted support
	if info.PPVersion < 3 {
		sstlog.Infof("SST PP version %d (less than 3), giving up...")
		return info, nil
	}

	// Read the status of currently active perf-profile
	if info.PPSupported {
		if rsp, err = sendMboxCmd(cpu, CONFIG_TDP, CONFIG_TDP_GET_TDP_CONTROL, uint32(info.PPCurrentLevel)); err != nil {
			return info, fmt.Errorf("failed to read SST CP status: %v", err)
		}

		info.BFSupported = isBitSet(rsp, 1)
		info.BFEnabled = isBitSet(rsp, 17)

		info.TFSupported = isBitSet(rsp, 0)
		info.TFEnabled = isBitSet(rsp, 16)

	}

	// Read base-frequency info
	if info.BFSupported {
		info.BFCores = IDSet{}

		punitCoreIDs := make(map[ID]IDSet, len(ids))
		var maxPunitCore ID
		for _, id := range ids {
			pc, err := punitCPU(id)
			if err != nil {
				return info, err
			}
			punitCore := pc >> 1
			if _, ok := punitCoreIDs[punitCore]; !ok {
				punitCoreIDs[punitCore] = IDSet{}
			}
			punitCoreIDs[punitCore].Add(id)
			if punitCore > maxPunitCore {
				maxPunitCore = punitCore
			}
		}

		// Read out core masks in batches of 32 (32 bits per response)
		for i := 0; i <= int(maxPunitCore)/32; i++ {
			if rsp, err = sendMboxCmd(cpu, CONFIG_TDP, CONFIG_TDP_PBF_GET_CORE_MASK_INFO, uint32(info.PPCurrentLevel+(i<<8))); err != nil {
				return info, fmt.Errorf("failed to read SST BF core mask (#%d): %v", i, err)
			}
			for bit := 0; bit < 32; bit++ {
				if isBitSet(rsp, uint32(bit)) {
					info.BFCores.Add(punitCoreIDs[ID(i*32+bit)].Members()...)
				}
			}
		}
	}

	// Read core-power feature info
	if rsp, err = sendMboxCmd(cpu, READ_PM_CONFIG, PM_FEATURE, 0); err != nil {
		return info, fmt.Errorf("failed to read SST CP info: %v", err)
	}

	info.CPSupported = isBitSet(rsp, 0)
	info.CPEnabled = isBitSet(rsp, 16)

	if info.CPSupported {
		if rsp, err = sendMboxCmd(cpu, CONFIG_CLOS, CLOS_PM_QOS_CONFIG, 0); err != nil {
			return info, fmt.Errorf("failed to read SST CP status: %v", err)
		}

		closEnabled := isBitSet(rsp, 1)
		if closEnabled != info.CPEnabled {
			sstlog.Warnf("SST firmware returned conflicting CP enabled status %v vs. %v", info.CPEnabled, closEnabled)
		}
		info.CPEnabled = closEnabled
		info.CPPriority = CPPriorityType(getBits(rsp, 2, 2))

		for i := uint32(0); i < NumClos; i++ {
			if rsp, err = sendMMIOCmd(cpu, (i<<2)+PM_CLOS_OFFSET); err != nil {
				return info, fmt.Errorf("failed to read SST CLOS #%d info: %v", i, err)
			}
			info.ClosInfo[i] = SstClosInfo{
				EPP:                  int(getBits(rsp, 0, 3)),
				ProportionalPriority: int(getBits(rsp, 4, 7)),
				MinFreq:              int(getBits(rsp, 8, 15)),
				MaxFreq:              int(getBits(rsp, 16, 23)),
				DesiredFreq:          int(getBits(rsp, 24, 31)),
			}
		}
	}

	return info, nil
}

func getCPUClosID(cpu ID) (int, error) {
	p, err := punitCPU(cpu)
	if err != nil {
		return -1, err
	}
	punitCore := uint32(p) >> 1

	rsp, err := sendMMIOCmd(cpu, (punitCore<<2)+PQR_ASSOC_OFFSET)
	if err != nil {
		return 0, fmt.Errorf("failed to read CLOS number of cpu %d: %v", cpu, err)
	}
	return int(getBits(rsp, 16, 17)), nil
}

func getCPUClosIDs(cpus []ID) ([]int, error) {
	ret := make([]int, len(cpus))
	for i, cpu := range cpus {
		id, err := getCPUClosID(cpu)
		if err != nil {
			return ret, err
		}
		ret[i] = id
	}
	return ret, nil
}

func getBits(val, i, j uint32) uint32 {
	lsb := i
	msb := j
	if i > j {
		lsb = j
		msb = i
	}
	return (val >> lsb) & ((1 << (msb - lsb + 1)) - 1)
}

func isBitSet(val, n uint32) bool {
	return val&(1<<n) != 0
}
