package cgroups

import (
	"fmt"
	"io/ioutil"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// BlkioDeviceBytes contains a single operations line of blkio.throttle.io_service_bytes_recursive file
type BlkioDeviceBytes struct {
	Major      int
	Minor      int
	Operations map[string]int64
}

// BlkioThrottleBytes has parsed contents of blkio.throttle.io_service_bytes_recursive file
type BlkioThrottleBytes struct {
	DeviceBytes []BlkioDeviceBytes
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

var blkLineRe = regexp.MustCompile(`^(\d+):(\d+) (\w+) (\d+)$`)

// GetBlkioThrottleBytes returns amount of bytes transferred to/from the disk.
func GetBlkioThrottleBytes(cgroupPath string) (BlkioThrottleBytes, error) {

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

	lines, err := readCgroupFileLines(path.Join(cgroupPath, "blkio.throttle.io_service_bytes_recursive"))

	if err != nil {
		return BlkioThrottleBytes{}, err
	}

	if len(lines) == 1 && lines[0] == "Total 0" {
		return BlkioThrottleBytes{TotalBytes: 0}, nil
	}

	result := BlkioThrottleBytes{}
	result.DeviceBytes = make([]BlkioDeviceBytes, 0)

	for _, line := range lines {
		if line[:5] == "Total" {
			tokens := strings.Split(line, " ")
			if len(tokens) == 2 {
				totalBytes, err := strconv.ParseInt(tokens[1], 10, 64)
				if err != nil {
					return BlkioThrottleBytes{}, err
				}
				result.TotalBytes = totalBytes
			}
		} else {
			tokens := blkLineRe.FindStringSubmatch(line)
			if tokens == nil || len(tokens) != 5 {
				return BlkioThrottleBytes{}, fmt.Errorf("error parsing file")
			}
			major64, err := strconv.ParseInt(string(tokens[1]), 10, 32)
			if err != nil {
				return BlkioThrottleBytes{}, err
			}
			minor64, err := strconv.ParseInt(string(tokens[2]), 10, 32)
			if err != nil {
				return BlkioThrottleBytes{}, err
			}
			major := int(major64)
			minor := int(minor64)

			var device *BlkioDeviceBytes
			// Find if we already have the device. There should only be a few so this isn't too costly.
			for _, dev := range result.DeviceBytes {
				if dev.Major == major && dev.Minor == minor {
					device = &dev
				}
			}

			if device == nil {
				device = &BlkioDeviceBytes{Major: major, Minor: minor, Operations: make(map[string]int64)}
				result.DeviceBytes = append(result.DeviceBytes, *device)
			}
			bytes, err := strconv.ParseInt(string(tokens[4]), 10, 64)
			if err != nil {
				return BlkioThrottleBytes{}, err
			}

			device.Operations[string(tokens[3])] = bytes
		}
	}

	return result, nil
}

// GetCPUAcctStats retrieves CPU account statistics for a given cgroup.
func GetCPUAcctStats(cgroupPath string) ([]CPUAcctUsage, error) {

	// File looks like this:
	//
	// cpu user system
	// 0 3723082232186 2456599218
	// 1 3748398003001 1149546796

	lines, err := readCgroupFileLines(path.Join(cgroupPath, "cpuacct.usage_all"))

	if err != nil {
		return nil, err
	}

	result := make([]CPUAcctUsage, len(lines)-1)

	for i, line := range lines {
		// skip first line
		if i > 0 && len(line) > 0 {
			tokens := strings.Split(line, " ")
			if len(tokens) == 3 {
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
				result[i-1] = CPUAcctUsage{CPU: int(cpu), User: user, System: system}
			}
		}
	}
	return result, nil
}

// GetCPUSetMemoryMigrate returns boolean indicating whether memory migration is enabled.
func GetCPUSetMemoryMigrate(cgroupPath string) (bool, error) {

	// File looks like this:
	//
	// 0

	number, err := readCgroupSingleNumber(path.Join(cgroupPath, "cpuset.memory_migrate"))

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

var hugetlbUsageInBytesRe = regexp.MustCompile(`^\S*/hugetlb\.(.*)\.usage_in_bytes$`)
var hugetlbMaxUsageInBytesRe = regexp.MustCompile(`^\S*/hugetlb\.(.*)\.max_usage_in_bytes$`)

// GetHugetlbUsage retrieves huge pages statistics for a given cgroup.
func GetHugetlbUsage(cgroupPath string) ([]HugetlbUsage, error) {

	// Files look like this:
	//
	// 124

	usageFiles, err := filepath.Glob(path.Join(cgroupPath, "/hugetlb.*.usage_in_bytes"))

	if err != nil {
		return nil, err
	}

	maxUsageFiles, err := filepath.Glob(path.Join(cgroupPath, "/hugetlb.*.max_usage_in_bytes"))

	if err != nil {
		return nil, err
	}

	if len(usageFiles) != len(maxUsageFiles) {
		return nil, fmt.Errorf("error finding files")
	}

	result := make([]HugetlbUsage, len(usageFiles))

	for i, file := range usageFiles {
		tokens := hugetlbUsageInBytesRe.FindStringSubmatch(file)
		if tokens == nil || len(tokens) != 2 {
			return nil, fmt.Errorf("error finding files")
		}
		size := string(tokens[1])

		result[i].Size = size

		number, err := readCgroupSingleNumber(file)

		if err != nil {
			return nil, err
		}

		result[i].Bytes = number
	}

	for _, file := range maxUsageFiles {
		tokens := hugetlbMaxUsageInBytesRe.FindStringSubmatch(file)
		if tokens == nil || len(tokens) != 2 {
			return nil, fmt.Errorf("error finding files")
		}
		size := string(tokens[1])

		// Find the already existing result.

		var res *HugetlbUsage

		for j, tmp := range result {
			if tmp.Size == size {
				res = &result[j]
			}
		}
		if res == nil {
			return nil, fmt.Errorf("error finding files")
		}

		number, err := readCgroupSingleNumber(file)

		if err != nil {
			return nil, err
		}

		res.MaxBytes = number
	}

	return result, nil
}

// GetMemoryUsage retrieves cgroup memory usage.
func GetMemoryUsage(cgroupPath string) (MemoryUsage, error) {

	// Files look like this:
	//
	// 142

	usage, err := readCgroupSingleNumber(path.Join(cgroupPath, "memory.usage_in_bytes"))
	if err != nil {
		return MemoryUsage{}, err
	}

	maxUsage, err := readCgroupSingleNumber(path.Join(cgroupPath, "memory.max_usage_in_bytes"))
	if err != nil {
		return MemoryUsage{}, err
	}

	result := MemoryUsage{
		Bytes:    usage,
		MaxBytes: maxUsage,
	}

	return result, nil
}

var numaLineRe = regexp.MustCompile(`^(\S+)=(\d+)([ N\d+=\d+]+)$`)
var numaNodesRe = regexp.MustCompile(`(N\d+)=(\d+)`)

// GetNumaStats returns parsed cgroup NUMA statistics.
func GetNumaStats(cgroupPath string) (NumaStat, error) {

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

	lines, err := readCgroupFileLines(path.Join(cgroupPath, "memory.numa_stat"))

	if err != nil {
		return NumaStat{}, err
	}

	result := NumaStat{}
	for _, line := range lines {
		tokens := numaLineRe.FindStringSubmatch(line)
		if tokens == nil || len(tokens) != 4 {
			return NumaStat{}, fmt.Errorf("error parsing file")
		}
		number, err := strconv.ParseInt(tokens[2], 10, 64)
		if err != nil {
			return NumaStat{}, err
		}

		// Parse node list separately.
		nodeMatches := numaNodesRe.FindAllStringSubmatch(tokens[3], -1)

		nodes := make(map[string]int64)
		for _, nodeMatch := range nodeMatches {
			number, err := strconv.ParseInt(nodeMatch[2], 10, 64)
			if err != nil {
				return NumaStat{}, err
			}
			nodes[nodeMatch[1]] = number
		}

		switch tokens[1] {
		case "total":
			result.Total.Total = number
			result.Total.Nodes = nodes
		case "file":
			result.File.Total = number
			result.File.Nodes = nodes
		case "anon":
			result.Anon.Total = number
			result.Anon.Nodes = nodes
		case "unevictable":
			result.Unevictable.Total = number
			result.Unevictable.Nodes = nodes
		case "hierarchical_total":
			result.HierarchicalTotal.Total = number
			result.HierarchicalTotal.Nodes = nodes
		case "hierarchical_file":
			result.HierarchicalFile.Total = number
			result.HierarchicalFile.Nodes = nodes
		case "hierarchical_anon":
			result.HierarchicalAnon.Total = number
			result.HierarchicalAnon.Nodes = nodes
		case "hierarchical_unevictable":
			result.HierarchicalUnevictable.Total = number
			result.HierarchicalUnevictable.Nodes = nodes
		default:
			return NumaStat{}, fmt.Errorf("error parsing file")
		}
	}

	return result, nil
}

var globalNumaNodeRe = regexp.MustCompile(`^\S*(\d+)$`)

// GetGlobalNumaStats returns the global (non-cgroup) NUMA statistics per node.
func GetGlobalNumaStats() (map[int]GlobalNumaStats, error) {

	// Files look like this:
	//
	// numa_hit 1851614569
	// numa_miss 0
	// numa_foreign 0
	// interleave_hit 49101
	// local_node 1851614569
	// other_node 0

	result := make(map[int]GlobalNumaStats)

	nodeDirs, err := filepath.Glob("/sys/devices/system/node/node*")

	if err != nil {
		return map[int]GlobalNumaStats{}, err
	}

	if len(nodeDirs) <= 0 {
		return map[int]GlobalNumaStats{}, fmt.Errorf("error finding files")
	}

	for _, dir := range nodeDirs {
		nameTokens := globalNumaNodeRe.FindStringSubmatch(dir)
		if nameTokens == nil || len(nameTokens) != 2 {
			return map[int]GlobalNumaStats{}, fmt.Errorf("error parsing directory name")
		}
		nodeNumber, err := strconv.ParseInt(nameTokens[1], 10, 64)
		if err != nil {
			return map[int]GlobalNumaStats{}, fmt.Errorf("error parsing directory name")
		}

		nodeStat := GlobalNumaStats{}

		numastat := path.Join(dir, "numastat")
		lines, err := readCgroupFileLines(numastat)
		if err != nil {
			return map[int]GlobalNumaStats{}, fmt.Errorf("error reading %s", numastat)
		}

		for _, line := range lines {
			tokens := strings.Split(line, " ")
			if len(tokens) != 2 {
				return map[int]GlobalNumaStats{}, fmt.Errorf("error parsing numastat file")
			}
			number, err := strconv.ParseInt(tokens[1], 10, 64)
			if err != nil {
				return map[int]GlobalNumaStats{}, fmt.Errorf("error parsing numastat file")
			}
			switch tokens[0] {
			case "numa_hit":
				nodeStat.NumaHit = number
			case "numa_miss":
				nodeStat.NumaMiss = number
			case "numa_foreign":
				nodeStat.NumaForeign = number
			case "interleave_hit":
				nodeStat.InterleaveHit = number
			case "local_node":
				nodeStat.LocalNode = number
			case "other_node":
				nodeStat.OtherNode = number
			default:
				return map[int]GlobalNumaStats{}, fmt.Errorf("error parsing numastat file")
			}
		}

		result[int(nodeNumber)] = nodeStat
	}

	return result, nil
}
