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
	"encoding/json"
	"github.com/intel/cri-resource-manager/pkg/config"
	"go.opencensus.io/trace"
	"os"
	"strconv"
	"strings"
)

// TraceConfig represents a pre-defined instrumentation configuration.
type TraceConfig float64

const (
	// Disabled is the trace configuration for disabling tracing.
	Disabled TraceConfig = 0.0
	// Production is a trace configuration for production use.
	Production TraceConfig = 0.1
	// Testing is a trace configuration for testing.
	Testing TraceConfig = 1.0
	// defaultCollector is the default Jaeger collector endpoint.
	defaultCollector = "http://localhost:14268/api/traces"
	// defaultAgent is the default Jeager agent endpoint.
	defaultAgent = "localhost:6831"
	// defaultMetrics is the default Prometheus /metrics endpoint.
	defaultMetrics = ":8888"
)

// options encapsulates our configurable instrumentation parameters.
type options struct {
	// Trace is the tracing configuration.
	Trace TraceConfig
	// Collector is the Jaeger collector endpoint.
	Collector string
	// Agent is the Jaeger agent endpoint.
	Agent string
	// Metrics is the Prometheus metrics exporter endpoint.
	Metrics string
}

// Our instrumentation options.
var opt = defaultOptions().(*options)

// MarshalJSON is the JSON marshaller for TraceConfig values.
func (tc TraceConfig) MarshalJSON() ([]byte, error) {
	switch {
	case tc <= 0.005:
		return json.Marshal("disabled")
	case tc <= 0.1:
		return json.Marshal("production")
	case tc >= 0.95:
		return json.Marshal("testing")
	}
	return json.Marshal(tc)
}

// UnmarshalJSON is the JSON unmarshaller for TraceConfig values.
func (tc *TraceConfig) UnmarshalJSON(raw []byte) error {
	var obj interface{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return instrumentationError("failed to unmarshal TraceConfig value:%v", err)
	}
	switch obj.(type) {
	case string:
		switch strings.ToLower(obj.(string)) {
		case "disabled":
			*tc = Disabled
		case "testing":
			*tc = Testing
		case "production":
			*tc = Production
		default:
			return instrumentationError("invalid TraceConfig value '%s'", obj.(string))
		}
	case float64:
		*tc = obj.(TraceConfig)
	default:
		return instrumentationError("invalid TraceConfig value of type %T: %v", obj, obj)
	}
	return nil
}

// Sampler returns a trace.Sampler corresponding to the TraceConfig value.
func (tc TraceConfig) String() string {
	switch {
	case tc <= 0.005:
		return "disabled"
	case tc <= 0.1:
		return "production"
	case tc >= 0.95:
		return "testing"
	}
	return strconv.FormatFloat(float64(tc), 'f', -1, 64)
}

// Sampler returns a trace.Sampler corresponding to the TraceConfig value.
func (tc TraceConfig) Sampler() trace.Sampler {
	switch {
	case tc >= 0.95:
		return trace.AlwaysSample()
	case tc <= 0.005:
		return trace.NeverSample()
	default:
		return trace.ProbabilitySampler(float64(tc))
	}
}

// defaultOptions returns a new options instance, all initialized to defaults.
func defaultOptions() interface{} {
	collector := os.Getenv("JAEGER_COLLECTOR")
	agent := os.Getenv("JAEGER_AGENT")
	metrics := os.Getenv("PROMETHEUS_ENDPOINT")

	if collector == "" {
		collector = defaultCollector
	}
	if agent == "" {
		agent = defaultAgent
	}
	if metrics == "" {
		metrics = defaultMetrics
	}

	return &options{
		Trace:     Disabled,
		Collector: collector,
		Agent:     agent,
		Metrics:   metrics,
	}
}

// configNotify is our configuration udpate notification handler.
func configNotify(event config.Event, source config.Source) error {
	log.Info("instrumentation configuration is now %v", opt)

	log.Info("restarting...")
	if err := Restart(); err != nil {
		log.Error("failed to restart instrumentation: %v", err)
	}

	return nil
}

// Register us for for configuration handling.
func init() {
	config.Register("instrumentation", "Instrumentation for traces and metrics.",
		opt, defaultOptions, config.WithNotify(configNotify))
}
