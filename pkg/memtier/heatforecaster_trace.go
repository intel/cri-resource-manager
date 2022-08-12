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
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type HeatForecasterTraceConfig struct {
	File         string
	HideHeatZero bool
}

type HeatForecasterTrace struct {
	config *HeatForecasterTraceConfig
}

func init() {
	HeatForecasterRegister("trace", NewHeatForecasterTrace)
}

func NewHeatForecasterTrace() (HeatForecaster, error) {
	return &HeatForecasterTrace{}, nil
}

func (hf *HeatForecasterTrace) SetConfigJson(configJson string) error {
	config := &HeatForecasterTraceConfig{}
	if err := unmarshal(configJson, config); err != nil {
		return err
	}
	return hf.SetConfig(config)
}

func (hf *HeatForecasterTrace) SetConfig(config *HeatForecasterTraceConfig) error {
	if config.File == "" {
		return fmt.Errorf("invalid trace heatforecaster configuration: 'file' missing")
	}
	hf.config = config
	return nil
}

func (hf *HeatForecasterTrace) GetConfigJson() string {
	configString, err := json.Marshal(hf.config)
	if err != nil {
		return ""
	}
	return string(configString)
}

func (hf *HeatForecasterTrace) Forecast(heats *Heats) (*Heats, error) {
	if heats == nil {
		return nil, nil
	}
	f, err := os.OpenFile(hf.config.File, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	f.WriteString(fmt.Sprintf("table: heat forecast %d pid addr size heat created updated\n", time.Now().UnixNano()))
	for pid, heatranges := range *heats {
		for _, hr := range *heatranges {
			if hf.config.HideHeatZero && hr.heat < 0.000000001 {
				continue
			}
			f.WriteString(fmt.Sprintf("%d %x %d %.9f %d %d\n",
				pid, hr.addr, hr.length*constUPagesize, hr.heat, hr.created, hr.updated))
		}
	}
	return nil, nil
}

func (hf *HeatForecasterTrace) Dump(args []string) string {
	return "HeatForecasterTrace{config=" + hf.GetConfigJson() + "}"
}
