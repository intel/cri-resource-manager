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

package memtier

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"sigs.k8s.io/yaml"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
)

const (
	// annotation key for opting in to multiple isolated exclusive CPUs per container.
	keyIsolationPreference = "prefer-isolated-cpus"
	// annotation key for opting out of exclusive allocation and relaxed topology fitting.
	keySharedCPUPreference = "prefer-shared-cpus"
	// annotation key for type of memory to allocate
	keyMemoryTypePreference = "memory-type"
	// annotation key for type "cold start" of workloads
	keyColdStartPreference = "cold-start"
)

// types by memory type name
var memoryNamedTypes = map[string]memoryType{
	"dram":  memoryDRAM,
	"pmem":  memoryPMEM,
	"hbm":   memoryHBM,
	"mixed": memoryAll,
}

// names by memory type
var memoryTypeNames = map[memoryType]string{
	memoryDRAM: "DRAM",
	memoryPMEM: "PMEM",
	memoryHBM:  "HBM",
}

// memoryType is bitmask of types of memory to allocate
type memoryType int

// memoryType bits
const (
	memoryUnspec memoryType = (0x1 << iota) >> 1
	memoryDRAM
	memoryPMEM
	memoryHBM
	memoryFirstUnusedBit
	memoryAll = memoryFirstUnusedBit - 1

	// type of memory to use if none specified
	defaultMemoryType = memoryAll
)

// podIsolationPreference checks if containers explicitly prefers to run on multiple isolated CPUs.
// The first return value indicates whether the container is isolated or not.
// The second return value indicates whether that decision was explicit (true) or implicit (false).
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
// The first return value indicates if the container prefers to opt-out from
// exclusive (sliced-off or isolated) CPU allocation even if it was otherwise
// eligible for it.
// The second return value, elevate, indicates how much to elevate the actual
// allocation of the container in the tree of pools. Or in other words how many
// levels to go up in the tree starting at the best fitting pool, before
// assigning the container to an actual pool.
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

// ColdStartPreference lists the various ways the container can be configured to trigger
// cold start. Currently, only timer is supported. If the "duration" is set to a duration
// greater than 0, cold start is enabled and the DRAM controller is added to the container
// after the duration has passed.
type ColdStartPreference struct {
	duration time.Duration
}

type coldStartPreferenceYaml struct {
	Duration string // `yaml:"duration,omitempty"`
}

// coldStartPreference figures out if the container memory should be first allocated from PMEM.
// It returns the time (in milliseconds) after which DRAM controller should be added to the mix.
func coldStartPreference(pod cache.Pod, container cache.Container) (ColdStartPreference, error) {
	value, ok := pod.GetResmgrAnnotation(keyColdStartPreference)
	if !ok {
		return ColdStartPreference{}, nil
	}
	preferences := map[string]coldStartPreferenceYaml{}
	if err := yaml.Unmarshal([]byte(value), &preferences); err != nil {
		log.Error("failed to parse cold start preference %s = '%s': %v",
			keyColdStartPreference, value, err)
		return ColdStartPreference{}, err
	}
	name := container.GetName()
	coldStartPreference, ok := preferences[name]
	if !ok {
		log.Debug("container %s has no entry among cold start preferences", container.PrettyName())
		return ColdStartPreference{}, nil
	}

	// Parse the cold start data.
	duration, err := time.ParseDuration(coldStartPreference.Duration)
	if err != nil {
		log.Error("failed to parse cold start timeout %s: %v",
			coldStartPreference.Duration, err)
		return ColdStartPreference{}, err
	}

	if duration < 0 || duration > time.Hour {
		// Duration can't be negative. We also reject durations which are longer than one hour.
		return ColdStartPreference{}, fmt.Errorf("failed to validate cold start timeout %s: value out of scope", duration.String())
	}

	return ColdStartPreference{
		duration: duration,
	}, nil
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

// podMemoryTypePreference returns what type of memory should be allocated for the container.
func podMemoryTypePreference(pod cache.Pod, c cache.Container) memoryType {
	value, ok := pod.GetResmgrAnnotation(keyMemoryTypePreference)
	if !ok {
		log.Debug("pod %s has no memory preference annotations", pod.GetName())
		return memoryUnspec
	}

	// Try to parse as per-container preference. Assume common for all containers if fails.
	pref := ""
	preferences := map[string]string{}
	if err := yaml.Unmarshal([]byte(value), &preferences); err == nil {
		name := c.GetName()
		p, ok := preferences[name]
		if !ok {
			log.Debug("container %s has no entry among memory preferences", c.PrettyName())
			return memoryUnspec
		}
		pref = p
	} else {
		pref = value
	}

	mtype, err := parseMemoryType(pref)
	if err != nil {
		log.Error("invalid memory type preference ('%s') in annotation %s: %v",
			pref, keyMemoryTypePreference, err)
		return memoryUnspec
	}
	log.Debug("container %s has effective memory preference: %s", c.PrettyName(), mtype)
	return mtype
}

// memoryAllocationPreference returns the amount and kind of memory to allocate.
func memoryAllocationPreference(pod cache.Pod, c cache.Container) (uint64, uint64, memoryType) {
	resources := c.GetResourceRequirements()
	mtype := podMemoryTypePreference(pod, c)
	req, lim := uint64(0), uint64(0)

	if memReq, ok := resources.Requests[corev1.ResourceMemory]; ok {
		req = uint64(memReq.Value())
	}
	if memLim, ok := resources.Limits[corev1.ResourceMemory]; ok {
		lim = uint64(memLim.Value())
	}

	return req, lim, mtype
}

// String stringifies a memoryType.
func (t memoryType) String() string {
	str := ""
	sep := ""
	for _, bit := range []memoryType{memoryDRAM, memoryPMEM, memoryHBM} {
		if int(t)&int(bit) != 0 {
			str += sep + memoryTypeNames[bit]
			sep = ","
		}
	}
	return str
}

// parseMemoryType parses a memory type string, ideally produced by String()
func parseMemoryType(value string) (memoryType, error) {
	if value == "" {
		return memoryUnspec, nil
	}
	mtype := 0
	for _, typestr := range strings.Split(value, ",") {
		t, ok := memoryNamedTypes[strings.ToLower(typestr)]
		if !ok {
			return memoryUnspec, policyError("unknown memory type value '%s'", typestr)
		}
		mtype |= int(t)
	}
	return memoryType(mtype), nil
}

// MarshalJSON is the JSON marshaller for memoryType.
func (t memoryType) MarshalJSON() ([]byte, error) {
	value := t.String()
	return json.Marshal(value)
}

// UnmarshalJSON is the JSON unmarshaller for memoryType
func (t *memoryType) UnmarshalJSON(data []byte) error {
	ival := 0
	if err := json.Unmarshal(data, &ival); err == nil {
		*t = memoryType(ival)
		return nil
	}

	value := ""
	if err := json.Unmarshal(data, &value); err != nil {
		return policyError("failed to unmarshal memoryType '%s': %v",
			string(data), err)
	}

	mtype, err := parseMemoryType(value)
	if err != nil {
		return policyError("failed parse memoryType '%s': %v", value, err)
	}

	*t = mtype
	return nil
}
