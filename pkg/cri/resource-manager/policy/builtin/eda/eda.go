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

package eda

import (
	"fmt"
	"strings"

	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"

	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/policy"
	logger "github.com/intel/cri-resource-manager/pkg/log"
)

const (
	// PolicyName is the symbol used to pull us in as a builtin policy.
	PolicyName = "eda"
	// PolicyDescription is a short description of this policy.
	PolicyDescription = "Enforcement for CPU device plugin."
	// cpusetAnnotationPrefix is the prefix of container annotations
	// that the external agent uses for communicating the cpuset enforcement.
	cpuSetContainerAnnotationPrefix = "cpu."
)

type eda struct {
	logger.Logger

	state cache.Cache // state cache
}

var _ policy.Backend = &eda{}

//
// Policy backend implementation
//

// CreateEdaPolicy creates a new policy instance.
func CreateEdaPolicy(opts *policy.BackendOptions) policy.Backend {
	eda := &eda{Logger: logger.NewLogger(PolicyName)}
	eda.Info("creating policy...")
	// TODO: policy configuration (if any)
	return eda
}

// Name returns the name of this policy.
func (eda *eda) Name() string {
	return PolicyName
}

// Description returns the description for this policy.
func (eda *eda) Description() string {
	return PolicyDescription
}

// Start prepares this policy for accepting allocation/release requests.
func (eda *eda) Start(cch cache.Cache, add []cache.Container, del []cache.Container) error {
	eda.Debug("preparing for making decisions...")
	return nil
}

// Sync synchronizes the state of this policy.
func (eda *eda) Sync(add []cache.Container, del []cache.Container) error {
	eda.Debug("synchronizing state...")
	for _, c := range del {
		eda.ReleaseResources(c)
	}
	for _, c := range add {
		eda.AllocateResources(c)
	}

	return nil
}

// AllocateResources is a resource allocation request for this policy.
func (eda *eda) AllocateResources(c cache.Container) error {
	containerID := c.GetCacheID()
	eda.Debug("allocating resources for container %s...", containerID)

	// Allocate (CPU) resources for the container
	keys := c.GetResmgrAnnotationKeys()
	cpus := cpuset.CPUSet{}
	for _, key := range keys {
		if strings.HasPrefix(key, cpuSetContainerAnnotationPrefix) {
			strValue, _ := c.GetResmgrAnnotation(key, nil)
			eda.Debug("detected cpuset annotation %s=%s", key, strValue)
			cpusetValue, err := cpuset.Parse(strValue)
			if err != nil {
				return edaError("failed to parse cpuset %q: %v", strValue, err)
			}
			cpus = cpus.Union(cpusetValue)
		}
	}
	if cpus.Size() > 0 {
		eda.Debug("enforcing cpuset of container %q to %q", containerID, cpus.String())
		c.SetCpusetCpus(cpus.String())
	}
	return nil
}

// ReleaseResources is a resource release request for this policy.
func (eda *eda) ReleaseResources(c cache.Container) error {
	eda.Debug("releasing resources of container %s...", c.PrettyName())
	return nil
}

// UpdateResources is a resource allocation update request for this policy.
func (eda *eda) UpdateResources(c cache.Container) error {
	eda.Debug("updating resource allocations of container %s...", c.PrettyName())
	return nil
}

// ExportResourceData provides resource data to export for the container.
func (eda *eda) ExportResourceData(c cache.Container, syntax policy.DataSyntax) []byte {
	return nil
}

func (eda *eda) PostStart(cch cache.Container) error {
	return nil
}

// SetConfig sets the policy backend configuration
func (eda *eda) SetConfig(conf string) error {
	return nil
}

//
// Helper functions for STP policy backend
//

func edaError(format string, args ...interface{}) error {
	return fmt.Errorf(PolicyName+": "+format, args...)
}

//
// Automatically register us as a policy implementation.
//

// Implementation is the implementation we register with the policy module.
type Implementation func(*policy.BackendOptions) policy.Backend

// Name returns the name of this policy implementation.
func (i Implementation) Name() string {
	return PolicyName
}

// Description returns the desccription of this policy implementation.
func (i Implementation) Description() string {
	return PolicyDescription
}

// CreateFn returns the functions used to instantiate this policy.
func (i Implementation) CreateFn() policy.CreateFn {
	return policy.CreateFn(i)
}

var _ policy.Implementation = Implementation(nil)

func init() {
	policy.Register(Implementation(CreateEdaPolicy))
}
