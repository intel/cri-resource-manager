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
	"contrib.go.opencensus.io/exporter/jaeger"
	"go.opencensus.io/trace"
)

const (
	// jaegerExporter is used in log messages.
	jaegerExporter = "Jaeger trace exporter"
)

// tracing encapsulates the state of our Jaeger exporter.
type tracing struct {
	exporter  *jaeger.Exporter
	agent     string
	collector string
	sampling  Sampling
}

// start starts our Jaeger exporter.
func (t *tracing) start(agent, collector string, sampling Sampling) error {
	if agent == "" && collector == "" {
		log.Info("%s is disabled", jaegerExporter)
		return nil
	}

	log.Info("creating %s...", jaegerExporter)

	cfg := jaeger.Options{
		ServiceName:       ServiceName,
		CollectorEndpoint: collector,
		AgentEndpoint:     agent,

		Process: jaeger.Process{ServiceName: ServiceName},
		OnError: func(err error) { log.Error("jaeger error: %v", err) },
	}

	exp, err := jaeger.NewExporter(cfg)
	if err != nil {
		return instrumentationError("failed to create %s: %v", jaegerExporter, err)
	}

	t.exporter = exp
	t.agent = agent
	t.collector = collector
	t.sampling = sampling

	trace.RegisterExporter(t.exporter)
	trace.ApplyConfig(trace.Config{DefaultSampler: t.sampling.Sampler()})

	return nil
}

// stop stops our Jaeger exporter.
func (t *tracing) stop() {
	if t.exporter == nil {
		return
	}

	log.Info("stopping Jaeger trace exporter...")

	trace.UnregisterExporter(t.exporter)
	*t = tracing{}
}

// reconfigure reconfigures our Jaeger exporter.
func (t *tracing) reconfigure(agent, collector string, sampling Sampling) error {
	log.Info("reconfiguring %s...", jaegerExporter)

	if agent == "" && collector == "" {
		t.stop()
		return nil
	}

	if t.agent != agent || t.collector != collector {
		t.stop()
	}

	if t.exporter != nil {
		t.sampling = sampling
		trace.ApplyConfig(trace.Config{DefaultSampler: t.sampling.Sampler()})
		return nil
	}

	return t.start(agent, collector, sampling)
}
