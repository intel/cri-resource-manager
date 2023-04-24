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

//go:build !benchmarking
// +build !benchmarking

package balloons

import (
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"

	idset "github.com/intel/goresctrl/pkg/utils"
)

func (p *balloons) instValidateConfig(bpoptions *BalloonsOptions) error {
	for _, binst := range bpoptions.BalloonInsts {
		if len(binst.AllowedCpuPackages) > 0 ||
			len(binst.AllowedCpuNumas) > 0 ||
			len(binst.AllowedCpus) > 0 ||
			len(binst.ForcedMemNumas) > 0 {
			return balloonsError("this balloons policy build does not support benchmarking features: allowed/forced CPU and memory pinning")
		}
	}
	return nil
}

func (p *balloons) instAllowedCpus(blnDef *BalloonDef, instance int, availCpus cpuset.CPUSet) cpuset.CPUSet {
	// Ignore forced balloon allocations in production builds.
	return availCpus
}

func (p *balloons) instForcedMems(blnDef *BalloonDef, instance int) idset.IDSet {
	// No forced memory nodes.
	return idset.NewIDSet()
}
