// Copyright 2022 Intel Corporation. All Rights Reserved.
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

package dyp

import (
	"sort"
	"strconv"
	"strings"

	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/policy"
	"github.com/intel/cri-resource-manager/pkg/utils/cpuset"
	"github.com/prometheus/client_golang/prometheus"
)

// Prometheus Metric descriptor indices and descriptor table
const (
	dynamicPoolsDesc = iota
)

var descriptors = []*prometheus.Desc{
	dynamicPoolsDesc: prometheus.NewDesc(
		"DynamicPools",
		"CPUs",
		[]string{
			"dynamicPool_type",
			"cpu_class",
			"dynamicPool",
			"cpus",
			"mems",
			"containers",
			"tot_req_millicpu",
			"tot_limit_millicpu",
		}, nil,
	),
}

// Metrics defines the dynamicPools-specific metrics from policy level.
type Metrics struct {
	DynamicPools []*DynamicPoolMetrics
}

// DynamicPoolMetrics define metrics of a dynamicPool instance.
type DynamicPoolMetrics struct {
	// dynamicPool type metrics
	DefName  string
	CpuClass string
	// DynamicPool instance metrics
	PrettyName              string
	Cpus                    cpuset.CPUSet
	Mems                    string
	ContainerNames          string
	ContainerReqMilliCpus   int
	ContainerLimitMilliCpus int
}

// DescribeMetrics generates policy-specific prometheus metrics data
// descriptors.
func (p *dynamicPools) DescribeMetrics() []*prometheus.Desc {
	return descriptors
}

// PollMetrics provides policy metrics for monitoring.
func (p *dynamicPools) PollMetrics() policy.Metrics {
	policyMetrics := &Metrics{}
	policyMetrics.DynamicPools = make([]*DynamicPoolMetrics, len(p.dynamicPools))
	for index, dp := range p.dynamicPools {
		dm := &DynamicPoolMetrics{}
		policyMetrics.DynamicPools[index] = dm
		dm.DefName = dp.Def.Name
		dm.CpuClass = dp.Def.CpuClass
		dm.PrettyName = dp.PrettyName()
		dm.Cpus = dp.Cpus
		dm.Mems = dp.Mems.String()
		cNames := []string{}
		// Get container names, total requested milliCPUs and total limit milliCPUs.
		for _, containerIDs := range dp.PodIDs {
			for _, containerID := range containerIDs {
				if c, ok := p.cch.LookupContainer(containerID); ok {
					cNames = append(cNames, c.PrettyName())
					dm.ContainerReqMilliCpus += p.containerRequestedMilliCpus(containerID)
					dm.ContainerLimitMilliCpus += p.containerLimitedMilliCpus(containerID)
				}
			}
		}
		sort.Strings(cNames)
		dm.ContainerNames = strings.Join(cNames, ",")
	}

	return policyMetrics
}

// CollectMetrics generates prometheus metrics from cached/polled
// policy-specific metrics data.
func (p *dynamicPools) CollectMetrics(m policy.Metrics) ([]prometheus.Metric, error) {
	metrics, ok := m.(*Metrics)
	if !ok {
		return nil, dynamicPoolsError("type mismatch in dynamicPools metrics")
	}
	promMetrics := make([]prometheus.Metric, len(metrics.DynamicPools))
	for index, dm := range metrics.DynamicPools {
		promMetrics[index] = prometheus.MustNewConstMetric(
			descriptors[dynamicPoolsDesc],
			prometheus.GaugeValue,
			float64(dm.Cpus.Size()),
			dm.DefName,
			dm.CpuClass,
			dm.PrettyName,
			dm.Cpus.String(),
			dm.Mems,
			dm.ContainerNames,
			strconv.Itoa(dm.ContainerReqMilliCpus),
			strconv.Itoa(dm.ContainerLimitMilliCpus))
	}
	return promMetrics, nil
}
