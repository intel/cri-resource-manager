// Copyright 2023 Intel Corporation. All Rights Reserved.
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
)

type PidWatcherPidlistConfig struct {
	Pids []int // list of absolute cgroup directory paths
}

type PidWatcherPidlist struct {
	config      *PidWatcherPidlistConfig
	pidListener PidListener
}

func init() {
	PidWatcherRegister("pidlist", NewPidWatcherPidlist)
}

func NewPidWatcherPidlist() (PidWatcher, error) {
	return &PidWatcherPidlist{}, nil
}

func (w *PidWatcherPidlist) SetConfigJson(configJson string) error {
	config := &PidWatcherPidlistConfig{}
	if err := unmarshal(configJson, config); err != nil {
		return err
	}
	w.config = config
	return nil
}

func (w *PidWatcherPidlist) GetConfigJson() string {
	if w.config == nil {
		return ""
	}
	if configStr, err := json.Marshal(w.config); err == nil {
		return string(configStr)
	}
	return ""
}

func (w *PidWatcherPidlist) SetPidListener(l PidListener) {
	w.pidListener = l
}

func (w *PidWatcherPidlist) Poll() error {
	if w.pidListener == nil {
		log.Warnf("pidwatcher pidlist: poll skips reporting pids %v because nobody is listening", w.config.Pids)
		return nil
	}
	w.pidListener.AddPids(w.config.Pids)
	return nil
}

func (w *PidWatcherPidlist) Start() error {
	if w.pidListener == nil {
		log.Warnf("pidwatcher pidlist: skip reporting pids %v because nobody is listening", w.config.Pids)
		return nil
	}
	w.pidListener.AddPids(w.config.Pids)
	return nil
}

func (w *PidWatcherPidlist) Stop() {
}

func (w *PidWatcherPidlist) Dump([]string) string {
	return fmt.Sprintf("%+v", w)
}
