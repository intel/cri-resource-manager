// Copyright 2021 Intel Corporation. All Rights Reserved.
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

type PidWatcherConfig struct {
	Name   string
	Config string
}

type PidWatcher interface {
	SetConfigJson(string) error // Set new configuration.
	GetConfigJson() string      // Get current configuration.
	SetPidListener(PidListener)
	Poll() error
	Start() error
	Stop()
	Dump([]string) string
}

type PidListener interface {
	AddPids([]int)
	RemovePids([]int)
}

type PidWatcherCreator func() (PidWatcher, error)

// pidwatchers is a map of pidwatcher name -> pidwatcher creator
var pidwatchers map[string]PidWatcherCreator = make(map[string]PidWatcherCreator, 0)

func PidWatcherRegister(name string, creator PidWatcherCreator) {
	pidwatchers[name] = creator
}

func PidWatcherList() []string {
	keys := make([]string, 0, len(pidwatchers))
	for key := range pidwatchers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func NewPidWatcher(name string) (PidWatcher, error) {
	if creator, ok := pidwatchers[name]; ok {
		return creator()
	}
	return nil, fmt.Errorf("invalid pidwatcher name %q", name)
}
