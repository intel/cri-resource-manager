// Copyright The NRI Plugins Authors. All Rights Reserved.
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

package kubernetes

const (
	// Constants for converting back and forth between CPU requirements in
	// terms of milli-CPUs and kernel cgroup/scheduling parameters.

	// MinShares is the minimum cpu.shares accepted by cgroups.
	MinShares = 2
	// MaxShares is the minimum cpu.shares accepted by cgroups.
	MaxShares = 262144
	// SharesPerCPU is cpu.shares worth one full CPU.
	SharesPerCPU = 1024
	// MilliCPUToCPU is milli-CPUs worth a full CPU.
	MilliCPUToCPU = 1000
	// QuotaPeriod is 100000 microseconds, or 100ms
	QuotaPeriod = 100000
	// MinQuotaPeriod is 1000 microseconds, or 1ms
	MinQuotaPeriod = 1000
)

// MilliCPUToQuota converts milliCPU to CFS quota and period values.
// (Almost) identical to the same function in kubelet.
func MilliCPUToQuota(milliCPU int64) (quota, period int64) {
	if milliCPU == 0 {
		return 0, 0
	}

	// TODO(klihub): this is behind the CPUSFSQuotaPerdiod feature gate in kubelet
	period = int64(QuotaPeriod)

	quota = (milliCPU * period) / MilliCPUToCPU

	if quota < MinQuotaPeriod {
		quota = MinQuotaPeriod
	}

	return quota, period
}

// MilliCPUToShares converts the milliCPU to CFS shares.
// Identical to the same function in kubelet.
func MilliCPUToShares(milliCPU int64) uint64 {
	if milliCPU == 0 {
		return MinShares
	}
	shares := (milliCPU * SharesPerCPU) / MilliCPUToCPU
	if shares < MinShares {
		return MinShares
	}
	if shares > MaxShares {
		return MaxShares
	}
	return uint64(shares)
}

// SharesToMilliCPU converts CFS CPU shares to milli-CPUs.
func SharesToMilliCPU(shares int64) int64 {
	if shares == MinShares {
		return 0
	}
	return int64(float64(shares*MilliCPUToCPU)/float64(SharesPerCPU) + 0.5)
}

// QuotaToMilliCPU converts CFS quota and period to milli-CPUs.
func QuotaToMilliCPU(quota, period int64) int64 {
	if quota == 0 || period == 0 {
		return 0
	}
	return int64(float64(quota*MilliCPUToCPU)/float64(period) + 0.5)
}
