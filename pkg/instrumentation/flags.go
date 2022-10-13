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
	"os"
	"strconv"
	"strings"
	"time"

	"go.opencensus.io/trace"

	"github.com/intel/cri-resource-manager/pkg/config"
	"github.com/intel/cri-resource-manager/pkg/utils"
)

// Sampling defines how often trace samples are taken.
type Sampling float64

const (
	// Disabled is the trace configuration for disabling tracing.
	Disabled Sampling = 0.0
	// Production is a trace configuration for production use.
	Production Sampling = 0.1
	// Testing is a trace configuration for testing.
	Testing Sampling = 1.0

	// defaultSampling is the default sampling rate
	defaultSampling = Disabled
	// defaultReportPeriod is the default report period
	defaultReportPeriod = 15 * time.Second
	// defaultJaegerCollector is the default Jaeger collector endpoint.
	defaultJaegerCollector = ""
	// defaultJaegerAgent is the default Jaeger agent endpoint.
	defaultJaegerAgent = ""
	// defaultHTTPEndpoint is the default HTTP endpoint serving Prometheus /metrics.
	defaultHTTPEndpoint = ""
	// defaultPrometheusExport is the default state for Prometheus exporting.
	defaultPrometheusExport = false
)

// options encapsulates our configurable instrumentation parameters.
type options optstruct

type optstruct struct {
	// Sampling is the sampling frequency for traces.
	Sampling Sampling
	// ReportPeriod is the OpenCensus view reporting period.
	ReportPeriod time.Duration
	// jaegerCollector is the URL to the Jaeger HTTP Thrift collector.
	JaegerCollector string
	// jaegerAgent, if set, defines the address of a Jaeger agent to send spans to.
	JaegerAgent string
	// HTTPEndpoint is our HTTP endpoint, used among others to export Prometheus /metrics.
	HTTPEndpoint string
	// PrometheusExport defines whether we export /metrics to/for Prometheus.
	PrometheusExport bool `json:"PrometheusExport"`
}

// UnmarshalJSON is a resetting JSON unmarshaller for options.
func (o *options) UnmarshalJSON(raw []byte) error {
	ostruct := optstruct{}
	if err := json.Unmarshal(raw, &ostruct); err != nil {
		return instrumentationError("failed to unmashal options: %v", err)
	}
	*o = options(ostruct)
	return nil
}

// Our instrumentation options.
var opt = defaultOptions().(*options)

// MarshalJSON is the JSON marshaller for Sampling values.
func (s Sampling) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

// UnmarshalJSON is the JSON unmarshaller for Sampling values.
func (s *Sampling) UnmarshalJSON(raw []byte) error {
	var obj interface{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return instrumentationError("failed to unmarshal Sampling value: %v", err)
	}
	switch v := obj.(type) {
	case string:
		if err := s.Parse(v); err != nil {
			return err
		}
	case float64:
		*s = Sampling(v)
	default:
		return instrumentationError("invalid Sampling value of type %T: %v", obj, obj)
	}
	return nil
}

// Parse parses the given string to a Sampling value.
func (s *Sampling) Parse(value string) error {
	switch strings.ToLower(value) {
	case "disabled":
		*s = Disabled
	case "testing":
		*s = Testing
	case "production":
		*s = Production
	default:
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return instrumentationError("invalid Sampling value '%s': %v", value, err)
		}
		*s = Sampling(f)
	}
	return nil
}

// String returns the Sampling value as a string.
func (s Sampling) String() string {
	switch s {
	case Disabled:
		return "disabled"
	case Production:
		return "production"
	case Testing:
		return "testing"
	}
	return strconv.FormatFloat(float64(s), 'f', -1, 64)
}

// Sampler returns a trace.Sampler corresponding to the Sampling value.
func (s Sampling) Sampler() trace.Sampler {
	if s == Disabled {
		return trace.NeverSample()
	}
	return trace.ProbabilitySampler(float64(s))
}

// parseEnv parses the environment for default values.
func parseEnv(name, defval string, parsefn func(string) error) {
	if envval := os.Getenv(name); envval != "" {
		err := parsefn(envval)
		if err == nil {
			return
		}
		log.Error("invalid environment %s=%q: %v, using default %q", name, envval, err, defval)
	}
	if err := parsefn(defval); err != nil {
		log.Error("invalid default %s=%q: %v", name, defval, err)
	}
}

// defaultOptions returns a new options instance, all initialized to defaults.
func defaultOptions() interface{} {
	o := &options{}
	o.Reset()

	return o
}

const (
	// ConfigDescription describes our configuration fragment.
	ConfigDescription = "Instrumentation for traces and metrics." // XXX TODO
)

func (o *options) Describe() string {
	return ConfigDescription
}

func (o *options) Reset() {
	*o = options{
		JaegerCollector:  defaultJaegerCollector,
		JaegerAgent:      defaultJaegerAgent,
		HTTPEndpoint:     defaultHTTPEndpoint,
		PrometheusExport: defaultPrometheusExport,
		Sampling:         defaultSampling,
		ReportPeriod:     defaultReportPeriod,
	}

	if v := os.Getenv("JAEGER_COLLECTOR"); v != "" {
		o.JaegerCollector = v
	}

	if v := os.Getenv("HTTP_ENDPOINT"); v != "" {
		o.HTTPEndpoint = v
	}

	if v := os.Getenv("PROMETHEUS_EXPORT"); v != "" {
		if enabled, err := utils.ParseEnabled(v); err != nil {
			log.Warn("invalid PROMETHEUS_EXPORT=%s: %v", v, err)
		} else {
			o.PrometheusExport = enabled
		}
	}

	if v := os.Getenv("SAMPLING_FREQUENCY"); v != "" {
		if err := o.Sampling.Parse(v); err != nil {
			log.Warn("invalid SAMPLING_FREQUENCY=%s: %v", v, err)
			o.Sampling = defaultSampling
		}
	}

	if v := os.Getenv("REPORT_PERIOD"); v != "" {
		if d, err := time.ParseDuration(v); err != nil {
			log.Warn("invalid REPORT_PERIOD=%s: %v", err)
		} else {
			o.ReportPeriod = d
		}
	}
}

func (o *options) Validate() error {
	log.Info("instrumentation configuration is now %v", opt)

	if err := svc.reconfigure(); err != nil {
		log.Error("failed to restart instrumentation: %v", err)
	}

	return nil
}

// Register us for for configuration handling.
func init() {
	config.Register("instrumentation", ConfigDescription, opt, defaultOptions)
}
