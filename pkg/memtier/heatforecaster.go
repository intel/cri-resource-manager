// Copyright 2022 Intel Corporation. All Rights Reserved.
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

package memtier

import (
	"fmt"
	"sort"
)

type HeatForecasterConfig struct {
	Name   string
	Config string
}

type HeatForecaster interface {
	SetConfigJson(string) error // Set new configuration.
	GetConfigJson() string      // Get current configuration.
	Forecast(*Heats) (*Heats, error)
	Dump(args []string) string
}

type HeatForecasterCreator func() (HeatForecaster, error)

// heatforecasters is a map of heatforecaster name -> heatforecaster creator
var heatforecasters map[string]HeatForecasterCreator = make(map[string]HeatForecasterCreator, 0)

func HeatForecasterRegister(name string, creator HeatForecasterCreator) {
	heatforecasters[name] = creator
}

func HeatForecasterList() []string {
	keys := make([]string, 0, len(heatforecasters))
	for key := range heatforecasters {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func NewHeatForecaster(name string) (HeatForecaster, error) {
	if creator, ok := heatforecasters[name]; ok {
		return creator()
	}
	return nil, fmt.Errorf("invalid heat forecaster name %q", name)
}
