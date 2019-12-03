package cgroupstats

import (
	"flag"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/intel/cri-resource-manager/pkg/cgroups"
	logger "github.com/intel/cri-resource-manager/pkg/log"
	"github.com/intel/cri-resource-manager/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	numaStatsDesc = prometheus.NewDesc(
		"cgroup_numa_stats",
		"NUMA statistics for a given container and pod.",
		[]string{
			// cgroup path
			"cgroup_path",
			// NUMA node ID
			"numa_node_id",
			// NUMA memory type
			"type",
		}, nil,
	)

	memoryUsageDesc = prometheus.NewDesc(
		"cgroup_memory_usage",
		"Memory usage statistics for a given container and pod.",
		[]string{
			"cgroup_path",
			"type",
		}, nil,
	)

	memoryMigrateDesc = prometheus.NewDesc(
		"cgroup_memory_migrate",
		"Memory migrate status for a given container and pod.",
		[]string{
			"cgroup_path",
		}, nil,
	)

	cpuAcctUsageDesc = prometheus.NewDesc(
		"cgroup_cpu_acct",
		"CPU accounting for a given container and pod.",
		[]string{
			"cgroup_path",
			// CPU ID
			"cpu",
			"type",
		}, nil,
	)

	hugeTlbUsageDesc = prometheus.NewDesc(
		"cgroup_hugetlb_usage",
		"Hugepages usage for a given container and pod.",
		[]string{
			"cgroup_path",
			"size",
			"type",
		}, nil,
	)

	blkioDeviceUsageDesc = prometheus.NewDesc(
		"cgroup_blkio_device_usage",
		"Blkio Device bytes usage for a given container and pod.",
		[]string{
			"cgroup_path",
			"major",
			"minor",
			"operation",
		}, nil,
	)
)

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
	prometheus.DescribeByCollect(c, ch)
}

func updateCPUAcctUsageMetric(ch chan<- prometheus.Metric, path string, metric []cgroups.CPUAcctUsage) {
	for i, acct := range metric {
		ch <- prometheus.MustNewConstMetric(
			cpuAcctUsageDesc,
			prometheus.CounterValue,
			float64(acct.CPU),
			path, strconv.FormatInt(int64(i), 10), "CPU",
		)
		ch <- prometheus.MustNewConstMetric(
			cpuAcctUsageDesc,
			prometheus.CounterValue,
			float64(acct.User),
			path, strconv.FormatInt(int64(i), 10), "User",
		)
		ch <- prometheus.MustNewConstMetric(
			cpuAcctUsageDesc,
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
		memoryMigrateDesc,
		prometheus.GaugeValue,
		float64(migrateValue),
		path,
	)
}

func updateMemoryUsageMetric(ch chan<- prometheus.Metric, path string, metric cgroups.MemoryUsage) {
	ch <- prometheus.MustNewConstMetric(
		memoryUsageDesc,
		prometheus.GaugeValue,
		float64(metric.Bytes),
		path, "Bytes",
	)
	ch <- prometheus.MustNewConstMetric(
		memoryUsageDesc,
		prometheus.GaugeValue,
		float64(metric.MaxBytes),
		path, "MaxBytes",
	)
}

func updateNumaStatMetric(ch chan<- prometheus.Metric, path string, metric cgroups.NumaStat) {
	// TODO: use "reflect" to iterate through the struct fields of NumaStat?

	for key, value := range metric.Total.Nodes {
		ch <- prometheus.MustNewConstMetric(
			numaStatsDesc,
			prometheus.GaugeValue,
			float64(value),
			path, key, "Total",
		)
	}
	for key, value := range metric.File.Nodes {
		ch <- prometheus.MustNewConstMetric(
			numaStatsDesc,
			prometheus.GaugeValue,
			float64(value),
			path, key, "File",
		)
	}
	for key, value := range metric.Anon.Nodes {
		ch <- prometheus.MustNewConstMetric(
			numaStatsDesc,
			prometheus.GaugeValue,
			float64(value),
			path, key, "Anon",
		)
	}
	for key, value := range metric.Unevictable.Nodes {
		ch <- prometheus.MustNewConstMetric(
			numaStatsDesc,
			prometheus.GaugeValue,
			float64(value),
			path, key, "Unevictable",
		)
	}
	for key, value := range metric.HierarchicalTotal.Nodes {
		ch <- prometheus.MustNewConstMetric(
			numaStatsDesc,
			prometheus.GaugeValue,
			float64(value),
			path, key, "HierarchicalTotal",
		)
	}
	for key, value := range metric.HierarchicalFile.Nodes {
		ch <- prometheus.MustNewConstMetric(
			numaStatsDesc,
			prometheus.GaugeValue,
			float64(value),
			path, key, "HierarchicalFile",
		)
	}
	for key, value := range metric.HierarchicalAnon.Nodes {
		ch <- prometheus.MustNewConstMetric(
			numaStatsDesc,
			prometheus.GaugeValue,
			float64(value),
			path, key, "HierarchicalAnon",
		)
	}
	for key, value := range metric.HierarchicalUnevictable.Nodes {
		ch <- prometheus.MustNewConstMetric(
			numaStatsDesc,
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
			hugeTlbUsageDesc,
			prometheus.GaugeValue,
			float64(hugeTlbUsage.Bytes),
			path, hugeTlbUsage.Size, "Bytes",
		)
		ch <- prometheus.MustNewConstMetric(
			hugeTlbUsageDesc,
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
				blkioDeviceUsageDesc,
				prometheus.CounterValue,
				float64(val),
				path, strconv.FormatInt(int64(deviceBytes.Major), 10),
				strconv.FormatInt(int64(deviceBytes.Minor), 10), operation,
			)
		}
	}
}

func walkCgroups() []string {
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

	collectors := []func(string){
		func(path string) {
			defer wg.Done()
			numa, err := cgroups.GetNumaStats(cgroupPath("memory", path))
			if err == nil {
				updateNumaStatMetric(ch, path, numa)
			} else {
				log.Error("failed to collect NUMA stats for %s: %v", path, err)
			}
		},
		func(path string) {
			defer wg.Done()
			memory, err := cgroups.GetMemoryUsage(cgroupPath("memory", path))
			if err == nil {
				updateMemoryUsageMetric(ch, path, memory)
			} else {
				log.Error("failed to collect memory usage stats for %s: %v", path, err)
			}
		},
		func(path string) {
			defer wg.Done()
			migrate, err := cgroups.GetCPUSetMemoryMigrate(cgroupPath("cpuset", path))
			if err == nil {
				updateMemoryMigrateMetric(ch, path, migrate)
			} else {
				log.Error("failed to collect memory migration stats for %s: %v", path, err)
			}
		},
		func(path string) {
			defer wg.Done()
			cpuAcctUsage, err := cgroups.GetCPUAcctStats(cgroupPath("cpuacct", path))
			if err == nil {
				updateCPUAcctUsageMetric(ch, path, cpuAcctUsage)
			} else {
				log.Error("failed to collect CPU accounting stats for %s: %v", path, err)
			}
		},
		func(path string) {
			defer wg.Done()
			hugeTlbUsage, err := cgroups.GetHugetlbUsage(cgroupPath("hugetlb", path))
			if err == nil {
				updateHugeTlbUsageMetric(ch, path, hugeTlbUsage)
			} else {
				log.Error("failed to collect hugetlb stats for %s: %v", path, err)
			}
		},
		func(path string) {
			defer wg.Done()
			blkioDeviceUsage, err := cgroups.GetBlkioThrottleBytes(cgroupPath("blkio", path))
			if err == nil {
				updateBlkioDeviceUsageMetric(ch, path, blkioDeviceUsage)
			} else {
				log.Error("failed to collect blkio stats for %s: %v", path, err)
			}
		},
	}

	for _, path := range walkCgroups() {
		wg.Add(len(collectors))
		for _, fn := range collectors {
			go fn(path)
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
		log.Error("failed register cgroupstats collector: %v", err)
	}
}
