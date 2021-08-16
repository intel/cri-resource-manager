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

package procstats

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/policy/builtin/podpools"
	logger "github.com/intel/cri-resource-manager/pkg/log"
	"github.com/intel/cri-resource-manager/pkg/metrics"
	"github.com/intel/cri-resource-manager/pkg/sysfs"
	"github.com/prometheus/client_golang/prometheus"
)

// core time statistic is used to calculate the CPU usage.
type coreTimeStat struct {
	sync.RWMutex
	prevIdleTime       []uint64
	prevTotalTime      []uint64
	curIdleTime        []uint64
	curTotalTime       []uint64
	deltaIdleTime      []uint64
	deltaTotalTime     []uint64
	cpuUsage           []float64
	isGetCpuUsageBegin bool
}

// Prometheus Metric descriptor indices and descriptor table
const (
	coreCpuUsageDesc = iota
	poolCpuUsageDesc // count the pool CPU usage for podpools policy
	numDescriptors   // descriptors total
)

var descriptors = [numDescriptors]*prometheus.Desc{
	coreCpuUsageDesc: prometheus.NewDesc(
		"core_cpu_usage",
		"CPU usage for a given core",
		[]string{
			"core_id",
		}, nil,
	),
	poolCpuUsageDesc: prometheus.NewDesc(
		"pool_cpu_usage",
		"CPU usage for a given pool",
		[]string{
			"policy",
			"pretty_name",
			"def_name",
			"CPUs",
			"memory",
			"pool_size",
			"pod_name",
			"container_name",
		}, nil,
	),
}

var (
	// procRoot is the mount point for the proc (v1) filesystem
	procRoot = "/proc"
	procStat = procRoot + "/stat"
	// our logger instance
	log = logger.NewLogger("procstats")
)

var sys, _ = sysfs.DiscoverSystem()
var coreNumber = len(sys.CPUIDs())
var coreCpuTimeStat = &coreTimeStat{
	prevIdleTime:       make([]uint64, coreNumber),
	prevTotalTime:      make([]uint64, coreNumber),
	curIdleTime:        make([]uint64, coreNumber),
	curTotalTime:       make([]uint64, coreNumber),
	deltaIdleTime:      make([]uint64, coreNumber),
	deltaTotalTime:     make([]uint64, coreNumber),
	cpuUsage:           make([]float64, coreNumber),
	isGetCpuUsageBegin: false,
}

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

// updateCoreCpuUsageMetric collect the CPU usage of each core.
func updateCoreCpuUsageMetric(ch chan<- prometheus.Metric) {
	coreCpuTimeStat.RLock()
	defer coreCpuTimeStat.RUnlock()
	for i := 0; i < coreNumber; i++ {
		ch <- prometheus.MustNewConstMetric(
			descriptors[coreCpuUsageDesc],
			prometheus.GaugeValue,
			coreCpuTimeStat.cpuUsage[i],
			strconv.Itoa(i),
		)
	}
}

// updatePoolCpuUsageMetric collect the CPU usage of pools which are defined by podpools-policy.
func updatePoolCpuUsageMetric(ch chan<- prometheus.Metric, ppm *podpools.Metrics) {
	coreCpuTimeStat.RLock()
	defer coreCpuTimeStat.RUnlock()

	// Sort the pool metrics.
	poolNames := make([]string, 0, len(ppm.PoolMetrics))
	for poolName := range ppm.PoolMetrics {
		poolNames = append(poolNames, poolName)
	}
	sort.Sort(sort.StringSlice(poolNames))

	// Calculate the CPU usage of a pool and send to prometheus.
	poolCpuUsageList := make(map[string]float64, len(poolNames))
	for _, poolName := range poolNames {
		cpus := resolvePools(ppm.PoolMetrics[poolName].CPUs)
		cpusPerPoolList := strings.Split(cpus, ",")
		poolDeltaIdleTime := uint64(0)
		poolDeltaTotalTime := uint64(0)
		for _, cpuId := range cpusPerPoolList {
			cpuIdInt, _ := strconv.Atoi(cpuId)
			poolDeltaIdleTime += coreCpuTimeStat.deltaIdleTime[cpuIdInt]
			poolDeltaTotalTime += coreCpuTimeStat.deltaTotalTime[cpuIdInt]
		}
		poolCpuUsageList[poolName] = (1.0 - float64(poolDeltaIdleTime)/float64(poolDeltaTotalTime)) * 100.0 * float64(len(cpusPerPoolList))
		ch <- prometheus.MustNewConstMetric(
			descriptors[poolCpuUsageDesc],
			prometheus.GaugeValue,
			poolCpuUsageList[poolName],
			"podpools",
			poolName,
			ppm.PoolMetrics[poolName].DefName,
			ppm.PoolMetrics[poolName].CPUs,
			ppm.PoolMetrics[poolName].Memory,
			ppm.PoolMetrics[poolName].CPUMiliSize,
			ppm.PoolMetrics[poolName].PodNames,
			ppm.PoolMetrics[poolName].ContainerNames,
		)

	}
}

// Resolve pools' cpuset into single cpus.
func resolvePools(cpuset string) string {
	cpusetArray := strings.Split(cpuset, ",")
	cpus := ""
	for _, cpusetStr := range cpusetArray {
		cpuMember := strings.Split(cpusetStr, "-")
		if cpus != "" {
			cpus = fmt.Sprintf(cpus+",%s", cpuMember[0])
		} else {
			cpus = fmt.Sprintf("%s", cpuMember[0])
		}
		if len(cpuMember) > 1 {
			begin, _ := strconv.Atoi(cpuMember[0])
			end, _ := strconv.Atoi(cpuMember[1])
			for j := begin + 1; j <= end; j++ {
				cpus = fmt.Sprintf(cpus+",%d", j)
			}
		}
	}
	return cpus
}

// Collect implements prometheus.Collector interface
func (c collector) Collect(ch chan<- prometheus.Metric) {
	var wg sync.WaitGroup
	coreCpuTimeStat.getCoreTimeStat()
	collectors := []func(){
		func() {
			defer wg.Done()
			updateCoreCpuUsageMetric(ch)
		},
		func() {
			defer wg.Done()
			metricsStr := metrics.Get()
			policyMetrics := &metrics.PolicyMetrics{}
			err := json.Unmarshal([]byte(metricsStr), policyMetrics)
			if err != nil {
				log.Error("Fail to unmarshal state: %s", err)
				return
			}
			switch policyMetrics.Policy {
			case "podpools":
				poolMetrics := &podpools.Metrics{}
				err := json.Unmarshal(policyMetrics.Data, poolMetrics)
				if err != nil {
					log.Error("Fail to unmarshal state: %s", err)
					return
				}
				updatePoolCpuUsageMetric(ch, poolMetrics)
			default:
				log.Info("Policy %s metrics are not defined.", policyMetrics.Policy)
			}
		},
	}
	wg.Add(len(collectors))
	for _, fn := range collectors {
		go fn()
	}
	// We need to wait so that the response channel doesn't get closed.
	wg.Wait()
}

func init() {
	err := metrics.RegisterCollector("procstats", NewCollector)
	if err != nil {
		log.Error("failed register cgroupstats collector: %v", err)
	}
}

func readProcFileLines(filePath string) ([]string, error) {
	f, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	data := string(f)
	rawLines := strings.Split(data, "\n")
	lines := make([]string, 0)
	for _, rawLine := range rawLines {
		if len(strings.TrimSpace(rawLine)) > 0 {
			lines = append(lines, rawLine)
		}
	}
	return lines, nil
}

func (t *coreTimeStat) getCoreTimeStat() error {
	lines, err := readProcFileLines(procStat)
	/*
		/proc/stat
		cpuid：user，nice, system, idle, iowait, irq, softirq
		cpu  130216 19944 162525 1491240 3784 24749 17773 0 0 0
		cpu0 40321 11452 49784 403099 2615 6076 6748 0 0 0
		cpu1 26585 2425 36639 151166 404 2533 3541 0 0 0
		...
	*/
	if err != nil {
		return err
	}
	t.Lock()
	defer t.Unlock()
	for i := 0; i < coreNumber; i++ {
		index := i + 1
		split := strings.Split(lines[index], " ")
		t.curIdleTime[i], _ = strconv.ParseUint(split[4], 10, 64)
		totalTime := uint64(0)
		for _, s := range split {
			u, _ := strconv.ParseUint(s, 10, 64)
			totalTime += u
		}
		t.curTotalTime[i] = totalTime
		t.cpuUsage[i] = 0.0
		if t.isGetCpuUsageBegin {
			t.deltaIdleTime[i] = t.curIdleTime[i] - t.prevIdleTime[i]
			t.deltaTotalTime[i] = t.curTotalTime[i] - t.prevTotalTime[i]
			t.cpuUsage[i] = (1.0 - float64(t.deltaIdleTime[i])/float64(t.deltaTotalTime[i])) * 100.0
		}
		t.prevIdleTime[i] = t.curIdleTime[i]
		t.prevTotalTime[i] = t.curTotalTime[i]
	}
	t.isGetCpuUsageBegin = true
	return nil
}
