// Copyright 2019-2020 Intel Corporation. All Rights Reserved.
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

package instrumentation

import (
	"strings"
	"sync"
	"time"

	"contrib.go.opencensus.io/exporter/prometheus"
	pclient "github.com/prometheus/client_golang/prometheus"
	model "github.com/prometheus/client_model/go"
	"go.opencensus.io/stats/view"

	"github.com/intel/cri-resource-manager/pkg/instrumentation/http"
)

const (
	// PrometheusMetricsPath is the URL path for exposing metrics to Prometheus.
	PrometheusMetricsPath = "/metrics"
	// prometheusExporter is used in log messages.
	prometheusExporter = "Prometheus metrics exporter"
)

// metrics encapsulates the state of our Prometheus exporter.
type metrics struct {
	exporter *prometheus.Exporter
	mux      *http.ServeMux
	period   time.Duration
}

// start starts our Prometheus exporter.
func (m *metrics) start(mux *http.ServeMux, period time.Duration, enable bool) error {
	if !enable {
		log.Infof("%s is disabled", prometheusExporter)
		return nil
	}

	log.Infof("starting %s...", prometheusExporter)

	cfg := prometheus.Options{
		Namespace: prometheusNamespace(ServiceName),
		Gatherer:  pclient.Gatherers{dynamicGatherers},
		OnError:   func(err error) { log.Errorf("prometheus error: %v", err) },
	}

	exp, err := prometheus.NewExporter(cfg)
	if err != nil {
		return instrumentationError("failed to create %s: %v", prometheusExporter, err)
	}

	m.exporter = exp
	m.mux = mux
	m.period = period

	m.mux.Handle(PrometheusMetricsPath, m.exporter)
	view.RegisterExporter(m.exporter)
	view.SetReportingPeriod(m.period)

	return nil
}

// stop stops our Prometheus exporter.
func (m *metrics) stop() {
	if m.exporter == nil {
		return
	}

	log.Infof("stopping %s...", prometheusExporter)

	view.UnregisterExporter(m.exporter)
	m.mux.Unregister(PrometheusMetricsPath)

	*m = metrics{}
}

// reconfigure reconfigures our Prometheus exporter.
func (m *metrics) reconfigure(mux *http.ServeMux, period time.Duration, enable bool) error {
	log.Infof("reconfiguring %s...", prometheusExporter)

	if !enable {
		m.stop()
		return nil
	}

	if m.exporter != nil {
		m.period = period
		view.SetReportingPeriod(m.period)
		return nil
	}

	return m.start(mux, period, enable)
}

// mutate service name into a valid Prometheus namespace name.
func prometheusNamespace(service string) string {
	return strings.ReplaceAll(strings.ToLower(service), "-", "_")
}

// gatherers is a trivial wrapper around prometheus Gatherers.
type gatherers struct {
	sync.RWMutex
	gatherers pclient.Gatherers
}

// Our dynamically registered Prometheus gatherers.
var dynamicGatherers = &gatherers{gatherers: pclient.Gatherers{}}

// Register registers a new gatherer.
func (g *gatherers) Register(gatherer pclient.Gatherer) {
	g.Lock()
	defer g.Unlock()
	g.gatherers = append(g.gatherers, gatherer)
}

// Gather implements the pclient.Gatherer interface.
func (g *gatherers) Gather() ([]*model.MetricFamily, error) {
	g.RLock()
	defer g.RUnlock()
	return g.gatherers.Gather()
}

// RegisterGatherer registers a new prometheus Gatherer.
func RegisterGatherer(g pclient.Gatherer) {
	dynamicGatherers.Register(g)
}
