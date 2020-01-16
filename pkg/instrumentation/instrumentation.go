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

package instrumentation

import (
	"strings"
	"time"

	"contrib.go.opencensus.io/exporter/jaeger"
	"contrib.go.opencensus.io/exporter/prometheus"
	prom "github.com/prometheus/client_golang/prometheus"
	model "github.com/prometheus/client_model/go"
	"go.opencensus.io/plugin/ocgrpc"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/trace"
	"google.golang.org/grpc"

	logger "github.com/intel/cri-resource-manager/pkg/log"
)

const (
	// ServiceName is our Jaeger and Prometheus service names.
	ServiceName = "CRI-RM"
)

// Our logger instance.
var log logger.Logger = logger.NewLogger("tracing")

// Our Jaeger trace and Prometheus metrics exporters.
var jexport *jaeger.Exporter
var pexport *prometheus.Exporter

// prometheus Gatherers dynamically registered with us.
var dynamicGatherers = &gatherers{gatherers: prom.Gatherers{}}

// ConfigureTracing configures Jaeger-tracing.
func ConfigureTracing(tc TraceConfig) error {
	log.Debug("applying trace configuration %v", tc)
	trace.ApplyConfig(trace.Config{DefaultSampler: tc.Sampler()})
	return nil
}

// TracingEnabled returns true if the tracing sampler is not disabled.
func TracingEnabled() bool {
	return float64(opt.Trace) > 0.0
}

// Start sets up instrumentation (tracing, metrics collection, etc.).
func Start() error {
	log.Info("Starting instrumenation...")
	ConfigureTracing(opt.Trace)

	registerJaegerExporter()
	registerGrpcTraceViews()
	registerPrometheusExporter()

	if err := HTTPStart(); err != nil {
		return traceError("failed to start tracing/metrics HTTP server: %v", err)
	}

	return nil
}

// Stop shuts down instrumentation.
func Stop() {
	unregisterJaegerExporter()
	unregisterGrpcTraceViews()
	unregisterPrometheusExporter()
	HTTPShutdown()
}

// InjectGrpcClientTrace injects gRPC dial options for instrumentation if necessary.
func InjectGrpcClientTrace(opts ...grpc.DialOption) []grpc.DialOption {
	extra := grpc.WithStatsHandler(&ocgrpc.ClientHandler{})

	if len(opts) > 0 {
		opts = append(opts, extra)
	} else {
		opts = []grpc.DialOption{extra}
	}

	return opts
}

// InjectGrpcServerTrace injects gRPC server options for instrumentation if necessary.
func InjectGrpcServerTrace(opts ...grpc.ServerOption) []grpc.ServerOption {
	extra := grpc.StatsHandler(&ocgrpc.ServerHandler{})

	if len(opts) > 0 {
		opts = append(opts, extra)
	} else {
		opts = []grpc.ServerOption{extra}
	}

	return opts
}

// RegisterGatherer registers a new prometheus Gatherer.
func RegisterGatherer(g prom.Gatherer) {
	dynamicGatherers.Add(g)
}

func registerJaegerExporter() error {
	var err error

	if !TracingEnabled() || jexport != nil {
		return nil
	}

	log.Debug("registering Jaeger exporter...")

	log := logger.NewLogger("jaeger/" + ServiceName)
	cfg := jaeger.Options{
		ServiceName:       ServiceName,
		CollectorEndpoint: opt.Collector,
		AgentEndpoint:     opt.Agent,
		Process:           jaeger.Process{ServiceName: ServiceName},
		OnError:           func(err error) { log.Error("%v", err) },
	}
	jexport, err = jaeger.NewExporter(cfg)
	if err != nil {
		return traceError("failed to create Jaeger exporter: %v", err)
	}
	trace.RegisterExporter(jexport)

	return nil
}

func unregisterJaegerExporter() {
	if jexport == nil {
		return
	}

	trace.UnregisterExporter(jexport)
	jexport = nil
}

func registerGrpcTraceViews() error {
	log.Debug("registering gRPC trace views...")
	if err := view.Register(ocgrpc.DefaultClientViews...); err != nil {
		return traceError("failed to register default gRPC client views: %v", err)
	}
	if err := view.Register(ocgrpc.DefaultServerViews...); err != nil {
		return traceError("failed to register default gRPC server views: %v", err)
	}
	return nil
}

func unregisterGrpcTraceViews() {
	view.Unregister(ocgrpc.DefaultClientViews...)
	view.Unregister(ocgrpc.DefaultServerViews...)
}

func registerPrometheusExporter() error {
	var err error

	if !TracingEnabled() || pexport != nil {
		return nil
	}

	log.Debug("registering Prometheus exporter...")

	log := logger.NewLogger("metrics/" + ServiceName)
	cfg := prometheus.Options{
		Namespace: prometheusNamespace(ServiceName),
		Gatherer:  prom.Gatherers{dynamicGatherers, prom.NewRegistry()},
		OnError:   func(err error) { log.Error("%v", err) },
	}
	pexport, err = prometheus.NewExporter(cfg)
	if err != nil {
		return traceError("failed to create Prometheus exporter: %v", err)
	}

	view.RegisterExporter(pexport)
	view.SetReportingPeriod(5 * time.Second)

	GetHTTPMux().Handle("/metrics", pexport)

	return nil
}

func unregisterPrometheusExporter() {
	if pexport == nil {
		return
	}

	view.UnregisterExporter(pexport)
	pexport = nil
}

// mutate service name into a valid Prometheus namespace.
func prometheusNamespace(service string) string {
	return strings.ReplaceAll(strings.ToLower(service), "-", "_")
}

// gatherers is a trivial wrapper around prometheus Gatherers.
type gatherers struct {
	gatherers prom.Gatherers
}

// Gather implements the prometheus.Gatherer interface.
func (g *gatherers) Gather() ([]*model.MetricFamily, error) {
	return g.gatherers.Gather()
}

// Add adds a a new gatherer.
func (g *gatherers) Add(gatherer prom.Gatherer) {
	g.gatherers = append(g.gatherers, gatherer)
}
