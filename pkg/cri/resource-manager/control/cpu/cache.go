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

package cpu

import (
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	"github.com/intel/goresctrl/pkg/utils"
)

const (
	cacheKeyCPUAssignments = "CPUClassAssignments"
)

// cpuClassAssignments contains the information about how cpus are assigned to
// classes
type cpuClassAssignments map[string]utils.IDSet

// Get the state of CPU class assignments from cache
func getClassAssignments(c cache.Cache) *cpuClassAssignments {
	a := &cpuClassAssignments{}

	if !c.GetPolicyEntry(cacheKeyCPUAssignments, a) {
		log.Error("no cached state of CPU class assignments found")
	}

	return a
}

// Save the state of CPU class assignments in cache
func setClassAssignments(c cache.Cache, a *cpuClassAssignments) {
	c.SetPolicyEntry(cacheKeyCPUAssignments, cache.Cachable(a))
}

// Set the value of cached cpuClassAssignments
func (c *cpuClassAssignments) Set(value interface{}) {
	switch value.(type) {
	case cpuClassAssignments:
		*c = value.(cpuClassAssignments)
	case *cpuClassAssignments:
		cp := value.(*cpuClassAssignments)
		*c = *cp
	}
}

// Get cached cpuClassAssignments
func (c *cpuClassAssignments) Get() interface{} {
	return *c
}
