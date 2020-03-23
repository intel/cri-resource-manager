/*
Copyright 2019 Intel Corporation

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

package rdt

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Info contains information about the RDT support in the system
type info struct {
	resctrlPath      string
	resctrlMountOpts map[string]struct{}
	numClosids       uint64
	cacheIds         []uint64
	l3               l3Info
	l3code           l3Info
	l3data           l3Info
	mb               mbInfo
}

type l3Info struct {
	cbmMask       Bitmask
	minCbmBits    uint64
	shareableBits Bitmask
}

type mbInfo struct {
	bandwidthGran uint64
	delayLinear   uint64
	minBandwidth  uint64
	mbpsEnabled   bool // true if MBA_MBps is enabled
}

// l3Info is a helper method for a "unified API" for getting L3 information
func (i info) l3Info() l3Info {
	switch {
	case i.l3code.Supported():
		return i.l3code
	case i.l3data.Supported():
		return i.l3data
	}
	return i.l3
}

func (i info) l3CbmMask() Bitmask {
	mask := i.l3Info().cbmMask
	if mask != 0 {
		return mask
	}
	return Bitmask(^uint64(0))
}

func (i info) l3MinCbmBits() uint64 {
	return i.l3Info().minCbmBits
}

func getRdtInfo() (info, error) {
	var err error
	info := info{}

	info.resctrlPath, info.resctrlMountOpts, err = getResctrlMountInfo()
	if err != nil {
		return info, rdtError("failed to detect resctrl mount point: %v", err)
	}
	log.Info("detected resctrl filesystem at %q", info.resctrlPath)

	// Check that RDT is available
	infopath := filepath.Join(info.resctrlPath, "info")
	if _, err := os.Stat(infopath); err != nil {
		return info, rdtError("failed to read RDT info from %q: %v", infopath, err)
	}

	l3path := filepath.Join(infopath, "L3")
	if _, err = os.Stat(l3path); err == nil {
		info.l3, info.numClosids, err = getL3Info(l3path)
		if err != nil {
			return info, rdtError("failed to get L3 info from %q: %v", l3path, err)
		}
	}

	l3path = filepath.Join(infopath, "L3CODE")
	if _, err = os.Stat(l3path); err == nil {
		info.l3code, info.numClosids, err = getL3Info(l3path)
		if err != nil {
			return info, rdtError("failed to get L3CODE info from %q: %v", l3path, err)
		}
	}

	l3path = filepath.Join(infopath, "L3DATA")
	if _, err = os.Stat(l3path); err == nil {
		info.l3data, info.numClosids, err = getL3Info(l3path)
		if err != nil {
			return info, rdtError("failed to get L3DATA info from %q: %v", l3path, err)
		}
	}

	mbpath := filepath.Join(infopath, "MB")
	if _, err = os.Stat(mbpath); err == nil {
		info.mb, info.numClosids, err = getMBInfo(mbpath)
		if err != nil {
			return info, rdtError("failed to get MBA info from %q: %v", mbpath, err)
		}
	}

	info.cacheIds, err = getCacheIds(info.resctrlPath)
	if err != nil {
		return info, rdtError("failed to get cache IDs: %v", err)
	}

	return info, nil
}

func getL3Info(basepath string) (l3Info, uint64, error) {
	var err error
	var numClosids uint64
	info := l3Info{}

	info.cbmMask, err = readFileBitmask(filepath.Join(basepath, "cbm_mask"))
	if err != nil {
		return info, numClosids, err
	}
	info.minCbmBits, err = readFileUint64(filepath.Join(basepath, "min_cbm_bits"))
	if err != nil {
		return info, numClosids, err
	}
	info.shareableBits, err = readFileBitmask(filepath.Join(basepath, "shareable_bits"))
	if err != nil {
		return info, numClosids, err
	}
	numClosids, err = readFileUint64(filepath.Join(basepath, "num_closids"))
	if err != nil {
		return info, numClosids, err
	}

	return info, numClosids, nil
}

// Supported returns true if L3 cache allocation has is supported and enabled in the system
func (i l3Info) Supported() bool {
	return i.cbmMask != 0
}

func getMBInfo(basepath string) (mbInfo, uint64, error) {
	var err error
	var numClosids uint64
	info := mbInfo{}

	info.bandwidthGran, err = readFileUint64(filepath.Join(basepath, "bandwidth_gran"))
	if err != nil {
		return info, numClosids, err
	}
	info.delayLinear, err = readFileUint64(filepath.Join(basepath, "delay_linear"))
	if err != nil {
		return info, numClosids, err
	}
	info.minBandwidth, err = readFileUint64(filepath.Join(basepath, "min_bandwidth"))
	if err != nil {
		return info, numClosids, err
	}
	numClosids, err = readFileUint64(filepath.Join(basepath, "num_closids"))
	if err != nil {
		return info, numClosids, err
	}

	// Detect MBps mode directly from mount options as it's not visible in MB
	// info directory
	_, mountOpts, err := getResctrlMountInfo()
	if err != nil {
		return info, numClosids, fmt.Errorf("failed to get resctrl mount options: %v", err)
	}
	if _, ok := mountOpts["mba_MBps"]; ok {
		info.mbpsEnabled = true
	}

	return info, numClosids, nil
}

// Supported returns true if memory bandwidth allocation has is supported and enabled in the system
func (i mbInfo) Supported() bool {
	return i.minBandwidth != 0
}

func getCacheIds(basepath string) ([]uint64, error) {
	var ids []uint64

	// Parse cache IDs from the root schemata
	data, err := readFileString(filepath.Join(basepath, "schemata"))
	if err != nil {
		return ids, rdtError("failed to read root schemata: %v", err)
	}

	for _, line := range strings.Split(data, "\n") {
		trimmed := strings.TrimSpace(line)

		// Find line with L3 or MB schema
		if strings.HasPrefix(trimmed, "L3") || strings.HasPrefix(trimmed, "MB:") {
			schema := strings.Split(trimmed[3:], ";")
			ids = make([]uint64, len(schema))

			// Get individual cache configurations from the schema
			for idx, definition := range schema {
				split := strings.Split(definition, "=")
				if len(split) != 2 {
					return ids, rdtError("looks like an invalid L3 %q", trimmed)
				}
				ids[idx], err = strconv.ParseUint(split[0], 10, 64)
				if err != nil {
					if len(split) != 2 {
						return ids, rdtError("failed to parse cache id in %q: %v", trimmed, err)
					}
				}
			}
			return ids, nil
		}
	}
	return ids, rdtError("no resources in root schemata")
}

func getResctrlMountInfo() (string, map[string]struct{}, error) {
	mountOptions := map[string]struct{}{}

	f, err := os.Open("/proc/mounts")
	if err != nil {
		return "", mountOptions, err
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	for s.Scan() {
		split := strings.Split(s.Text(), " ")
		if len(split) > 3 && split[2] == "resctrl" {
			opts := strings.Split(split[3], ",")
			for _, opt := range opts {
				mountOptions[opt] = struct{}{}
			}
			return split[1], mountOptions, nil
		}
	}
	return "", mountOptions, rdtError("resctrl not found in /proc/mounts")
}

func readFileUint64(path string) (uint64, error) {
	data, err := readFileString(path)
	if err != nil {
		return 0, err
	}

	return strconv.ParseUint(data, 10, 64)
}

func readFileBitmask(path string) (Bitmask, error) {
	data, err := readFileString(path)
	if err != nil {
		return 0, err
	}

	value, err := strconv.ParseUint(data, 16, 64)
	return Bitmask(value), err
}

func readFileString(path string) (string, error) {
	data, err := ioutil.ReadFile(path)
	return strings.TrimSpace(string(data)), err
}
