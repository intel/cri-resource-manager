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
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/events"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/introspect"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/policy"
	logger "github.com/intel/cri-resource-manager/pkg/log"
)

const (
	// PolicyName is the name used to activate this policy implementation.
	PolicyName = "none"
	// PolicyDescription is a short description of this policy.
	PolicyDescription = "A no-op policy, doing pretty much nothing."
)

type none struct {
	logger.Logger
	cch cache.Cache
}

var _ policy.Backend = &none{}

// CreateNonePolicy creates a new policy instance.
func CreateNonePolicy(opts *policy.BackendOptions) policy.Backend {
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
func (n *none) Start(add []cache.Container, del []cache.Container) error {
	n.Debug("got started...")
	return nil
}

// Sync synchronizes the active policy state.
func (n *none) Sync(add []cache.Container, del []cache.Container) error {
	n.Debug("(not) synchronizing policy state")
	return nil
}

// AllocateResources is a resource allocation request for this policy.
func (n *none) AllocateResources(c cache.Container) error {
	n.Debug("(not) allocating container %s...", c.PrettyName())
	return nil
}

// ReleaseResources is a resource release request for this policy.
func (n *none) ReleaseResources(c cache.Container) error {
	n.Debug("(not) releasing container %s...", c.PrettyName())
	return nil
}

// UpdateResources is a resource allocation update request for this policy.
func (n *none) UpdateResources(c cache.Container) error {
	n.Debug("(not) updating container %s...", c.PrettyName())
	return nil
}

// Rebalance tries to find an optimal allocation of resources for the current containers.
func (n *none) Rebalance() (bool, error) {
	n.Debug("(not) rebalancing containers...")
	return false, nil
}

// HandleEvent handles policy-specific events.
func (n *none) HandleEvent(*events.Policy) (bool, error) {
	n.Debug("(not) handling event...")
	return false, nil
}

// ExportResourceData provides resource data to export for the container.
func (n *none) ExportResourceData(c cache.Container) map[string]string {
	return nil
}

// Introspect provides data for external introspection.
func (n *none) Introspect(*introspect.State) {
	return
}

// Register us as a policy implementation.
func init() {
	policy.Register(PolicyName, PolicyDescription, CreateNonePolicy)
}
