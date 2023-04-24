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

//go:build benchmarking
// +build benchmarking

package balloons

import (
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"

	idset "github.com/intel/goresctrl/pkg/utils"
)

func (p *balloons) instValidateConfig(bpoptions *BalloonsOptions) error {
	return nil
}

func (p *balloons) instAllowedCpus(blnDef *BalloonDef, instance int, availCpus cpuset.CPUSet) cpuset.CPUSet {
	mask := p.instAllowedCpuMask(blnDef, instance)
	if mask != nil {
		allowed := availCpus.Intersection(*mask)
		log.Debugf("benchmarking: restricting CPU allocation for balloon %s[%d] from free %s, allowed mask %s, result: %s", blnDef.Name, instance, availCpus, *mask, allowed)
		return allowed
	}
	return availCpus
}

func (p *balloons) instForcedMems(blnDef *BalloonDef, instance int) idset.IDSet {
	for _, binst := range p.balloonInsts(blnDef, instance) {
		if len(binst.ForcedMemNumas) > 0 {
			forced := idset.NewIDSet(binst.ForcedMemNumas...)
			log.Debugf("benchmarking: forcing memory of balloon %s[%d] to %v", blnDef.Name, instance, forced)
			return forced
		}
	}
	return idset.NewIDSet()
}

func (p *balloons) balloonInsts(blnDef *BalloonDef, instance int) []*BalloonInst {
	binsts := []*BalloonInst{}
	for _, binst := range p.bpoptions.BalloonInsts {
		if binst.Type == blnDef.Name && containsInt(binst.Instances, instance) {
			binsts = append(binsts, binst)
		}
	}
	return binsts
}

func containsInt(haystack []int, needle int) bool {
	for _, hay := range haystack {
		if hay == needle {
			return true
		}
	}
	return false
}

func (p *balloons) instAllowedCpuMask(blnDef *BalloonDef, instance int) *cpuset.CPUSet {
	for _, binst := range p.balloonInsts(blnDef, instance) {
		switch {
		case len(binst.AllowedCpuPackages) > 0:
			cpus := p.cpuTree.CpusIn(CPUTopologyLevelPackage, binst.AllowedCpuPackages)
			return &cpus
		case len(binst.AllowedCpuNumas) > 0:
			cpus := p.cpuTree.CpusIn(CPUTopologyLevelNuma, binst.AllowedCpuNumas)
			return &cpus
		case len(binst.AllowedCpus) > 0:
			cpus := cpuset.NewCPUSet(binst.AllowedCpus...)
			return &cpus
		}
	}
	return nil
}
