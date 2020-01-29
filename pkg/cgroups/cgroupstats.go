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

package cgroups

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strconv"
	"strings"

	logger "github.com/intel/cri-resource-manager/pkg/log"
	"github.com/intel/cri-resource-manager/pkg/sysfs"
)

// BlkioDeviceBytes contains a single operations line of blkio.throttle.io_service_bytes_recursive file
type BlkioDeviceBytes struct {
	Major      int
	Minor      int
	Operations map[string]int64
}

// BlkioThrottleBytes has parsed contents of blkio.throttle.io_service_bytes_recursive file
type BlkioThrottleBytes struct {
	DeviceBytes []*BlkioDeviceBytes
	TotalBytes  int64
}

// CPUAcctUsage has a parsed line of cpuacct.usage_all file
type CPUAcctUsage struct {
	CPU    int
	User   int64
	System int64
}

// HugetlbUsage has parsed contents of huge pages usage in bytes.
type HugetlbUsage struct {
	Size     string
	Bytes    int64
	MaxBytes int64
}

// MemoryUsage has parsed contents of memory usage in bytes.
type MemoryUsage struct {
	Bytes    int64
	MaxBytes int64
}

// NumaLine represents one line in the NUMA statistics file.
type NumaLine struct {
	Total int64
	Nodes map[string]int64
}

// NumaStat has parsed contets of a NUMA statistics file.
type NumaStat struct {
	Total       NumaLine
	File        NumaLine
	Anon        NumaLine
	Unevictable NumaLine

	HierarchicalTotal       NumaLine
	HierarchicalFile        NumaLine
	HierarchicalAnon        NumaLine
	HierarchicalUnevictable NumaLine
}

// GlobalNumaStats has the statistics from one global NUMA nodestats file.
type GlobalNumaStats struct {
	NumaHit       int64
	NumaMiss      int64
	NumaForeign   int64
	InterleaveHit int64
	LocalNode     int64
	OtherNode     int64
}

// our logger instance
var log = logger.NewLogger("cgroup-metrics")

func readCgroupFileLines(filePath string) ([]string, error) {

	f, err := ioutil.ReadFile(filePath)

	if err != nil {
		return nil, err
	}

	data := string(f)

	rawLines := strings.Split(data, "\n")

	lines := make([]string, 0)

	// Sanitize the lines and remove empty ones.
	for _, rawLine := range rawLines {
		if len(strings.TrimSpace(rawLine)) > 0 {
			lines = append(lines, rawLine)
		}
	}

	return lines, nil
}

func readCgroupSingleNumber(filePath string) (int64, error) {

	// File looks like this:
	//
	// 4

	lines, err := readCgroupFileLines(filePath)

	if err != nil {
		return 0, err
	}

	if len(lines) != 1 {
		return 0, fmt.Errorf("error parsing file")
	}

	number, err := strconv.ParseInt(lines[0], 10, 64)
	if err != nil {
		return 0, err
	}

	return number, nil
}

// GetBlkioThrottleBytes returns amount of bytes transferred to/from the disk.
func GetBlkioThrottleBytes(cgroupPath string) (BlkioThrottleBytes, error) {
	const (
		cgroupEntry = "blkio.throttle.io_service_bytes_recursive"
	)

	// File looks like this:
	//
	// 8:16 Read 4223325184
	// 8:16 Write 3207528448
	// 8:16 Sync 5387592704
	// 8:16 Async 2043260928
	// 8:16 Discard 0
	// 8:16 Total 7430853632
	// 8:0 Read 5246572032
	// 8:0 Write 2361737216
	// 8:0 Sync 5575892480
	// 8:0 Async 2032416768
	// 8:0 Discard 0
	// 8:0 Total 7608309248
	// Total 15039162880

	var devops *BlkioDeviceBytes
	var device string

	result := BlkioThrottleBytes{DeviceBytes: make([]*BlkioDeviceBytes, 0)}
	path := filepath.Join(cgroupPath, cgroupEntry)
	err := sysfs.ParseFileByLines(path,
		// split line to fields
		func(line string) ([]*sysfs.Field, error) {
			split := strings.Split(line, " ")
			switch len(split) {
			case 2:
				if split[0] != "Total" {
					return nil, fmt.Errorf("unknown entry (%s)", split[0])
				}
				return []*sysfs.Field{{Index: 0, Key: "Total", Value: split[1]}}, nil
			case 3:
				return []*sysfs.Field{
					{Index: 1, Key: "Device", Value: split[0]},
					{Index: 2, Key: split[1], Value: split[2]},
				}, nil
			default:
				return nil, fmt.Errorf("unknown entry")
			}
		},
		// initialize/pick result.DeviceBytes.BlkioDeviceBytes
		func(_ int, f *sysfs.Field) (interface{}, error) {
			switch {
			case f.Key == "Total":
				return &result.TotalBytes, nil
			case f.Key == "Device":
				if device != f.Value {
					maj, min, err := parseDeviceNumber(f.Value)
					if err != nil {
						return nil, fmt.Errorf("invalid device number %s: %v", f.Value, err)
					}
					device = f.Value
					devops = &BlkioDeviceBytes{
						Major:      maj,
						Minor:      min,
						Operations: make(map[string]int64),
					}
					result.DeviceBytes = append(result.DeviceBytes, devops)
				}
				return nil, sysfs.ErrSkip
			default:
				if devops == nil {
					return nil, fmt.Errorf("invalid entry %s, 'Device' expected", f.Key)
				}
				return devops.Operations, nil
			}
		},
	)

	if err != nil {
		return BlkioThrottleBytes{}, fmt.Errorf("error parsing file %s: %v", path, err)
	}

	return result, nil
}

// parseDeviceNumber parses a device 'major:minor' string into numerical major, minor.
func parseDeviceNumber(dev string) (int, int, error) {
	majmin := strings.Split(dev, ":")
	if len(majmin) != 2 {
		return 0, 0, fmt.Errorf("invalid device %s, major:minor expected", dev)
	}
	maj, err := strconv.ParseInt(majmin[0], 10, 32)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid device major %s: %v", majmin[0], err)
	}
	min, err := strconv.ParseInt(majmin[1], 10, 32)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid device minor %s: %v", majmin[1], err)
	}
	return int(maj), int(min), nil
}

// GetCPUAcctStats retrieves CPU account statistics for a given cgroup.
func GetCPUAcctStats(cgroupPath string) ([]CPUAcctUsage, error) {

	// File looks like this:
	//
	// cpu user system
	// 0 3723082232186 2456599218
	// 1 3748398003001 1149546796

	lines, err := readCgroupFileLines(filepath.Join(cgroupPath, "cpuacct.usage_all"))

	if err != nil {
		return nil, err
	}

	result := make([]CPUAcctUsage, 0, len(lines)-1)

	for _, line := range lines[1:] {
		tokens := strings.Split(line, " ")
		if len(tokens) != 3 {
			continue
		}
		cpu, err := strconv.ParseInt(tokens[0], 10, 32)
		if err != nil {
			return nil, err
		}
		user, err := strconv.ParseInt(tokens[1], 10, 64)
		if err != nil {
			return nil, err
		}
		system, err := strconv.ParseInt(tokens[2], 10, 64)
		if err != nil {
			return nil, err
		}
		result = append(result, CPUAcctUsage{CPU: int(cpu), User: user, System: system})
	}
	return result, nil
}

// GetCPUSetMemoryMigrate returns boolean indicating whether memory migration is enabled.
func GetCPUSetMemoryMigrate(cgroupPath string) (bool, error) {

	// File looks like this:
	//
	// 0

	number, err := readCgroupSingleNumber(filepath.Join(cgroupPath, "cpuset.memory_migrate"))

	if err != nil {
		return false, err
	}

	if number == 0 {
		return false, nil
	} else if number == 1 {
		return true, nil
	}

	return false, fmt.Errorf("error parsing file")
}

// GetHugetlbUsage retrieves huge pages statistics for a given cgroup.
func GetHugetlbUsage(cgroupPath string) ([]HugetlbUsage, error) {
	const (
		prefix         = "/hugetlb."
		usageSuffix    = ".usage_in_bytes"
		maxUsageSuffix = ".max_usage_in_bytes"
	)

	// Files look like this:
	//
	// 124

	usageFiles, err := filepath.Glob(filepath.Join(cgroupPath, prefix+"*"+usageSuffix))
	if err != nil {
		return nil, err
	}

	result := make([]HugetlbUsage, 0, len(usageFiles))

	for _, file := range usageFiles {
		size := strings.TrimSuffix(strings.TrimPrefix(file, prefix), usageSuffix)
		bytes, err := readCgroupSingleNumber(file)
		if err != nil {
			return nil, err
		}
		max, err := readCgroupSingleNumber(strings.TrimSuffix(file, usageSuffix) + maxUsageSuffix)
		if err != nil {
			return nil, err
		}
		result = append(result, HugetlbUsage{
			Size:     size,
			Bytes:    bytes,
			MaxBytes: max,
		})
	}

	return result, nil
}

// GetMemoryUsage retrieves cgroup memory usage.
func GetMemoryUsage(cgroupPath string) (MemoryUsage, error) {

	// Files look like this:
	//
	// 142

	usage, err := readCgroupSingleNumber(filepath.Join(cgroupPath, "memory.usage_in_bytes"))
	if err != nil {
		return MemoryUsage{}, err
	}

	maxUsage, err := readCgroupSingleNumber(filepath.Join(cgroupPath, "memory.max_usage_in_bytes"))
	if err != nil {
		return MemoryUsage{}, err
	}

	result := MemoryUsage{
		Bytes:    usage,
		MaxBytes: maxUsage,
	}

	return result, nil
}

// GetNumaStats returns parsed cgroup NUMA statistics.
func GetNumaStats(cgroupPath string) (NumaStat, error) {
	const (
		cgroupEntry = "memory.numa_stat"
	)

	// File looks like this:
	//
	// total=44611 N0=32631 N1=7501 N2=1982 N3=2497
	// file=44428 N0=32614 N1=7335 N2=1982 N3=2497
	// anon=183 N0=17 N1=166 N2=0 N3=0
	// unevictable=0 N0=0 N1=0 N2=0 N3=0
	// hierarchical_total=768133 N0=509113 N1=138887 N2=20464 N3=99669
	// hierarchical_file=722017 N0=496516 N1=119997 N2=20181 N3=85323
	// hierarchical_anon=46096 N0=12597 N1=18890 N2=283 N3=14326
	// hierarchical_unevictable=20 N0=0 N1=0 N2=0 N3=20

	var nline *NumaLine

	result := NumaStat{}
	path := filepath.Join(cgroupPath, cgroupEntry)
	err := sysfs.ParseFileByLines(path,
		// split line to fields by spaces
		func(line string) ([]*sysfs.Field, error) {
			split := strings.Split(line, " ")
			fields := make([]*sysfs.Field, len(split))
			for idx, entry := range split {
				keyval := strings.Split(entry, "=")
				if len(keyval) != 2 {
					return nil, fmt.Errorf("failed to parse entry '%s' of line '%s'", entry, line)
				}
				fields[idx] = &sysfs.Field{
					Index: idx,
					Key:   keyval[0],
					Value: keyval[1],
				}
			}
			return fields, nil
		},
		// pick NumaLine.Total or pick NumaLine.Nodes, based on this/last non-node field key
		func(_ int, f *sysfs.Field) (interface{}, error) {
			switch f.Key {
			case "total":
				nline = &result.Total
			case "file":
				nline = &result.File
			case "anon":
				nline = &result.Anon
			case "unevictable":
				nline = &result.Unevictable
			case "hierarchical_total":
				nline = &result.HierarchicalTotal
			case "hierarchical_file":
				nline = &result.HierarchicalFile
			case "hierarchical_anon":
				nline = &result.HierarchicalAnon
			case "hierarchical_unevictable":
				nline = &result.HierarchicalUnevictable
			default:
				if f.Key[0] != 'N' {
					return nil, fmt.Errorf("unknown field key %s", f.Key)
				}
			}

			switch {
			case nline == nil:
				return nil, fmt.Errorf("node data for unknown NumaStat entry")
			case f.Key[0] != 'N':
				log.Debug("NumaStat(%s): %s = %s",
					logger.Delay(func() string { return filepath.Base(cgroupPath) }), f.Key, f.Value)
				return &nline.Total, nil
			default: // f.Key[0] == 'N'
				log.Debug("NumaStat(%s): %s = %s",
					logger.Delay(func() string { return filepath.Base(cgroupPath) }), f.Key, f.Value)
				if nline.Nodes == nil {
					nline.Nodes = make(map[string]int64)
				}
				return nline.Nodes, nil
			}
		},
	)

	if err != nil {
		return NumaStat{}, fmt.Errorf("error parsing file %s: %v", path, err)
	}

	return result, nil
}

// GetGlobalNumaStats returns the global (non-cgroup) NUMA statistics per node.
func GetGlobalNumaStats() (map[int]GlobalNumaStats, error) {
	const (
		prefix = "/sys/devices/system/node/node"
	)

	// Files look like this:
	//
	// numa_hit 1851614569
	// numa_miss 0
	// numa_foreign 0
	// interleave_hit 49101
	// local_node 1851614569
	// other_node 0

	result := make(map[int]GlobalNumaStats)

	nodeDirs, err := filepath.Glob(prefix + "*")
	if err != nil {
		return map[int]GlobalNumaStats{}, err
	}

	log.Debug("GlobalNumaStat: nodedirs: %s",
		logger.Delay(func() string { return strings.Join(nodeDirs, ", ") }))

	for _, dir := range nodeDirs {
		id := strings.TrimPrefix(dir, prefix)
		node, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			return map[int]GlobalNumaStats{}, fmt.Errorf("error parsing directory name")
		}

		nodeStat := GlobalNumaStats{}

		numastat := filepath.Join(dir, "numastat")
		err = sysfs.ParseFileEntries(numastat,
			map[string]interface{}{
				"numa_hit":       &nodeStat.NumaHit,
				"numa_miss":      &nodeStat.NumaMiss,
				"numa_foreign":   &nodeStat.NumaForeign,
				"interleave_hit": &nodeStat.InterleaveHit,
				"local_node":     &nodeStat.LocalNode,
				"other_node":     &nodeStat.OtherNode,
			},
			func(line string) (string, string, error) {
				fields := strings.Fields(strings.TrimSpace(line))
				if len(fields) != 2 {
					return "", "", fmt.Errorf("failed to parse line '%s'", line)
				}
				log.Debug("GlobalNumaStat(%s) %s = %s",
					logger.Delay(func() string { return filepath.Base(dir) }), fields[0], fields[1])
				return fields[0], fields[1], nil
			},
		)

		if err != nil {
			return map[int]GlobalNumaStats{}, fmt.Errorf("error parsing numastat file: %v", err)
		}

		result[int(node)] = nodeStat
	}

	return result, nil
}
