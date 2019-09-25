// Copyright 2019 Intel Corporation. All Rights Reserved.
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

package topologyaware

import (
	"github.com/ghodss/yaml"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
)

const (
	// annotation key for opting in to multiple isolated exclusive CPUs per container.
	keyIsolationPreference = "prefer-isolated-cpus"
	// annotation key for opting out of exclusive allocation and relaxed topology fitting.
	keySharedCPUPreference = "prefer-shared-cpus"
)

// podIsolationPreference checks if containers explicitly prefers to run on multiple isolated CPUs.
func podIsolationPreference(pod cache.Pod, container cache.Container) (bool, bool) {
	value, ok := pod.GetResmgrAnnotation(keyIsolationPreference)
	if !ok {
		return opt.PreferIsolated, false
	}
	if value == "false" || value == "true" {
		return (value[0] == 't'), true
	}

	preferences := map[string]bool{}
	if err := yaml.Unmarshal([]byte(value), &preferences); err != nil {
		log.Error("failed to parse isolation preference %s = '%s': %v",
			keyIsolationPreference, value, err)
		return opt.PreferIsolated, false
	}

	name := container.GetName()
	if pref, ok := preferences[name]; ok {
		log.Debug("%s per-container isolation preference '%v'", name, pref)
		return pref, true
	}

	log.Debug("%s defaults to isolation preference '%v'", name, opt.PreferIsolated)
	return opt.PreferIsolated, false
}

// podSharedCPUPreference checks if a container wants to opt-out from exclusive allocation.
func podSharedCPUPreference(pod cache.Pod, container cache.Container) (bool, int) {
	value, ok := pod.GetResmgrAnnotation(keySharedCPUPreference)
	if !ok {
		return opt.PreferShared, 0
	}
	if value == "false" || value == "true" {
		return value[0] == 't', 0
	}

	preferences := map[string]string{}
	if err := yaml.Unmarshal([]byte(value), &preferences); err != nil {
		log.Error("failed to parse shared CPU preference %s = '%s': %v",
			keySharedCPUPreference, value, err)
		return opt.PreferShared, 0
	}

	name := container.GetName()
	pref, ok := preferences[name]
	if !ok {
		return opt.PreferShared, 0
	}
	if pref == "false" || pref == "true" {
		return pref[0] == 't', 0
	}

	elevate, err := strconv.ParseInt(pref, 0, 8)
	if err != nil {
		log.Error("invalid shared CPU preference for container %s (%s): %v", name, pref, err)
		return opt.PreferShared, 0
	}

	if elevate > 0 {
		log.Error("invalid (> 0) node displacement for container %s: %d", name, elevate)
		return opt.PreferShared, 0
	}

	return true, int(elevate)
}

// cpuAllocationPreferences figures out the amount and kind of CPU to allocate.
func cpuAllocationPreferences(pod cache.Pod, container cache.Container) (int, int, bool, int) {
	req, ok := container.GetResourceRequirements().Requests[corev1.ResourceCPU]
	if !ok {
		return 0, 0, false, 0
	}

	qos := pod.GetQOSClass()

	preferIsol, explicit := podIsolationPreference(pod, container)
	preferShared, elevate := podSharedCPUPreference(pod, container)

	full, fraction, isolate := 0, 0, false
	switch {
	case container.GetNamespace() == metav1.NamespaceSystem:
		full, fraction = 0, int(req.MilliValue())

	case qos == corev1.PodQOSBurstable || preferShared:
		full, fraction = 0, int(req.MilliValue())

	case qos == corev1.PodQOSGuaranteed:
		full = int(req.MilliValue()) / 1000
		fraction = int(req.MilliValue()) % 1000
	}

	if !preferShared {
		switch {
		case full == 1:
			if explicit {
				isolate = preferIsol
			} else {
				isolate = true
			}
		case full > 1:
			isolate = preferIsol && explicit
		}
	} else {
		elevate = -elevate
	}

	return full, fraction, isolate, elevate
}
