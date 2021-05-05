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
)

// Gather is our prometheus.Gatherer interface for proxying metrics.
func (m *Metrics) Gather() ([]*model.MetricFamily, error) {
	m.Lock()
	pend := m.pend
	m.Unlock()

	if pend == nil {
		log.Debugf("no data to proxy to prometheus...")
	} else {
		log.Debugf("proxying data to prometheus...")
	}

	return pend, nil
}
