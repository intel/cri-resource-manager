// Copyright 2020 Intel Corporation. All Rights Reserved.
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

package metrics

import (
	model "github.com/prometheus/client_model/go"
	"path/filepath"

	"github.com/intel/cri-resource-manager/pkg/cgroups"
)

// AvxEvent describes cgroup/container AVX512 instruction usage.
type AvxEvent struct {
	// Updates contains updates to cgroup/container AVX512 instruction usage.
	Updates map[string]bool
}

func (m *Metrics) collectAvxEvents(raw map[string]*model.MetricFamily) *AvxEvent {
	all, ok := raw["all_switch_count_per_cgroup"]
	if !ok {
		return nil
	}
	dump("all context switches", all)

	avx, ok := raw["avx_switch_count_per_cgroup"]
	if !ok {
		return nil
	}
	dump("AVX context switches", avx)

	ratio := map[string]float64{}
	for _, v := range avx.Metric {
		cgroup, err := filepath.Rel(cgroups.V2path, v.Label[0].GetValue())
		if err != nil {
			continue
		}
		ratio[cgroup] = v.Gauge.GetValue()
	}
	for _, v := range all.Metric {
		cgroup, err := filepath.Rel(cgroups.V2path, v.Label[0].GetValue())
		if err != nil {
			continue
		}
		ratio[cgroup] /= v.Gauge.GetValue()
	}

	usage := map[string]bool{}
	for cgroup, use := range ratio {
		active := use >= m.opts.AvxThreshold
		log.Debug(" %s AVX ratio = %f, active?: %v", cgroup, use, active)
		usage["/"+cgroup] = active
	}

	return &AvxEvent{Updates: usage}
}
