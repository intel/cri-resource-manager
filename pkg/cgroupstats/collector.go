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

package cgroupstats

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/intel/cri-resource-manager/pkg/cgroups"
	logger "github.com/intel/cri-resource-manager/pkg/log"
	"github.com/intel/cri-resource-manager/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

// Prometheus Metric descriptor indices and descriptor table
const (
	numaStatsDesc = iota
	memoryUsageDesc
	memoryMigrateDesc
	cpuAcctUsageDesc
	hugeTlbUsageDesc
	blkioDeviceUsageDesc
	numDescriptors
)

var descriptors = [numDescriptors]*prometheus.Desc{
	numaStatsDesc: prometheus.NewDesc(
		"cgroup_numa_stats",
		"NUMA statistics for a given container and pod.",
		[]string{
			// cgroup path
			"container_id",
			// NUMA node ID
			"numa_node_id",
			// NUMA memory type
			"type",
		}, nil,
	),
	memoryUsageDesc: prometheus.NewDesc(
		"cgroup_memory_usage",
		"Memory usage statistics for a given container and pod.",
		[]string{
			"container_id",
			"type",
		}, nil,
	),
	memoryMigrateDesc: prometheus.NewDesc(
		"cgroup_memory_migrate",
		"Memory migrate status for a given container and pod.",
		[]string{
			"container_id",
		}, nil,
	),
	cpuAcctUsageDesc: prometheus.NewDesc(
		"cgroup_cpu_acct",
		"CPU accounting for a given container and pod.",
		[]string{
			"container_id",
			// CPU ID
			"cpu",
			"type",
		}, nil,
	),
	hugeTlbUsageDesc: prometheus.NewDesc(
		"cgroup_hugetlb_usage",
		"Hugepages usage for a given container and pod.",
		[]string{
			"container_id",
			"size",
			"type",
		}, nil,
	),
	blkioDeviceUsageDesc: prometheus.NewDesc(
		"cgroup_blkio_device_usage",
		"Blkio Device bytes usage for a given container and pod.",
		[]string{
			"container_id",
			"major",
			"minor",
			"operation",
		}, nil,
	),
}

var (
	// cgroupRoot is the mount point for the cgroup (v1) filesystem
	cgroupRoot = "/sys/fs/cgroup"
	// our logger instance
	log = logger.NewLogger("cgroupstats")
)

const (
	kubepodsDir = "kubepods.slice"
)

type collector struct {
}

// NewCollector creates new Prometheus collector
func NewCollector() (prometheus.Collector, error) {
	return &collector{}, nil
}

// Describe implements prometheus.Collector interface
func (c *collector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range descriptors {
		ch <- d
	}
}

func updateCPUAcctUsageMetric(ch chan<- prometheus.Metric, path string, metric []cgroups.CPUAcctUsage) {
	for i, acct := range metric {
		ch <- prometheus.MustNewConstMetric(
			descriptors[cpuAcctUsageDesc],
			prometheus.CounterValue,
			float64(acct.CPU),
			path, strconv.FormatInt(int64(i), 10), "CPU",
		)
		ch <- prometheus.MustNewConstMetric(
			descriptors[cpuAcctUsageDesc],
			prometheus.CounterValue,
			float64(acct.User),
			path, strconv.FormatInt(int64(i), 10), "User",
		)
		ch <- prometheus.MustNewConstMetric(
			descriptors[cpuAcctUsageDesc],
			prometheus.CounterValue,
			float64(acct.System),
			path, strconv.FormatInt(int64(i), 10), "System",
		)
	}
}

func updateMemoryMigrateMetric(ch chan<- prometheus.Metric, path string, migrate bool) {
	migrateValue := 0
	if migrate {
		migrateValue = 1
	}
	ch <- prometheus.MustNewConstMetric(
		descriptors[memoryMigrateDesc],
		prometheus.GaugeValue,
		float64(migrateValue),
		path,
	)
}

func updateMemoryUsageMetric(ch chan<- prometheus.Metric, path string, metric cgroups.MemoryUsage) {
	ch <- prometheus.MustNewConstMetric(
		descriptors[memoryUsageDesc],
		prometheus.GaugeValue,
		float64(metric.Bytes),
		path, "Bytes",
	)
	ch <- prometheus.MustNewConstMetric(
		descriptors[memoryUsageDesc],
		prometheus.GaugeValue,
		float64(metric.MaxBytes),
		path, "MaxBytes",
	)
}

func updateNumaStatMetric(ch chan<- prometheus.Metric, path string, metric cgroups.NumaStat) {
	// TODO: use "reflect" to iterate through the struct fields of NumaStat?

	for key, value := range metric.Total.Nodes {
		ch <- prometheus.MustNewConstMetric(
			descriptors[numaStatsDesc],
			prometheus.GaugeValue,
			float64(value),
			path, key, "Total",
		)
	}
	for key, value := range metric.File.Nodes {
		ch <- prometheus.MustNewConstMetric(
			descriptors[numaStatsDesc],
			prometheus.GaugeValue,
			float64(value),
			path, key, "File",
		)
	}
	for key, value := range metric.Anon.Nodes {
		ch <- prometheus.MustNewConstMetric(
			descriptors[numaStatsDesc],
			prometheus.GaugeValue,
			float64(value),
			path, key, "Anon",
		)
	}
	for key, value := range metric.Unevictable.Nodes {
		ch <- prometheus.MustNewConstMetric(
			descriptors[numaStatsDesc],
			prometheus.GaugeValue,
			float64(value),
			path, key, "Unevictable",
		)
	}
	for key, value := range metric.HierarchicalTotal.Nodes {
		ch <- prometheus.MustNewConstMetric(
			descriptors[numaStatsDesc],
			prometheus.GaugeValue,
			float64(value),
			path, key, "HierarchicalTotal",
		)
	}
	for key, value := range metric.HierarchicalFile.Nodes {
		ch <- prometheus.MustNewConstMetric(
			descriptors[numaStatsDesc],
			prometheus.GaugeValue,
			float64(value),
			path, key, "HierarchicalFile",
		)
	}
	for key, value := range metric.HierarchicalAnon.Nodes {
		ch <- prometheus.MustNewConstMetric(
			descriptors[numaStatsDesc],
			prometheus.GaugeValue,
			float64(value),
			path, key, "HierarchicalAnon",
		)
	}
	for key, value := range metric.HierarchicalUnevictable.Nodes {
		ch <- prometheus.MustNewConstMetric(
			descriptors[numaStatsDesc],
			prometheus.GaugeValue,
			float64(value),
			path, key, "HierarchicalUnevictable",
		)
	}
}

func updateHugeTlbUsageMetric(ch chan<- prometheus.Metric, path string, metric []cgroups.HugetlbUsage) {
	// One HugeTlbUsage for each size.
	for _, hugeTlbUsage := range metric {
		ch <- prometheus.MustNewConstMetric(
			descriptors[hugeTlbUsageDesc],
			prometheus.GaugeValue,
			float64(hugeTlbUsage.Bytes),
			path, hugeTlbUsage.Size, "Bytes",
		)
		ch <- prometheus.MustNewConstMetric(
			descriptors[hugeTlbUsageDesc],
			prometheus.GaugeValue,
			float64(hugeTlbUsage.MaxBytes),
			path, hugeTlbUsage.Size, "MaxBytes",
		)
	}
}

func updateBlkioDeviceUsageMetric(ch chan<- prometheus.Metric, path string, metric cgroups.BlkioThrottleBytes) {
	for _, deviceBytes := range metric.DeviceBytes {
		for operation, val := range deviceBytes.Operations {
			ch <- prometheus.MustNewConstMetric(
				descriptors[blkioDeviceUsageDesc],
				prometheus.CounterValue,
				float64(val),
				path, strconv.FormatInt(int64(deviceBytes.Major), 10),
				strconv.FormatInt(int64(deviceBytes.Minor), 10), operation,
			)
		}
	}
}

func walkCgroups() []string {
	// XXX TODO: add support for kubelet cgroupfs cgroup driver.

	containerDirs := []string{}

	cpuset := filepath.Join(cgroupRoot, "cpuset")
	filepath.Walk(filepath.Join(cpuset, kubepodsDir),
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return nil
				}
				return err
			}
			if !info.IsDir() {
				return nil
			}

			dir := info.Name()
			if !strings.HasSuffix(dir, ".scope") {
				return nil
			}

			switch {
			case strings.HasPrefix(dir, "cri-containerd-"):
				break
			case strings.HasPrefix(dir, "crio-"):
				break
			case strings.HasPrefix(dir, "docker-"):
				break
			default:
				return filepath.SkipDir
			}

			path = strings.TrimPrefix(path, cpuset+"/")
			containerDirs = append(containerDirs, path)

			return nil
		})

	return containerDirs
}

func cgroupPath(controller, path string) string {
	return filepath.Join(cgroupRoot, controller, path)
}

// Collect implements prometheus.Collector interface
func (c collector) Collect(ch chan<- prometheus.Metric) {
	var wg sync.WaitGroup

	// We don't bail out on errors because those can happen if there is a race condition between
	// the destruction of a container and us getting to read the cgroup data. We just don't report
	// the values we don't get.

	collectors := []func(string, *regexp.Regexp){
		func(path string, re *regexp.Regexp) {
			defer wg.Done()
			numa, err := cgroups.GetNumaStats(cgroupPath("memory", path))
			if err == nil {
				updateNumaStatMetric(ch, re.FindStringSubmatch(filepath.Base(path))[0], numa)
			} else {
				log.Errorf("failed to collect NUMA stats for %s: %v", path, err)
			}
		},
		func(path string, re *regexp.Regexp) {
			defer wg.Done()
			memory, err := cgroups.GetMemoryUsage(cgroupPath("memory", path))
			if err == nil {
				updateMemoryUsageMetric(ch, re.FindStringSubmatch(filepath.Base(path))[0], memory)
			} else {
				log.Errorf("failed to collect memory usage stats for %s: %v", path, err)
			}
		},
		func(path string, re *regexp.Regexp) {
			defer wg.Done()
			migrate, err := cgroups.GetCPUSetMemoryMigrate(cgroupPath("cpuset", path))
			if err == nil {
				updateMemoryMigrateMetric(ch, re.FindStringSubmatch(filepath.Base(path))[0], migrate)
			} else {
				log.Errorf("failed to collect memory migration stats for %s: %v", path, err)
			}
		},
		func(path string, re *regexp.Regexp) {
			defer wg.Done()
			cpuAcctUsage, err := cgroups.GetCPUAcctStats(cgroupPath("cpuacct", path))
			if err == nil {
				updateCPUAcctUsageMetric(ch, re.FindStringSubmatch(filepath.Base(path))[0], cpuAcctUsage)
			} else {
				log.Errorf("failed to collect CPU accounting stats for %s: %v", path, err)
			}
		},
		func(path string, re *regexp.Regexp) {
			defer wg.Done()
			hugeTlbUsage, err := cgroups.GetHugetlbUsage(cgroupPath("hugetlb", path))
			if err == nil {
				updateHugeTlbUsageMetric(ch, re.FindStringSubmatch(filepath.Base(path))[0], hugeTlbUsage)
			} else {
				log.Errorf("failed to collect hugetlb stats for %s: %v", path, err)
			}
		},
		func(path string, re *regexp.Regexp) {
			defer wg.Done()
			blkioDeviceUsage, err := cgroups.GetBlkioThrottleBytes(cgroupPath("blkio", path))
			if err == nil {
				updateBlkioDeviceUsageMetric(ch, re.FindStringSubmatch(filepath.Base(path))[0], blkioDeviceUsage)
			} else {
				log.Errorf("failed to collect blkio stats for %s: %v", path, err)
			}
		},
	}

	containerIDRegexp := regexp.MustCompile(`[a-z0-9]{64}`)

	for _, path := range walkCgroups() {
		wg.Add(len(collectors))
		for _, fn := range collectors {
			go fn(path, containerIDRegexp)
		}
	}

	// We need to wait so that the response channel doesn't get closed.
	wg.Wait()
}

func init() {
	flag.StringVar(&cgroupRoot, "cgroup-path", cgroupRoot,
		"Path to cgroup filesystem mountpoint")

	err := metrics.RegisterCollector("cgroupstats", NewCollector)
	if err != nil {
		log.Errorf("failed register cgroupstats collector: %v", err)
	}
}
