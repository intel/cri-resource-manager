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

package resmgr

import (
	"bytes"
	"strings"

	model "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"

	"github.com/intel/cri-resource-manager/pkg/avx"
	"github.com/intel/cri-resource-manager/pkg/cgroups"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	"github.com/intel/cri-resource-manager/pkg/instrumentation"
	"github.com/intel/cri-resource-manager/pkg/metrics"
	// pull in all metrics
	_ "github.com/intel/cri-resource-manager/pkg/metrics/register"
)

// avx512Event indicates a change in a containers usage of AVX512 instructions.
type avx512Event struct {
	container cache.Container
	active    bool
}

// setupMetricsCollection activates metrics data collection and processing.
func (m *resmgr) setupMetricsCollection() error {
	g, err := metrics.NewMetricGatherer()
	if err != nil {
		return err
	}
	m.gatherer = g

	instrumentation.RegisterGatherer(m)

	return nil
}

// Gather is our prometheus.Gatherer interface for proxying gathered metrics to Prometheus.
func (m *resmgr) Gather() ([]*model.MetricFamily, error) {
	return m.gathered, nil
}

// gatherMetrics polls metrics and caches them for proxying to prometheus
func (m *resmgr) gatherMetrics() []*model.MetricFamily {
	families, err := m.gatherer.Gather()
	if err != nil {
		elog.Error("failed to gather metrics: %v", err)
	}
	return families
}

// processMetrics processes the pending/gathered metrics data
func (m *resmgr) processMetrics(families []*model.MetricFamily) {
	for _, f := range families {
		m.processMetricFamily(f)
	}
}

// processMetricFamily processes the given metrics event.
func (m *resmgr) processMetricFamily(mf *model.MetricFamily) {
	elog.Debug("got metrics event %s...", *mf.Name)

	if elog.DebugEnabled() {
		buf := &bytes.Buffer{}
		if _, err := expfmt.MetricFamilyToText(buf, mf); err == nil {
			elog.DebugBlock("  <metric event> ", "%s", strings.TrimSpace(buf.String()))
		}
	}

	switch *mf.Name {
	case avx.AVXSwitchCountName:
		if *mf.Type != model.MetricType_GAUGE {
			elog.Warn("unexpected %s type: %v, expected %v",
				avx.AVXSwitchCountName, *mf.Type, model.MetricType_GAUGE)
			return
		}
		for _, metric := range mf.Metric {
			if len(metric.Label) < 1 {
				continue
			}
			if metric.Label[0].GetName() != "cgroup" {
				elog.Warn("expected cgroup gauge label not found")
				continue
			}
			cgroup := strings.TrimPrefix(metric.Label[0].GetValue(), cgroups.V2path)
			value := metric.Gauge.GetValue()

			elog.Info("%s %s: %f", *mf.Name, cgroup, value)
			if c, ok := m.resolveCgroupPath(cgroup); ok {
				elog.Info("  => container %s...", c.PrettyName())
				m.SendEvent(&avx512Event{container: c, active: true})
			}
		}

	case avx.AllSwitchCountName:
		elog.Debug("got metric event %s", *mf.Name)

	case avx.LastCPUName:
		elog.Debug("got metric event %s", *mf.Name)

	default:
		elog.Warn("ignoring metric event %s...", *mf.Name)
	}
}

// resolveCgroupPath resolves a cgroup path to a container.
func (m *resmgr) resolveCgroupPath(path string) (cache.Container, bool) {
	m.Lock()
	defer m.Unlock()
	return m.cache.LookupContainerByCgroup(path)
}
