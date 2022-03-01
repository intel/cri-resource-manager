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
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"sigs.k8s.io/yaml"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/intel/cri-resource-manager/pkg/config"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/kubernetes"
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

	// effective annotation key for isolated CPU preference
	preferIsolatedCPUsKey = keyIsolationPreference + "." + kubernetes.ResmgrKeyNamespace
	// effective annotation key for shared CPU preference
	preferSharedCPUsKey = keySharedCPUPreference + "." + kubernetes.ResmgrKeyNamespace
	// effective annotation key for memory type preference
	preferMemoryTypeKey = keyMemoryTypePreference + "." + kubernetes.ResmgrKeyNamespace
	// effective annotation key for "cold start" preference
	preferColdStartKey = keyColdStartPreference + "." + kubernetes.ResmgrKeyNamespace
)

// cpuClass is a type of CPU to allocate
type cpuClass int

// names by cpu class
var cpuClassNames = map[cpuClass]string{
	cpuNormal:   "normal",
	cpuReserved: "reserved",
}

const (
	cpuNormal cpuClass = iota
	cpuReserved
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

// isolatedCPUsPreference returns whether isolated CPUs should be preferred for
// containers that allocate multiple CPUs, and if the container was explicitly
// annotated with this setting.
//
// If the effective annotations are not found, this function falls back to
// looking for the deprecated syntax by calling podIsolationPreference.
func isolatedCPUsPreference(pod cache.Pod, container cache.Container) (bool, bool) {
	key := preferIsolatedCPUsKey
	value, ok := pod.GetEffectiveAnnotation(key, container.GetName())
	if !ok {
		return podIsolationPreference(pod, container)
	}

	preference, err := strconv.ParseBool(value)
	if err != nil {
		log.Error("invalid CPU isolation preference annotation (%q, %q): %v",
			key, value, err)
		return opt.PreferIsolated, false
	}

	log.Debug("%s: effective CPU isolation preference %v", container.PrettyName(), preference)

	return preference, true
}

// sharedCPUsPreference returns whether shared CPUs should be preferred for
// containers otherwise eligible for exclusive allocation, and whether the
// container was explicitly annotated with this setting.
//
// If the effective annotations are not found, this function falls back to
// looking for the deprecated syntax by calling podSharedCPUPreference.
func sharedCPUsPreference(pod cache.Pod, container cache.Container) (bool, bool) {
	key := preferSharedCPUsKey
	value, ok := pod.GetEffectiveAnnotation(key, container.GetName())
	if !ok {
		return podSharedCPUPreference(pod, container)
	}

	preference, err := strconv.ParseBool(value)
	if err != nil {
		log.Error("invalid shared CPU preference annotation (%q, %q): %v",
			key, value, err)
		return opt.PreferShared, false
	}

	log.Debug("%s: effective shared CPU preference %v", container.PrettyName(), preference)

	return preference, true
}

// memoryTypePreference returns what type of memory should be allocated for the container.
//
// If the effective annotations are not found, this function falls back to
// looking for the deprecated syntax by calling podMemoryTypePreference.
func memoryTypePreference(pod cache.Pod, container cache.Container) memoryType {
	key := preferMemoryTypeKey
	value, ok := pod.GetEffectiveAnnotation(key, container.GetName())
	if !ok {
		return podMemoryTypePreference(pod, container)
	}

	mtype, err := parseMemoryType(value)
	if err != nil {
		log.Error("invalid memory type preference (%q, %q): %v", key, value, err)
		return memoryUnspec
	}

	log.Debug("%s: effective cold start preference %v", container.PrettyName(), mtype)

	return mtype
}

// coldStartPreference figures out 'cold start' preferences for the container, IOW
// if the container memory should be allocated for an initial 'cold start' period
// from PMEM, and how long this initial period should be.
//
// If the effective annotations are not found, this function falls back to
// looking for the deprecated syntax by calling podColdStartPreference.
func coldStartPreference(pod cache.Pod, container cache.Container) (ColdStartPreference, error) {
	key := preferColdStartKey
	value, ok := pod.GetEffectiveAnnotation(key, container.GetName())
	if !ok {
		return podColdStartPreference(pod, container)
	}

	preference := ColdStartPreference{}
	if err := yaml.Unmarshal([]byte(value), &preference); err != nil {
		log.Error("failed to parse cold start preference (%q, %q): %v",
			keyColdStartPreference, value, err)
		return ColdStartPreference{}, policyError("invalid cold start preference %q: %v",
			value, err)
	}

	if preference.Duration < 0 || time.Duration(preference.Duration) > time.Hour {
		return ColdStartPreference{}, policyError("cold start duration %s out of range",
			preference.Duration.String())
	}

	log.Debug("%s: effective cold start preference %v",
		container.PrettyName(), preference.Duration.String())

	return preference, nil
}

// podIsolationPreference checks if containers explicitly prefers to run on multiple isolated CPUs.
// The first return value indicates whether the container is isolated or not.
// The second return value indicates whether that decision was explicit (true) or implicit (false).
func podIsolationPreference(pod cache.Pod, container cache.Container) (bool, bool) {
	key := keyIsolationPreference
	value, ok := pod.GetResmgrAnnotation(key)
	if !ok {
		return opt.PreferIsolated, false
	}

	log.Warn("WARNING: using deprecated annotation %q", key)
	log.Warn("WARNING: consider using instead")
	log.Warn("WARNING:     %q, or", preferIsolatedCPUsKey+"/container."+container.GetName())
	log.Warn("WARNING:     %q", preferIsolatedCPUsKey+"/pod")

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
func podSharedCPUPreference(pod cache.Pod, container cache.Container) (bool, bool) {
	key := keySharedCPUPreference
	value, ok := pod.GetResmgrAnnotation(key)
	if !ok {
		return opt.PreferShared, false
	}

	log.Warn("WARNING: using deprecated annotation %q", key)
	log.Warn("WARNING: consider using instead")
	log.Warn("WARNING:     %q, or", preferSharedCPUsKey+"/container."+container.GetName())
	log.Warn("WARNING:     %q", preferSharedCPUsKey+"/pod")

	if value == "false" || value == "true" {
		return value[0] == 't', true
	}

	preferences := map[string]string{}
	if err := yaml.Unmarshal([]byte(value), &preferences); err != nil {
		log.Error("failed to parse shared CPU preference %s = '%s': %v",
			keySharedCPUPreference, value, err)
		return opt.PreferShared, false
	}

	name := container.GetName()
	pref, ok := preferences[name]
	if !ok {
		return opt.PreferShared, false
	}
	if pref == "false" || pref == "true" {
		return pref[0] == 't', true
	}

	log.Error("invalid shared CPU boolean preference for container %s: %s", name, pref)
	return opt.PreferShared, false
}

// ColdStartPreference lists the various ways the container can be configured to trigger
// cold start. Currently, only timer is supported. If the "duration" is set to a duration
// greater than 0, cold start is enabled and the DRAM controller is added to the container
// after the duration has passed.
type ColdStartPreference struct {
	Duration config.Duration // `json:"duration,omitempty"`
}

// podColdStartPreference figures out if the container memory should be first allocated from PMEM.
// It returns the time (in milliseconds) after which DRAM controller should be added to the mix.
func podColdStartPreference(pod cache.Pod, container cache.Container) (ColdStartPreference, error) {
	key := keyColdStartPreference
	value, ok := pod.GetResmgrAnnotation(key)
	if !ok {
		return ColdStartPreference{}, nil
	}

	log.Warn("WARNING: using deprecated annotation %q", key)
	log.Warn("WARNING: consider using instead")
	log.Warn("WARNING:     %q, or", preferColdStartKey+"/container."+container.GetName())
	log.Warn("WARNING:     %q", preferColdStartKey+"/pod")

	preferences := map[string]ColdStartPreference{}
	if err := yaml.Unmarshal([]byte(value), &preferences); err != nil {
		log.Error("failed to parse cold start preference %s = '%s': %v",
			key, value, err)
		return ColdStartPreference{}, err
	}
	name := container.GetName()
	preference, ok := preferences[name]
	if !ok {
		log.Debug("container %s has no entry among cold start preferences", container.PrettyName())
		return ColdStartPreference{}, nil
	}

	if preference.Duration < 0 || time.Duration(preference.Duration) > time.Hour {
		// Duration can't be negative. We also reject durations which are longer than one hour.
		return ColdStartPreference{}, fmt.Errorf("failed to validate cold start timeout %s: value out of scope", preference.Duration.String())
	}

	return preference, nil
}

func checkReservedPoolNamespaces(namespace string) bool {
	if namespace == metav1.NamespaceSystem {
		return true
	}

	for _, str := range opt.ReservedPoolNamespaces {
		ret, err := filepath.Match(str, namespace)
		if err != nil {
			return false
		}

		if ret {
			return true
		}
	}

	return false
}

// cpuAllocationPreferences figures out the amount and kind of CPU to allocate.
// Returned values:
// 1. full: number of full CPUs
// 2. fraction: amount of fractional CPU in milli-CPU
// 3. isolate: (bool) whether to prefer isolated full CPUs
// 4. cpuType: (cpuClass) class of CPU to allocate (reserved vs. normal)
func cpuAllocationPreferences(pod cache.Pod, container cache.Container) (int, int, bool, cpuClass) {
	//
	// CPU allocation preferences for a container consist of
	//
	//   - the number of exclusive cores to allocate
	//   - the amount of fractional cores to allocate (in milli-CPU)
	//   - whether kernel-isolated cores are preferred for exclusive allocation
	//   - cpu class IOW, whether reserved or normal cores should be allocated
	//
	// The rules for determining these preferences are:
	//
	//   - reserved cores are only and always preferred for kube-system namespace containers
	//   - kube-system namespace containers:
	//       => fractional/shared (reserved) cores
	//   - BestEffort QoS class containers:
	//       => fractional/shared cores
	//   - Burstable QoS class containers:
	//       => fractional/shared cores
	//   - Guaranteed QoS class containers:
	//      - 1 full core > CPU request
	//          => fractional/shared cores
	//      - 1 full core <= CPU request < 2 full cores:
	//          a. fractional allocation:
	//            - shared preference explicitly annotated/configured false:
	//              => mixed cores, prefer isolated, unless annotated/configured otherwise (*)
	//            - shared preference explicitly annotated/configured true:
	//              => shared cores
	//          b. non-fractional allocation:
	//            - shared preference explicitly annotated true:
	//              => shared cores
	//            - isolated default preference false or explicitly annotated false:
	//              => exclusive cores
	//            - isolated default preference true or explicitly annotated true:
	//              => exclusive cores, prefer isolated (*)
	//      - 2 full cores <= CPU request
	//          a. fractional allocation:
	//            - shared preference explicitly annotated false:
	//              => mixed cores, prefer isolated only if explicitly annotated (**)
	//            - otherwise (no shared annotation):
	//              => shared cores
	//          b. non-fractional allocation:
	//            - shared preference explicitly annotated true:
	//              => shared cores
	//            - otherwise (no shared annotation):
	//              => exclusive cores, prefer isolated only if explicitly annotated (**)
	//
	//   - Rationale for isolation defaults:
	//     *)
	//        In the single core case, a workload does not need to do anything extra to
	//        benefit from running on isolated vs. ordinary exclusive cores. Therefore,
	//        allocating isolated cores is a safe default choice.
	//     **)
	//        In the multiple cores case, a workload needs to be 'isolation-aware' to
	//        benefit (or actually to not even get hindered) by running on isolated vs.
	//        ordinary exclusive cores. If it gets isolated cores allocated, it needs
	//        to actively spread itself/its correct processes over the cores, because
	//        the scheduler is not going to do load-balancing for it. Therefore, the
	//        safe choice in this case is to not allocate isolated cores by default.
	//

	namespace := container.GetNamespace()
	request := container.GetResourceRequirements().Requests[corev1.ResourceCPU]
	qosClass := pod.GetQOSClass()
	fraction := int(request.MilliValue())

	// easy cases: kube-system namespace, Burstable or BestEffort QoS class containers
	switch {
	case checkReservedPoolNamespaces(namespace):
		return 0, fraction, false, cpuReserved
	case qosClass == corev1.PodQOSBurstable:
		return 0, fraction, false, cpuNormal
	case qosClass == corev1.PodQOSBestEffort:
		return 0, 0, false, cpuNormal
	}

	// complex case: Guaranteed QoS class containers
	cores := fraction / 1000
	fraction = fraction % 1000
	preferIsolated, explicitIsolated := isolatedCPUsPreference(pod, container)
	preferShared, explicitShared := sharedCPUsPreference(pod, container)

	switch {
	// sub-core CPU request
	case cores == 0:
		return 0, fraction, false, cpuNormal
		// 1 <= CPU request < 2
	case cores < 2:
		// fractional allocation, potentially mixed
		if fraction > 0 {
			if preferShared {
				return 0, 1000*cores + fraction, false, cpuNormal
			}
			return cores, fraction, preferIsolated, cpuNormal
		}
		// non-fractional allocation
		if preferShared && explicitShared {
			return 0, 1000*cores + fraction, false, cpuNormal
		}
		return cores, fraction, preferIsolated, cpuNormal
		// CPU request >= 2
	default:
		// fractional allocation, only mixed if explicitly annotated as unshared
		if fraction > 0 {
			if !preferShared && explicitShared {
				return cores, fraction, preferIsolated && explicitIsolated, cpuNormal
			}
			return 0, 1000*cores + fraction, false, cpuNormal
		}
		// non-fractional allocation
		if preferShared && explicitShared {
			return 0, 1000 * cores, false, cpuNormal
		}
		return cores, fraction, preferIsolated && explicitIsolated, cpuNormal
	}
}

// podMemoryTypePreference returns what type of memory should be allocated for the container.
func podMemoryTypePreference(pod cache.Pod, c cache.Container) memoryType {
	key := keyMemoryTypePreference
	value, ok := pod.GetResmgrAnnotation(key)
	if !ok {
		log.Debug("pod %s has no memory preference annotations", pod.GetName())
		return memoryUnspec
	}

	log.Warn("WARNING: using deprecated annotation %q", key)
	log.Warn("WARNING: consider using instead")
	log.Warn("WARNING:     %q, or", keyMemoryTypePreference+"/container."+c.GetName())
	log.Warn("WARNING:     %q", keyMemoryTypePreference+"/pod")

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
	mtype := memoryTypePreference(pod, c)
	req, lim := uint64(0), uint64(0)

	if memReq, ok := resources.Requests[corev1.ResourceMemory]; ok {
		req = uint64(memReq.Value())
	}
	if memLim, ok := resources.Limits[corev1.ResourceMemory]; ok {
		lim = uint64(memLim.Value())
	}

	return req, lim, mtype
}

// String stringifies a cpuClass.
func (t cpuClass) String() string {
	if cpuClassName, ok := cpuClassNames[t]; ok {
		return cpuClassName
	}
	return fmt.Sprintf("#UNNAMED-CPUCLASS(%d)", int(t))
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
