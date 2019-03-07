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

package none

import (
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/policy"
	logger "github.com/intel/cri-resource-manager/pkg/log"
)

const (
	// PolicyName is the symbol used to pull us in as a builtin policy.
	PolicyName = "none"
	// PolicyDescription is a short description of this policy.
	PolicyDescription = "A no-op policy, doing pretty much nothing."
)

type none struct {
	logger.Logger
}

var _ policy.Backend = &none{}

// CreateNonePolicy creates a new policy instance.
func CreateNonePolicy(opts *policy.PolicyOpts) policy.Backend {
	n := &none{Logger: logger.NewLogger(PolicyName)}
	n.Info("creating policy...")
	return n
}

// Name returns the name of this policy.
func (n *none) Name() string {
	return PolicyName
}

// Description returns the description for this policy.
func (n *none) Description() string {
	return PolicyDescription
}

// Start prepares this policy for accepting allocation/release requests.
func (n *none) Start(cch cache.Cache) error {
	n.Debug("got started...")
	return nil
}

// AllocateResources is a resource allocation request for this policy.
func (n *none) AllocateResources(c cache.Container) error {
	n.Debug("(not) allocating container %s...", c.GetCacheId())
	return nil
}

// ReleaseResources is a resource release request for this policy.
func (n *none) ReleaseResources(c cache.Container) error {
	n.Debug("(not) releasing container %s...", c.GetCacheId())
	return nil
}

// UpdateResources is a resource allocation update request for this policy.
func (n *none) UpdateResources(c cache.Container) error {
	n.Debug("(not) updating container %s...", c.GetCacheId())
	return nil
}

// ExportResourceData provides resource data to export for the container.
func (n *none) ExportResourceData(c cache.Container, syntax policy.DataSyntax) []byte {
	return nil
}

func (n *none) PostStart(cch cache.Container) error {
	n.Debug("post start container...")
	return nil
}

// SetConfig sets the policy backend configuration
func (n *none) SetConfig(string) error {
	return nil
}

//
// Automatically register us as a policy implementation.
//

// Implementation is the implementation we register with the policy module.
type Implementation func(*policy.PolicyOpts) policy.Backend

// Name returns the name of this policy implementation.
func (n Implementation) Name() string {
	return PolicyName
}

// Description returns the desccription of this policy implementation.
func (n Implementation) Description() string {
	return PolicyDescription
}

// CreateFn returns the functions used to instantiate this policy.
func (n Implementation) CreateFn() policy.CreateFn {
	return policy.CreateFn(n)
}

var _ policy.Implementation = Implementation(nil)

func init() {
	policy.Register(Implementation(CreateNonePolicy))
}
