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
)

type HeatForecasterChainConfig struct {
	Forecasters []HeatForecasterConfig
}

type HeatForecasterChain struct {
	config      *HeatForecasterChainConfig
	forecasters []HeatForecaster
}

func init() {
	HeatForecasterRegister("chain", NewHeatForecasterChain)
}

func NewHeatForecasterChain() (HeatForecaster, error) {
	return &HeatForecasterChain{}, nil
}

func (hf *HeatForecasterChain) SetConfigJson(configJson string) error {
	config := &HeatForecasterChainConfig{}
	if err := unmarshal(configJson, config); err != nil {
		return err
	}
	return hf.SetConfig(config)
}

func (hf *HeatForecasterChain) SetConfig(config *HeatForecasterChainConfig) error {
	hf.forecasters = []HeatForecaster{}
	for _, conf := range config.Forecasters {
		fc, err := NewHeatForecaster(conf.Name)
		if err != nil {
			return err
		}
		err = fc.SetConfigJson(conf.Config)
		if err != nil {
			return err
		}
		hf.forecasters = append(hf.forecasters, fc)
	}
	hf.config = config
	return nil
}

func (hf *HeatForecasterChain) GetConfigJson() string {
	configString, err := json.Marshal(hf.config)
	if err != nil {
		return ""
	}
	return string(configString)
}

func (hf *HeatForecasterChain) Forecast(heats *Heats) (*Heats, error) {
	last_non_nil_heats := heats
	for _, fc := range hf.forecasters {
		forecasted_heats, _ := fc.Forecast(last_non_nil_heats)
		if forecasted_heats != nil {
			last_non_nil_heats = forecasted_heats
		}
	}
	return last_non_nil_heats, nil
}

func (hf *HeatForecasterChain) Dump(args []string) string {
	return "HeatForecasterChain{config=" + hf.GetConfigJson() + "}"
}
