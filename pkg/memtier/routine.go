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

type RoutineConfig struct {
	Name   string
	Config string
}

type Routine interface {
	SetConfigJson(string) error // Set new configuration.
	GetConfigJson() string      // Get current configuration.
	Start() error
	Stop()
	Dump(args []string) string
}

type RoutineCreator func() (Routine, error)

// policies is a map of policy name -> policy creator
var routines map[string]RoutineCreator = make(map[string]RoutineCreator, 0)

func RoutineRegister(name string, creator RoutineCreator) {
	routines[name] = creator
}

func RoutineList() []string {
	keys := make([]string, 0, len(routines))
	for key := range routines {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func NewRoutine(name string) (Routine, error) {
	if creator, ok := routines[name]; ok {
		return creator()
	}
	return nil, fmt.Errorf("invalid routine name %q", name)
}
