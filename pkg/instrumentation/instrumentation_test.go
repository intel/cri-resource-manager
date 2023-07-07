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
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestSamplingIdempotency(t *testing.T) {
	tcases := []Sampling{
		Disabled,
		Testing,
		Production,
		0.2, 0.25, 0.5, 0.75, 0.8,
	}
	for _, tc := range tcases {
		var chk Sampling
		if err := chk.Parse(tc.String()); err != nil {
			t.Errorf("failed to parse Sampling.String() %q: %v", tc, err)
		}
		if chk != tc {
			t.Errorf("expected sampling value for %q: %v, got: %v", tc, tc, chk)
		}
	}
}

func TestPrometheusConfiguration(t *testing.T) {
	log.EnableDebug(true)

	if opt.HTTPEndpoint == "" {
		opt.HTTPEndpoint = ":0"
	}

	s := newService()
	s.Start()

	address := s.http.GetAddress()
	if strings.HasSuffix(opt.HTTPEndpoint, ":0") {
		opt.HTTPEndpoint = address
	}

	checkPrometheus(t, address, !opt.PrometheusExport)

	opt.PrometheusExport = !opt.PrometheusExport
	s.reconfigure()
	checkPrometheus(t, address, !opt.PrometheusExport)

	opt.PrometheusExport = !opt.PrometheusExport
	s.reconfigure()
	checkPrometheus(t, address, !opt.PrometheusExport)

	opt.PrometheusExport = !opt.PrometheusExport
	s.reconfigure()
	checkPrometheus(t, address, !opt.PrometheusExport)

	s.http.Shutdown(true)
	s.Stop()
}

func checkPrometheus(t *testing.T, server string, shouldFail bool) {
	rpl, err := http.Get("http://" + server + "/metrics")

	switch shouldFail {
	case false:
		if err != nil {
			t.Errorf("Prometheus HTTP GET failed: %v", err)
			return
		}

		if rpl.StatusCode != 200 {
			t.Errorf("Prometheus HTTP GET failed: %s", rpl.Status)
			return
		}

		_, err = io.ReadAll(rpl.Body)
		rpl.Body.Close()
		if err != nil {
			t.Errorf("failed to read Prometheus response: %v", err)
		}
		return

	case true:
		if err == nil && rpl.StatusCode == 200 {
			t.Errorf("Prometheus HTTP GET should have failed, but it didn't.")
			return
		}
	}
}
