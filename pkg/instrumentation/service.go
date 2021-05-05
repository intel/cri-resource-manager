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
	"sync"

	"github.com/intel/cri-resource-manager/pkg/instrumentation/http"
)

// service is the state of our instrumentation services: HTTP endpoint, trace/metrics exporters.
type service struct {
	sync.RWMutex              // we're RW-lockable
	http         *http.Server // HTTP server
	tracing      *tracing     // tracing data exporter
	metrics      *metrics     // metrics data exporter
}

// newService creates an instance of our instrumentation services.
func newService() *service {
	return &service{
		http:    http.NewServer(),
		tracing: &tracing{},
		metrics: &metrics{},
	}
}

// Start starts instrumentation services.
func (s *service) Start() error {
	log.Infof("starting instrumentation services...")

	s.Lock()
	defer s.Unlock()

	err := s.http.Start(opt.HTTPEndpoint)
	if err != nil {
		return instrumentationError("failed to start HTTP server: %v", err)
	}
	err = s.tracing.start(opt.JaegerAgent, opt.JaegerCollector, opt.Sampling)
	if err != nil {
		return instrumentationError("failed to start tracing: %v", err)
	}
	err = s.metrics.start(s.http.GetMux(), opt.ReportPeriod, opt.PrometheusExport)
	if err != nil {
		return instrumentationError("failed to start metrics: %v", err)
	}

	if err := registerGrpcViews(); err != nil {
		s.metrics.stop()
		s.tracing.stop()
		s.http.Stop()
		return err
	}

	return nil
}

// Stop stops instrumentation services.
func (s *service) Stop() {
	s.Lock()
	defer s.Unlock()

	unregisterGrpcViews()
	s.metrics.stop()
	s.tracing.stop()
	s.http.Stop()
}

// reconfigure reconfigures instrumentation services.
func (s *service) reconfigure() error {
	s.Lock()
	defer s.Unlock()

	err := s.http.Reconfigure(opt.HTTPEndpoint)
	if err != nil {
		return instrumentationError("failed to reconfigure HTTP server: %v", err)
	}
	err = s.tracing.reconfigure(opt.JaegerAgent, opt.JaegerCollector, opt.Sampling)
	if err != nil {
		return instrumentationError("failed to reconfigure tracing: %v", err)
	}
	err = s.metrics.reconfigure(s.http.GetMux(), opt.ReportPeriod, opt.PrometheusExport)
	if err != nil {
		return instrumentationError("failed to reconfigure metrics: %v", err)
	}
	return nil
}

// Restart restarts instrumentation services.
func (s *service) Restart() error {
	s.Stop()
	return s.Start()
}

// TracingEnabled returns true if the Jaeger tracing sampler is not disabled.
func (s *service) TracingEnabled() bool {
	s.RLock()
	defer s.RUnlock()

	return float64(opt.Sampling) > 0.0
}
