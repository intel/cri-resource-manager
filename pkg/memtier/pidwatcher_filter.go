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
	"path/filepath"
	"regexp"
)

type PidWatcherFilterConfig struct {
	Source  PidWatcherConfig
	Filters []*PidFilterConfig
}

type PidFilterConfig struct {
	Exclude               bool
	ProcExeRegexp         string
	compiledProcExeRegexp *regexp.Regexp
}

type PidWatcherFilter struct {
	config      *PidWatcherFilterConfig
	source      PidWatcher
	pidListener PidListener
}

type FilteringPidListener struct {
	w         *PidWatcherFilter
	addedPids map[int]setMemberType
}

func init() {
	PidWatcherRegister("filter", NewPidWatcherFilter)
}

func NewPidWatcherFilter() (PidWatcher, error) {
	return &PidWatcherFilter{}, nil
}

func (w *PidWatcherFilter) SetConfigJson(configJson string) error {
	config := &PidWatcherFilterConfig{}
	if err := unmarshal(configJson, config); err != nil {
		return err
	}
	newSource, err := NewPidWatcher(config.Source.Name)
	if err != nil {
		return fmt.Errorf("pidwatcher filter failed to create source: %w", err)
	}
	if err = newSource.SetConfigJson(config.Source.Config); err != nil {
		return fmt.Errorf("configuring pidwatcher filter's source pidwatcher %q failed: %w", config.Source.Name, err)
	}
	// Validate filters.
	for _, fc := range config.Filters {
		if fc.ProcExeRegexp != "" {
			re, err := regexp.Compile(fc.ProcExeRegexp)
			if err != nil {
				return fmt.Errorf("pidwatcher filter: invalid ProcExeRegexp: %q: %w", fc.ProcExeRegexp, err)
			}
			fc.compiledProcExeRegexp = re
		}
	}
	w.source = newSource
	w.config = config
	f := &FilteringPidListener{
		addedPids: map[int]setMemberType{},
		w:         w,
	}
	if w.source != nil {
		w.source.SetPidListener(f)
	}
	return nil
}

func (w *PidWatcherFilter) GetConfigJson() string {
	if w.config == nil {
		return ""
	}
	if configStr, err := json.Marshal(w.config); err == nil {
		return string(configStr)
	}
	return ""
}

func (w *PidWatcherFilter) SetPidListener(l PidListener) {
	w.pidListener = l
}

func (w *PidWatcherFilter) Poll() error {
	if w.source == nil {
		return fmt.Errorf("pidwatcher filter: poll: missing pid source")
	}
	return w.source.Poll()
}

func (w *PidWatcherFilter) Start() error {
	if w.source == nil {
		return fmt.Errorf("pidwatcher filter: start: missing pid source")
	}
	return w.source.Start()
}

func (w *PidWatcherFilter) Stop() {
	if w.source == nil {
		return
	}
	w.source.Stop()
}

func (w *PidWatcherFilter) Dump([]string) string {
	return fmt.Sprintf("%+v", w)
}

func (f *FilteringPidListener) AddPids(pids []int) {
	passedPids := []int{}
	for _, pid := range pids {
		for _, fc := range f.w.config.Filters {
			if fc.compiledProcExeRegexp != nil {
				exeFilepath, err := filepath.EvalSymlinks(fmt.Sprintf("/proc/%d/exe", pid))
				if err != nil {
					// pid does not exist anymore, never mind about the rest of the filters
					break
				}
				matched := fc.compiledProcExeRegexp.MatchString(exeFilepath)
				if (matched && !fc.Exclude) || (!matched && fc.Exclude) {
					passedPids = append(passedPids, pid)
				}
			}
		}
	}
	for _, pid := range passedPids {
		f.addedPids[pid] = setMember
	}
	if f.w.pidListener != nil {
		f.w.pidListener.AddPids(passedPids)
	} else {
		log.Warnf("pidwatcher filter: ignoring new pids %v because nobody is listening", passedPids)
	}
}

func (f *FilteringPidListener) RemovePids(pids []int) {
	passedPids := []int{}
	for _, pid := range pids {
		if _, ok := f.addedPids[pid]; ok {
			passedPids = append(passedPids, pid)
			delete(f.addedPids, pid)
		}
	}
	if f.w.pidListener != nil {
		f.w.pidListener.RemovePids(passedPids)
	} else {
		log.Warnf("pidwatcher filter: ignoring disappeared pids %v because nobody is listening", passedPids)
	}
}
