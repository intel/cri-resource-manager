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

package policycollector

import (
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/policy"
	"github.com/intel/cri-resource-manager/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type PolicyCollector struct {
	policy policy.Policy
}

func (c *PolicyCollector) SetPolicy(policy policy.Policy) {
	c.policy = policy
}

// HasPolicySpecificMetrics judges whether the policy defines the policy-specific metrics
func (c *PolicyCollector) HasPolicySpecificMetrics() bool {
	if c.policy.DescribeMetrics() == nil {
		return false
	}
	return true
}

// Describe implements prometheus.Collector interface
func (c *PolicyCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range c.policy.DescribeMetrics() {
		ch <- d
	}
}

// Collect implements prometheus.Collector interface
func (c *PolicyCollector) Collect(ch chan<- prometheus.Metric) {
	prometheusMetrics, err := c.policy.CollectMetrics(c.policy.PollMetrics())
	if err != nil {
		return
	}
	for _, m := range prometheusMetrics {
		ch <- m
	}
}

// RegisterPolicyMetricsCollector registers policy-specific collector
func (c *PolicyCollector) RegisterPolicyMetricsCollector() error {
	return metrics.RegisterCollector("policyMetrics", func() (prometheus.Collector, error) {
		return c, nil
	})
}
