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
)

const (
	// PrometheusMetricsPath is the URL path for exposing metrics to Prometheus.
	PrometheusMetricsPath = "/metrics"
)

// dynamically registered prometheus gatherers
var dynamicGatherers = &gatherers{gatherers: pclient.Gatherers{}}

// createPrometheusExporter creates a metrics exporter for Prometheus.
func (s *Service) createPrometheusExporter() error {
	var err error

	log.Debug("creating Prometheus exporter...")

	cfg := prometheus.Options{
		Namespace: prometheusNamespace(ServiceName),
		Gatherer:  pclient.Gatherers{dynamicGatherers},
		OnError:   func(err error) { log.Error("%v", err) },
	}

	if s.pexport, err = prometheus.NewExporter(cfg); err != nil {
		return instrumentationError("failed to create Prometheus exporter: %v", err)
	}

	return nil
}

// startPrometheusExporter registers and starts the Prometheus exporter.
func (s *Service) startPrometheusExporter() {
	if s.pexport != nil && opt.Trace != Disabled {
		s.reqmux.Handle(PrometheusMetricsPath, s.pexport)
		view.RegisterExporter(s.pexport)
		view.SetReportingPeriod(5 * time.Second) // XXX TODO, make this configurable ?
	}
}

// stopPrometheusExporter 'stops' the Prometheus exporter by unregistering it.
func (s *Service) stopPrometheusExporter() {
	if s.pexport != nil {
		view.UnregisterExporter(s.pexport)
	}
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
