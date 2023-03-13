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

type PolicyConfig struct {
	Name   string
	Config string
}

type Policy interface {
	SetConfigJson(string) error // Set new configuration.
	GetConfigJson() string      // Get current configuration.
	Start() error
	Stop()
	// PidWatcher, Mover and Tracker are mostly for debugging in interactive prompt...
	PidWatcher() PidWatcher
	Mover() *Mover
	Tracker() Tracker
	Dump(args []string) string
}

type PolicyCreator func() (Policy, error)

// policies is a map of policy name -> policy creator
var policies map[string]PolicyCreator = make(map[string]PolicyCreator, 0)

func PolicyRegister(name string, creator PolicyCreator) {
	policies[name] = creator
}

func PolicyList() []string {
	keys := make([]string, 0, len(policies))
	for key := range policies {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func NewPolicy(name string) (Policy, error) {
	if creator, ok := policies[name]; ok {
		return creator()
	}
	return nil, fmt.Errorf("invalid policy name %q", name)
}
