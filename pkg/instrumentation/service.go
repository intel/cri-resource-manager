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
	"net/http"
	"sync"

	"contrib.go.opencensus.io/exporter/jaeger"
	"contrib.go.opencensus.io/exporter/prometheus"
	"go.opencensus.io/trace"
)

// Service abstracts our internal state for instrumentation (tracing, metrics, etc.)
type Service struct {
	sync.RWMutex
	*http.ServeMux                      // external HTTP request multiplexer
	reqmux         *http.ServeMux       // internal HTTP request multiplexer
	server         *http.Server         // HTTP server used to export various pieces of data
	jexport        *jaeger.Exporter     // exporter for tracing information
	pexport        *prometheus.Exporter // exporter for collected metrics
	running        bool                 // whether our HTTP server is up and running
}

// createService creates an instrumentation service instance.
func createService() *Service {
	s := &Service{ServeMux: http.NewServeMux()}

	if err := s.createJaegerExporter(); err != nil {
		log.Error("failed to create instrumentation service: %v", err)
	}
	if err := s.createPrometheusExporter(); err != nil {
		log.Error("failed to create instrumentation service: %v", err)
	}

	return s
}

// Start starts the instrumentation service.
func (s *Service) Start() error {
	s.Lock()
	defer s.Unlock()
	return s.start()
}

// Stop stops the instrumentation service.
func (s *Service) Stop() {
	s.Lock()
	defer s.Unlock()
	s.stop()
}

// Restart restarts the instrumentation service.
func (s *Service) Restart() error {
	s.Lock()
	defer s.Unlock()
	s.stop()
	return s.start()
}

// ConfigureTracing configures sampling for Jaeger tracing.
func (s *Service) ConfigureTracing(c TraceConfig) {
	s.RLock()
	defer s.RUnlock()
	s.configureTracing(c)
}

// TracingEnabled returns true if the Jaeger tracing sampler is not disabled.
func (s *Service) TracingEnabled() bool {
	s.RLock()
	defer s.RUnlock()

	return float64(opt.Trace) > 0.0
}

// start starts the instrumentation service.
func (s *Service) start() error {
	if s.running {
		return nil
	}

	log.Info("starting instrumentation service...")

	s.createHTTP()
	s.startJaegerExporter()
	s.startPrometheusExporter()

	if err := s.registerGrpcViews(); err != nil {
		s.stopJaegerExporter()
		s.stopPrometheusExporter()
		s.closeHTTP()
		return err
	}

	s.startHTTP()
	s.configureTracing(opt.Trace)
	s.running = true

	return nil
}

// stop stops the instrumentation service.
func (s *Service) stop() error {
	if !s.running {
		return nil
	}

	s.unregisterGrpcViews()
	s.stopJaegerExporter()
	s.stopPrometheusExporter()
	err := s.closeHTTP()
	s.running = false

	return err
}

// configureTracing configures sampling for Jaeger tracing.
func (s *Service) configureTracing(c TraceConfig) {
	log.Info("applying trace configuration '%s'...", c)
	trace.ApplyConfig(trace.Config{DefaultSampler: c.Sampler()})
}
