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

package cache

import (
	"io/ioutil"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	resapi "k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/sets"
	cri "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// Constants/variables needed for converting between milliCPU, CFS shares, quota and period.
const (
	// CFS CPU shares, quota and period to/from milliCPU conversion
	minShares      = 2
	sharesPerCPU   = 1024
	milliCPUToCPU  = 1000
	QuotaPeriod    = 100000
	minQuotaPeriod = 1000

	// memory limit to OOM score adjustement
	guaranteedOOMScoreAdj int = -998
	besteffortOOMScoreAdj int = 1000
)

var memoryCapacity int64

// estimateComputeResources calculates resource requests/limits from a CRI request.
func estimateComputeResources(lnx *cri.LinuxContainerResources) corev1.ResourceRequirements {
	resources := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{},
		Limits:   corev1.ResourceList{},
	}

	if lnx == nil {
		return resources
	}

	// calculate CPU request
	if value := SharesToMilliCPU(lnx.CpuShares); value > 0 {
		qty := resapi.NewMilliQuantity(value, resapi.DecimalSI)
		resources.Requests[corev1.ResourceCPU] = *qty
	}

	// calculate CPU limit
	if value := QuotaToMilliCPU(lnx.CpuQuota, lnx.CpuPeriod); value > 0 {
		qty := resapi.NewMilliQuantity(value, resapi.DecimalSI)
		resources.Limits[corev1.ResourceCPU] = *qty
	}

	// calculate memory request
	if value := OomScoreAdjToMemoryRequest(lnx.OomScoreAdj); value != 0 {
		qty := resapi.NewQuantity(value, resapi.DecimalSI)
		resources.Requests[corev1.ResourceMemory] = *qty
	}

	// calculate memory limit
	if value := lnx.MemoryLimitInBytes; value > 0 {
		qty := resapi.NewQuantity(value, resapi.DecimalSI)
		resources.Limits[corev1.ResourceMemory] = *qty
	}

	return resources
}

// SharesToMilliCPU converts CFS CPU shares to milliCPU.
func SharesToMilliCPU(shares int64) int64 {
	if shares == minShares {
		return 0
	}

	return int64((float64(shares*milliCPUToCPU) / float64(sharesPerCPU)) + 0.5)
}

// MilliCPUToShares converts milliCPU to CFS CPU shares.
func MilliCPUToShares(milliCPU int) int64 {
	if milliCPU == 0 {
		return minShares
	}

	shares := (milliCPU * sharesPerCPU) / milliCPUToCPU
	if shares < minShares {
		return minShares
	}

	return int64(shares)
}

// QuotaToMilliCPU converts CFS quota and period to milliCPU.
func QuotaToMilliCPU(quota, period int64) int64 {
	if quota == 0 || period == 0 {
		return 0
	}

	return int64(float64(quota*milliCPUToCPU)/float64(period) + 0.5)
}

// MilliCPUToQuota converts milliCPU to CFS quota and period values.
func MilliCPUToQuota(milliCPU int64) (int64, int64) {
	if milliCPU == 0 {
		return 0, 0
	}

	period := int64(QuotaPeriod)
	quota := (milliCPU * period) / milliCPUToCPU
	if quota < minQuotaPeriod {
		quota = minQuotaPeriod
	}

	return quota, period
}

// MemoryRequestToOomScoreAdj -- We don't do this direction (we leave the adjustment intact)...
func MemoryRequestToOomScoreAdj(namespace string, configSource string, qos corev1.PodQOSClass,
	memRequest int64) int64 {
	panic("this shouldn't be called... better leave the OOM score adjustment intact.")
	// return 0
}

// OomScoreAdjToMemoryRequest tries to convert OOM score to original memory request.
func OomScoreAdjToMemoryRequest(value int64) int64 {
	if value == 0 {
		return 0
	}

	switch value {
	case int64(1000 + guaranteedOOMScoreAdj):
		// lossy case, let's use boundary OOMScoreAdj value
		value = int64(1000 + guaranteedOOMScoreAdj - 1)
		return (memoryCapacity * (int64(1000) - value)) / int64(1000)

	case int64(besteffortOOMScoreAdj - 1):
		value = int64(besteffortOOMScoreAdj)
		return (memoryCapacity * (int64(1000) - value)) / int64(1000)

	default:
		return (memoryCapacity * (int64(1000) - value)) / int64(1000)
	}
}

// getMemoryCapacity parses memory capacity from /proc/meminfo (mimicking cAdvisor).
func getMemoryCapacity() int64 {
	var data []byte
	var err error

	if memoryCapacity > 0 {
		return memoryCapacity
	}

	if data, err = ioutil.ReadFile("/proc/meminfo"); err != nil {
		return -1
	}

	for _, line := range strings.Split(string(data), "\n") {
		keyval := strings.Split(line, ":")
		if len(keyval) != 2 || keyval[0] != "MemTotal" {
			continue
		}

		valunit := strings.Split(strings.TrimSpace(keyval[1]), " ")
		if len(valunit) != 2 || valunit[1] != "kB" {
			return -1
		}

		memoryCapacity, err = strconv.ParseInt(valunit[0], 10, 64)
		if err != nil {
			return -1
		}

		memoryCapacity *= 1024
		break
	}

	return memoryCapacity
}

// cgroupParentToQOS tries to map Pod cgroup parent to QOS class.
func cgroupParentToQOS(dir string) corev1.PodQOSClass {
	var qos corev1.PodQOSClass

	// The parent directory naming scheme depends on the cgroup driver in use.
	// Thus, rely on substring matching
	split := strings.Split(strings.TrimPrefix(dir, "/"), "/")
	switch {
	case len(split) < 2:
		qos = corev1.PodQOSClass("")
	case strings.Index(split[1], strings.ToLower(string(corev1.PodQOSBurstable))) != -1:
		qos = corev1.PodQOSBurstable
	case strings.Index(split[1], strings.ToLower(string(corev1.PodQOSBestEffort))) != -1:
		qos = corev1.PodQOSBestEffort
	default:
		qos = corev1.PodQOSGuaranteed
	}

	return qos
}

// resourcesToQOS tries to map Pod container resources (from annotation) to QOS class.
func resourcesToQOS(podResources *PodResourceRequirements) corev1.PodQOSClass {
	var qos corev1.PodQOSClass

	if podResources == nil {
		return qos
	}

	requests := corev1.ResourceList{}
	limits := corev1.ResourceList{}
	zeroQuantity := resapi.MustParse("0")
	isGuaranteed := true
	for _, resources := range podResources.Containers {
		// process requests
		for name, quantity := range resources.Requests {
			if !isSupportedQoSComputeResource(name) {
				continue
			}
			if quantity.Cmp(zeroQuantity) == 1 {
				delta := quantity.Copy()
				if _, exists := requests[name]; !exists {
					requests[name] = *delta
				} else {
					delta.Add(requests[name])
					requests[name] = *delta
				}
			}
		}
		// process limits
		qosLimitsFound := sets.NewString()
		for name, quantity := range resources.Limits {
			if !isSupportedQoSComputeResource(name) {
				continue
			}
			if quantity.Cmp(zeroQuantity) == 1 {
				qosLimitsFound.Insert(string(name))
				delta := quantity.Copy()
				if _, exists := limits[name]; !exists {
					limits[name] = *delta
				} else {
					delta.Add(limits[name])
					limits[name] = *delta
				}
			}
		}

		if !qosLimitsFound.HasAll(string(corev1.ResourceMemory), string(corev1.ResourceCPU)) {
			isGuaranteed = false
		}
	}
	if len(requests) == 0 && len(limits) == 0 {
		return corev1.PodQOSBestEffort
	}
	// Check is requests match limits for all resources.
	if isGuaranteed {
		for name, req := range requests {
			if lim, exists := limits[name]; !exists || lim.Cmp(req) != 0 {
				isGuaranteed = false
				break
			}
		}
	}
	if isGuaranteed &&
		len(requests) == len(limits) {
		return corev1.PodQOSGuaranteed
	}
	return corev1.PodQOSBurstable
}

/*
// ContainerQOS tries to map Pod container resources (from annotation) to QOS class.
func resourceRequirementsToQOS(resources *corev1.ResourceRequirements) corev1.PodQOSClass {
	var qos corev1.PodQOSClass

	if resources == nil {
		return qos
	}

	if podResources == nil {
		return qos
	}

	requests := corev1.ResourceList{}
	limits := corev1.ResourceList{}
	zeroQuantity := resapi.MustParse("0")
	isGuaranteed := true
	for _, resources := range podResources.Containers {
		// process requests
		for name, quantity := range resources.Requests {
			if !isSupportedQoSComputeResource(name) {
				continue
			}
			if quantity.Cmp(zeroQuantity) == 1 {
				delta := quantity.Copy()
				if _, exists := requests[name]; !exists {
					requests[name] = *delta
				} else {
					delta.Add(requests[name])
					requests[name] = *delta
				}
			}
		}
		// process limits
		qosLimitsFound := sets.NewString()
		for name, quantity := range resources.Limits {
			if !isSupportedQoSComputeResource(name) {
				continue
			}
			if quantity.Cmp(zeroQuantity) == 1 {
				qosLimitsFound.Insert(string(name))
				delta := quantity.Copy()
				if _, exists := limits[name]; !exists {
					limits[name] = *delta
				} else {
					delta.Add(limits[name])
					limits[name] = *delta
				}
			}
		}

		if !qosLimitsFound.HasAll(string(corev1.ResourceMemory), string(corev1.ResourceCPU)) {
			isGuaranteed = false
		}
	}
	if len(requests) == 0 && len(limits) == 0 {
		return corev1.PodQOSBestEffort
	}
	// Check is requests match limits for all resources.
	if isGuaranteed {
		for name, req := range requests {
			if lim, exists := limits[name]; !exists || lim.Cmp(req) != 0 {
				isGuaranteed = false
				break
			}
		}
	}
	if isGuaranteed &&
		len(requests) == len(limits) {
		return corev1.PodQOSGuaranteed
	}
	return corev1.PodQOSBurstable
}
*/

func isSupportedQoSComputeResource(name corev1.ResourceName) bool {
	return name == corev1.ResourceCPU || name == corev1.ResourceMemory
}

func init() {
	// TODO: get rid of this eventually, use pkg/sysfs instead...
	getMemoryCapacity()
}
