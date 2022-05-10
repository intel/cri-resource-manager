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

// GetClasses returns all available CPU classes.
func GetClasses() map[string]Class {
	return getCPUController().config.getClasses()
}

// Assign assigns a set of cpus to a class.
//
// TODO: Drop this function. Don't store cpu class in policy data but implement
// controller-specific data store in cache.
func Assign(c cache.Cache, class string, cpus ...int) error {
	// NOTE: no locking implemented anywhere around -> we don't expect multiple parallel callers

	// Store the class assignment. Assign cpus to a class and remove them from
	// other classes
	assignments := *getClassAssignments(c)

	if this, ok := assignments[class]; !ok {
		assignments[class] = utils.NewIDSetFromIntSlice(cpus...)
	} else {
		this.Add(cpus...)
	}

	for k, v := range assignments {
		if k != class {
			v.Del(cpus...)

			// Don't store empty classes, serves as a garbage collector, too
			if v.Size() == 0 {
				delete(assignments, k)
			}
		}
	}

	setClassAssignments(c, &assignments)

	if getCPUController().started {
		// We don't want to try to enforce until the controller has been fully
		// started. Enforcement of all assignments happens on StarT(), anyway.
		if err := getCPUController().enforce(class, cpus...); err != nil {
			log.Error("cpu class enforcement failed: %v", err)
		}
	}

	return nil
}
