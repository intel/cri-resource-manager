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

	"github.com/intel/cri-resource-manager/pkg/cri/client"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/control"
	"github.com/intel/cri-resource-manager/pkg/utils"

	"github.com/intel/cri-resource-manager/pkg/config"
	logger "github.com/intel/cri-resource-manager/pkg/log"
)

const (
	BlockIOController = cache.BlockIO
)

// blockio encapsulates the runtime state of our block I/O enforcment/controller.
type blockio struct {
	cache cache.Cache // resource manager cache
}

// Make sure blockio implements the control.Controller interface.
var _ control.Controller = &blockio{}

// Our singleton block I/O controller instance.
var singleton *blockio

// Our logger instance.
var log logger.Logger = logger.NewLogger(BlockIOController)

// getBlockIOController returns our singleton block I/O controller instance.
func getBlockIOController() control.Controller {
	if singleton == nil {
		singleton = &blockio{}
	}
	return singleton
}

// Start initializes the controller for enforcing decisions.
func (ctl *blockio) Start(cache cache.Cache, client client.Client) error {
	ctl.cache = cache

	return nil
}

// Stop shuts down the controller.
func (ctl *blockio) Stop() {
}

// PreCreateHook is the block I/O controller pre-create hook.
func (ctl *blockio) PreCreateHook(c cache.Container) error {
	return nil
}

// PreStartHook is the block I/O controller pre-start hook.
func (ctl *blockio) PreStartHook(c cache.Container) error {
	return nil
}

// PostStartHook is the block I/O controller post-start hook.
func (ctl *blockio) PostStartHook(c cache.Container) error {
	// Notes:
	//   Unlike in our PostUpdateHook, we don't filter here by checking
	//   if there are pending block I/O changes (c.HasPending(BlockIOController))
	//   because ATM we want to assign otherwise unassigned containers
	//   based on their QOS class.

	if err := ctl.assign(c, ctl.BlockIOClass(c)); err != nil {
		return err
	}
	c.ClearPending(BlockIOController)
	return nil
}

// PostUpdateHook is the block I/O controller post-update hook.
func (ctl *blockio) PostUpdateHook(c cache.Container) error {
	if !c.HasPending(BlockIOController) {
		return nil
	}
	if err := ctl.assign(c, ctl.BlockIOClass(c)); err != nil {
		return err
	}
	c.ClearPending(BlockIOController)
	return nil
}

// PostStop is the block I/O controller post-stop hook.
func (ctl *blockio) PostStopHook(c cache.Container) error {
	return nil
}

// assign assigns the container to the given block I/O class.
func (ctl *blockio) assign(c cache.Container, class string) error {
	pod, ok := c.GetPod()
	if !ok {
		return blockioError("failed to get Pod for %s", c.PrettyName())
	}

	pids, err := utils.GetProcessInContainer(pod.GetCgroupParentDir(), c.GetID(), nil)
	if err != nil {
		return blockioError("failed to get process list of %s: %v", c.PrettyName(), err)
	}

	for _, pid := range pids {
		log.Info(" *** should assign pid %s to block I/O class %s...", pid, class)
	}

	log.Info("container %s assigned to class %s", c.PrettyName(), class)

	return nil
}

// BlockIOClass determines the effective block I/O class for a container.
func (ctl *blockio) BlockIOClass(c cache.Container) string {
	cclass := c.GetBlockIOClass()
	if cclass == "" {
		cclass = string(c.GetQOSClass())
	}

	bioclass, ok := opt.Classes[cclass]
	if !ok {
		if bioclass, ok = opt.Classes["*"]; !ok {
			bioclass = cclass
		}
	}

	log.Debug("block I/O class for %s (%s): %s", c.PrettyName(), cclass, bioclass)

	return bioclass
}

// configNotify is our runtime configuration notification callback.
func (ctl *blockio) configNotify(event config.Event, source config.Source) error {
	log.Info("configuration updated")
	return nil
}

// blockioError creates an block I/O-controller-specific formatted error message.
func blockioError(format string, args ...interface{}) error {
	return fmt.Errorf("block I/O: "+format, args...)
}

// Register us as a controller.
func init() {
	control.Register(BlockIOController, "Block I/O controller", getBlockIOController())
}
