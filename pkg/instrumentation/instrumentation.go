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
	"net/http"
	"strings"
	"time"

	"contrib.go.opencensus.io/exporter/jaeger"
	"contrib.go.opencensus.io/exporter/prometheus"
	"go.opencensus.io/plugin/ocgrpc"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/trace"
	"google.golang.org/grpc"

	logger "github.com/intel/cri-resource-manager/pkg/log"
)

const (
	Service = "CRI-RM"
)

// Our logger instance.
var log logger.Logger = logger.NewLogger("tracing")

// shutdownFn is the type of function we run during shutdown.
type shutdownFn func()

// Our shutdown function.
var shutdown shutdownFn

// IsEnabled returns true if tracing is enabled.
func IsEnabled() bool {
	return opt.Trace != Disabled
}

// Setup sets up instrumentation (tracing, metrics collection, etc.).
func Setup() error {
	var cfg *trace.Config

	if !IsEnabled() {
		return nil
	}

	cfg = &trace.Config{DefaultSampler: opt.Trace.Sampler()}
	trace.ApplyConfig(*cfg)

	if shutdown != nil {
		return nil
	}

	jopt := jaeger.Options{
		ServiceName:       Service,
		CollectorEndpoint: opt.Collector,
		AgentEndpoint:     opt.Agent,
		Process:           jaeger.Process{ServiceName: Service},
		OnError:           func(err error) { log.Error("jaeger: %v", err) },
	}
	je, err := jaeger.NewExporter(jopt)
	if err != nil {
		return traceError("failed to create Jaeger exporter: %v", err)
	}
	trace.RegisterExporter(je)

	if err = view.Register(ocgrpc.DefaultClientViews...); err != nil {
		return traceError("failed to register default gRPC client views: %v", err)
	}
	if err = view.Register(ocgrpc.DefaultServerViews...); err != nil {
		return traceError("failed to register default gRPC server views: %v", err)
	}

	popt := prometheus.Options{
		Namespace: prometheusNamespace(Service),
		OnError:   func(err error) { log.Error("prometheus: %v", err) },
	}
	pe, err := prometheus.NewExporter(popt)
	if err != nil {
		return traceError("failed to create Prometheus exporter: %v", err)
	}
	view.RegisterExporter(pe)

	view.SetReportingPeriod(5 * time.Second)

	go serveMetrics(pe)

	shutdown = func() {
		je.Flush()
	}

	return nil
}

// Finish shuts down instrumentation.
func Finish() {
	if shutdown != nil {
		shutdown()
	}
}

// InjectGrpcClientTrace injects gRPC dial options for instrumentation if necessary.
func InjectGrpcClientTrace(opts ...grpc.DialOption) []grpc.DialOption {
	if !IsEnabled() {
		return opts
	}

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
	if !IsEnabled() {
		return opts
	}

	extra := grpc.StatsHandler(&ocgrpc.ServerHandler{})

	if len(opts) > 0 {
		opts = append(opts, extra)
	} else {
		opts = []grpc.ServerOption{extra}
	}

	return opts
}

// prometheusNamespace mutates a service name into a valid Prometheus namespace.
func prometheusNamespace(service string) string {
	return strings.ReplaceAll(strings.ToLower(service), "-", "_")
}

// serveMetrics runs the Prometheus /metrics endpoint.
func serveMetrics(pe *prometheus.Exporter) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", pe)
	if err := http.ListenAndServe(opt.Metrics, mux); err != nil {
		log.Fatal("failed to run Prometheus /metrics endpoint: %v", err)
	}
}
