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
	"strings"
)

type TrackerMultiConfig struct {
	Trackers []TrackerConfig
}

type TrackerMulti struct {
	config   *TrackerMultiConfig
	trackers []Tracker
}

func init() {
	TrackerRegister("multi", NewTrackerMulti)
}

func NewTrackerMulti() (Tracker, error) {
	return &TrackerMulti{}, nil
}

func (t *TrackerMulti) SetConfigJson(configJson string) error {
	config := &TrackerMultiConfig{}
	if err := unmarshal(configJson, config); err != nil {
		return err
	}
	t.trackers = make([]Tracker, len(config.Trackers))
	for tcIndex, trackerConfig := range config.Trackers {
		newTracker, err := NewTracker(trackerConfig.Name)
		if err != nil {
			return fmt.Errorf("configuring tracker multi: creating tracker index %d (%q): %w", tcIndex, trackerConfig.Name, err)
		}
		if err = newTracker.SetConfigJson(trackerConfig.Config); err != nil {
			return fmt.Errorf("configuring tracker multi: configuring tracker index %d (%q): %w", tcIndex, trackerConfig.Name, err)
		}
		t.trackers[tcIndex] = newTracker
	}
	t.config = config
	return nil
}

func (t *TrackerMulti) GetConfigJson() string {
	if t.config == nil {
		return ""
	}
	if configStr, err := json.Marshal(t.config); err == nil {
		return string(configStr)
	}
	return ""
}

func (t *TrackerMulti) AddPids(pids []int) {
	for _, tracker := range t.trackers {
		tracker.AddPids(pids)
	}
}

func (t *TrackerMulti) RemovePids(pids []int) {
	for _, tracker := range t.trackers {
		tracker.RemovePids(pids)
	}
}

func (t *TrackerMulti) Start() error {
	for tIndex, tracker := range t.trackers {
		if err := tracker.Start(); err != nil {
			return fmt.Errorf("starting tracker multi: starting tracker index %d: %w", tIndex, err)
		}
	}
	return nil
}

func (t *TrackerMulti) Stop() {
	for _, tracker := range t.trackers {
		tracker.Stop()
	}
}

func (t *TrackerMulti) ResetCounters() {
	for _, tracker := range t.trackers {
		tracker.ResetCounters()
	}
}

func (t *TrackerMulti) GetCounters() *TrackerCounters {
	var tcs *TrackerCounters = &TrackerCounters{}
	for _, tracker := range t.trackers {
		*tcs = append(*tcs, *tracker.GetCounters()...)
	}
	return tcs.Flattened(nil, nil)
}

func (t *TrackerMulti) Dump(args []string) string {
	dumps := make([]string, len(t.trackers))
	for tIndex, tracker := range t.trackers {
		dumps[tIndex] = fmt.Sprintf("tracker %d:\n%s", tIndex, tracker.Dump(args))
	}
	return strings.Join(dumps, "\n")
}
