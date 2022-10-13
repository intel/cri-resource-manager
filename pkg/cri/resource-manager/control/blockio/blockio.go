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

package blockio

import (
	"fmt"

	"github.com/hashicorp/go-multierror"

	"github.com/intel/cri-resource-manager/pkg/blockio"
	"github.com/intel/cri-resource-manager/pkg/cri/client"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/control"
	logger "github.com/intel/cri-resource-manager/pkg/log"
)

const (
	// BlockIOController is the name of the block I/O controller.
	BlockIOController = cache.BlockIO
)

// blockio encapsulates the runtime state of our block I/O enforcement/controller.
type blockioctl struct {
	cache cache.Cache // resource manager cache
	idle  *bool       // true if we run without any classes configured
}

// Our logger instance.
var log logger.Logger = logger.NewLogger(BlockIOController)

// Our singleton block I/O controller instance.
var singleton *blockioctl

// getBlockIOController returns our singleton block I/O controller instance.
func getBlockIOController() *blockioctl {
	if singleton == nil {
		singleton = &blockioctl{}
	}
	return singleton
}

// Start initializes the controller for enforcing decisions.
func (ctl *blockioctl) Start(cache cache.Cache, client client.Client) error {
	ctl.cache = cache
	ctl.reconfigureRunningContainers()
	return nil
}

// Stop shuts down the controller.
func (ctl *blockioctl) Stop() {
}

// PreCreateHook is the block I/O controller pre-create hook.
func (ctl *blockioctl) PreCreateHook(c cache.Container) error {
	return nil
}

// PreStartHook is the block I/O controller pre-start hook.
func (ctl *blockioctl) PreStartHook(c cache.Container) error {
	return nil
}

// PostStartHook is the block I/O controller post-start hook.
func (ctl *blockioctl) PostStartHook(c cache.Container) error {
	if !c.HasPending(BlockIOController) {
		return nil
	}

	if err := ctl.assign(c); err != nil {
		return err
	}

	c.ClearPending(BlockIOController)

	return nil
}

// PostUpdateHook is the block I/O controller post-update hook.
func (ctl *blockioctl) PostUpdateHook(c cache.Container) error {
	if !c.HasPending(BlockIOController) {
		return nil
	}

	if err := ctl.assign(c); err != nil {
		return err
	}

	c.ClearPending(BlockIOController)

	return nil
}

// PostStop is the block I/O controller post-stop hook.
func (ctl *blockioctl) PostStopHook(c cache.Container) error {
	return nil
}

// UpdateConfig activates an updated configuration.
func (ctl *blockioctl) UpdateConfig() error {
	return ctl.reconfigure(false)
}

// RevertConfig reverts configuration after a failed update.
func (ctl *blockioctl) RevertConfig() error {
	return ctl.reconfigure(true)
}

// isImplicitlyDisabled checks if we run without any classes confiured
func (ctl *blockioctl) isImplicitlyDisabled() bool {
	if ctl.idle != nil {
		return *ctl.idle
	}

	idle := len(blockio.GetClasses()) == 0
	if idle {
		log.Warn("controller implictly disabled (no configured classes)")
	}
	ctl.idle = &idle

	return *ctl.idle
}

// assign assigns the container to the given block I/O class.
func (ctl *blockioctl) assign(c cache.Container) error {
	class := c.GetBlockIOClass()
	if class == "" {
		return nil
	}

	if ctl.isImplicitlyDisabled() && cache.IsPodQOSClassName(class) {
		return nil
	}

	if err := blockio.SetContainerClass(c, class); err != nil {
		return blockioError("%q: failed to assign to class %q: %w", c.PrettyName(), class, err)
	}

	log.Info("%q: assigned to class %q", c.PrettyName(), class)

	return nil
}

// reconfigure blockio class mapping.
func (ctl *blockioctl) reconfigure(isRevert bool) error {
	err := blockio.UpdateOciConfig(isRevert)
	if err != nil {
		return err
	}
	// Possible errors in reconfiguring running containers are not errors in
	// the updated configuration, therefore silently ignored.
	ctl.reconfigureRunningContainers()

	// We'll re-check idleness at next operation/request.
	ctl.idle = nil

	return nil
}

// reconfigureRunningContainers force setting current blockio configuration to all containers running on the node
func (ctl *blockioctl) reconfigureRunningContainers() error {
	var errors *multierror.Error
	if ctl.cache == nil {
		return nil
	}
	for _, c := range ctl.cache.GetContainers() {
		class := c.GetBlockIOClass()
		log.Debug("%q: configure blockio class %q", c.PrettyName(), class)
		err := blockio.SetContainerClass(c, class)
		if err != nil {
			errors = multierror.Append(errors, err)
		}
	}
	return errors.ErrorOrNil()
}

// blockioError creates a block I/O-controller-specific formatted error message.
func blockioError(format string, args ...interface{}) error {
	return fmt.Errorf("blockio: "+format, args...)
}

// init registers this controller and sets configuration change handling.
func init() {
	control.Register(BlockIOController, "Block I/O controller", getBlockIOController())
}
