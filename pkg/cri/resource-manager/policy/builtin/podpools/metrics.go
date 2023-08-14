// Copyright 2020-2021 Intel Corporation. All Rights Reserved.
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

package podpools

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/policy"
	"github.com/intel/cri-resource-manager/pkg/procstats"
	"github.com/intel/cri-resource-manager/pkg/sysfs"
	"github.com/intel/cri-resource-manager/pkg/utils/cpuset"
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics defines the podpools-specific metrics from policy level.
type Metrics struct {
	PoolMetrics map[string]*PoolMetrics
}

// PoolMetrics defines the podpools-specific metrics from pool level.
type PoolMetrics struct {
	DefName        string
	PrettyName     string
	CPUs           cpuset.CPUSet
	CPUIds         []int
	MilliCPUs      string
	Memory         string
	ContainerNames string
	PodNames       string
}

// Prometheus Metric descriptor indices and descriptor table
const (
	cpuUsageDesc = iota
	poolCPUUsageDesc
)

var descriptors = []*prometheus.Desc{
	cpuUsageDesc: prometheus.NewDesc(
		"cpu_usage",
		"CPU usage per logical processor",
		[]string{
			"cpu",
		}, nil,
	),
	poolCPUUsageDesc: prometheus.NewDesc(
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

var cpuTimeStat *procstats.CPUTimeStat

// DescribeMetrics generates policy-specific prometheus metrics data descriptors.
func (p *podpools) DescribeMetrics() []*prometheus.Desc {
	return descriptors
}

// PollMetrics provides policy metrics for monitoring.
func (p *podpools) PollMetrics() policy.Metrics {
	if p.pools == nil || len(p.pools) <= 0 {
		log.Error("Failed to pull metrics.")
		return nil
	}
	policyMetrics := &Metrics{}
	policyMetrics.PoolMetrics = make(map[string]*PoolMetrics, len(p.pools))

	for _, pool := range p.pools {
		policyMetrics.PoolMetrics[pool.PrettyName()] = &PoolMetrics{}
		policyMetrics.PoolMetrics[pool.PrettyName()].DefName = pool.Def.Name
		policyMetrics.PoolMetrics[pool.PrettyName()].PrettyName = pool.PrettyName()
		policyMetrics.PoolMetrics[pool.PrettyName()].CPUs = pool.CPUs
		policyMetrics.PoolMetrics[pool.PrettyName()].CPUIds = pool.CPUs.List()
		policyMetrics.PoolMetrics[pool.PrettyName()].MilliCPUs = strconv.Itoa(pool.CPUs.Size() * 1000)
		policyMetrics.PoolMetrics[pool.PrettyName()].Memory = pool.Mems.String()
		policyMetrics.PoolMetrics[pool.PrettyName()].ContainerNames = ""
		policyMetrics.PoolMetrics[pool.PrettyName()].PodNames = ""
		if len(pool.PodIDs) > 0 {
			podIds := make([]string, 0, len(pool.PodIDs))
			for podId := range pool.PodIDs {
				podIds = append(podIds, podId)
			}
			sort.Sort(sort.StringSlice(podIds))
			for _, podId := range podIds {
				for _, containerId := range pool.PodIDs[podId] {
					if container, ok := p.cch.LookupContainer(containerId); ok {
						containerName := container.PrettyName()
						if policyMetrics.PoolMetrics[pool.PrettyName()].ContainerNames == "" {
							policyMetrics.PoolMetrics[pool.PrettyName()].ContainerNames = containerName
						} else {
							policyMetrics.PoolMetrics[pool.PrettyName()].ContainerNames = fmt.Sprintf("%s,%s", policyMetrics.PoolMetrics[pool.PrettyName()].ContainerNames, containerName)
						}
					}
				}
				if pod, ok := p.cch.LookupPod(podId); ok {
					podName := pod.GetName()
					if policyMetrics.PoolMetrics[pool.PrettyName()].PodNames == "" {
						policyMetrics.PoolMetrics[pool.PrettyName()].PodNames = podName
					} else {
						policyMetrics.PoolMetrics[pool.PrettyName()].PodNames = fmt.Sprintf("%s,%s", policyMetrics.PoolMetrics[pool.PrettyName()].PodNames, podName)
					}
				}
			}
		}
	}
	return policyMetrics
}

// CollectMetrics generates prometheus metrics from cached/polled policy-specific metrics data.
func (p *podpools) CollectMetrics(m policy.Metrics) ([]prometheus.Metric, error) {
	metrics, ok := m.(*Metrics)
	if !ok {
		return nil, fmt.Errorf("Wrong podpools metrics.")
	}
	if cpuTimeStat == nil {
		if initSys, err := sysfs.DiscoverSystem(); err != nil {
			return nil, err
		} else {
			cpuCount := len(initSys.CPUIDs())
			cpuTimeStat = &procstats.CPUTimeStat{
				PrevIdleTime:       make([]uint64, cpuCount),
				PrevTotalTime:      make([]uint64, cpuCount),
				CurIdleTime:        make([]uint64, cpuCount),
				CurTotalTime:       make([]uint64, cpuCount),
				DeltaIdleTime:      make([]uint64, cpuCount),
				DeltaTotalTime:     make([]uint64, cpuCount),
				CPUUsage:           make([]float64, cpuCount),
				IsGetCPUUsageBegin: false,
			}
		}
	}
	err := cpuTimeStat.GetCPUTimeStat()
	if err != nil {
		return nil, err
	}
	cpuMetrics, err := updateCPUUsageMetrics()
	if err != nil {
		return nil, err
	}
	poolCPUMetrics, err := updatePoolCPUUsageMetrics(metrics)
	if err != nil {
		return nil, err
	}
	return append(cpuMetrics, poolCPUMetrics...), nil
}

// updateCPUUsageMetrics collects the CPU usage per logical processor.
func updateCPUUsageMetrics() ([]prometheus.Metric, error) {
	cpuTimeStat.RLock()
	defer cpuTimeStat.RUnlock()
	sys, err := sysfs.DiscoverSystem()
	if err != nil {
		return nil, err
	}
	onlined := sys.CPUSet().Difference(sys.Offlined())
	onlinedUsage := make([]prometheus.Metric, onlined.Size())
	for i, j := range onlined.List() {
		onlinedUsage[i] = prometheus.MustNewConstMetric(
			descriptors[cpuUsageDesc],
			prometheus.GaugeValue,
			cpuTimeStat.CPUUsage[j],
			strconv.Itoa(j),
		)
	}
	return onlinedUsage, nil
}

// updatePoolCPUUsageMetrics collects the CPU usage of pools defined by podpools-policy.
func updatePoolCPUUsageMetrics(ppm *Metrics) ([]prometheus.Metric, error) {
	if ppm == nil {
		return nil, fmt.Errorf("Podpools metrics used to count pool CPU usage is missing.")
	}
	// Sort the pool metrics.
	poolNames := make([]string, 0, len(ppm.PoolMetrics))
	for poolName := range ppm.PoolMetrics {
		poolNames = append(poolNames, poolName)
	}
	sort.Sort(sort.StringSlice(poolNames))

	// Calculate the CPU usage of a pool and send to prometheus.
	poolCPUUsageMetrics := make([]prometheus.Metric, len(poolNames))
	poolCPUUsageList := make(map[string]float64, len(poolNames))
	cpuTimeStat.RLock()
	defer cpuTimeStat.RUnlock()
	for index, poolName := range poolNames {
		poolDeltaIdleTime := uint64(0)
		poolDeltaTotalTime := uint64(0)
		for _, cpuId := range ppm.PoolMetrics[poolName].CPUIds {
			poolDeltaIdleTime += cpuTimeStat.DeltaIdleTime[cpuId]
			poolDeltaTotalTime += cpuTimeStat.DeltaTotalTime[cpuId]
		}
		poolCPUUsageList[poolName] = 0.0
		if poolDeltaTotalTime != 0 {
			sys, err := sysfs.DiscoverSystem()
			if err != nil {
				return nil, err
			}
			poolCPUOnlined := ppm.PoolMetrics[poolName].CPUs.Difference(sys.Offlined())
			poolCPUUsageList[poolName] = (1.0 - float64(poolDeltaIdleTime)/float64(poolDeltaTotalTime)) * 100.0 * float64(len(poolCPUOnlined.List()))
		}
		poolCPUUsageMetrics[index] = prometheus.MustNewConstMetric(
			descriptors[poolCPUUsageDesc],
			prometheus.GaugeValue,
			poolCPUUsageList[poolName],
			PolicyName,
			poolName,
			ppm.PoolMetrics[poolName].DefName,
			ppm.PoolMetrics[poolName].CPUs.String(),
			ppm.PoolMetrics[poolName].Memory,
			ppm.PoolMetrics[poolName].MilliCPUs,
			ppm.PoolMetrics[poolName].PodNames,
			ppm.PoolMetrics[poolName].ContainerNames,
		)
	}
	return poolCPUUsageMetrics, nil
}
