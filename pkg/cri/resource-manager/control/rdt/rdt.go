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

package rdt

import (
	"fmt"

	"github.com/intel/cri-resource-manager/pkg/cri/client"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/control"
	"github.com/intel/cri-resource-manager/pkg/rdt"
	"github.com/intel/cri-resource-manager/pkg/utils"

	"github.com/intel/cri-resource-manager/pkg/config"
	logger "github.com/intel/cri-resource-manager/pkg/log"
)

const (
	// RDTController is the name of the RDT controller.
	RDTController = cache.RDT
)

// rdtctl encapsulates the runtime state of our RTD enforcement/controller.
type rdtctl struct {
	cache cache.Cache // resource manager cache
}

// Our logger instance.
var log logger.Logger = logger.NewLogger(RDTController)

// our RDT controller singleton instance.
var singleton *rdtctl

// getRDTController returns our singleton RDT controller instance.
func getRDTController() control.Controller {
	if singleton == nil {
		singleton = &rdtctl{}
	}
	return singleton
}

// Start initializes the controller for enforcing decisions.
func (ctl *rdtctl) Start(cache cache.Cache, client client.Client) error {
	if err := rdt.Initialize(); err != nil {
		return rdtError("failed to initialize RDT controls: %v", err)
	}

	ctl.cache = cache

	return nil
}

// Stop shuts down the controller.
func (ctl *rdtctl) Stop() {
}

// PreCreateHook is the RDT controller pre-create hook.
func (ctl *rdtctl) PreCreateHook(c cache.Container) error {
	return nil
}

// PreStartHook is the RDT controller pre-start hook.
func (ctl *rdtctl) PreStartHook(c cache.Container) error {
	return nil
}

// PostStartHook is the RDT controller post-start hook.
func (ctl *rdtctl) PostStartHook(c cache.Container) error {
	// Notes:
	//   Unlike in our PostUpdateHook, we don't bail out here if
	//   there are no pending RDT changes for the container. We
	//   might be configured to fall back to assign RDT class
	//   based on pod/container QoS class in which case there is
	//   no pending marker on the container.
	if err := ctl.assign(c, ctl.RDTClass(c)); err != nil {
		return err
	}
	c.ClearPending(RDTController)
	return nil
}

// PostUpdateHook is the RDT controller post-update hook.
func (ctl *rdtctl) PostUpdateHook(c cache.Container) error {
	if !c.HasPending(RDTController) {
		return nil
	}
	if err := ctl.assign(c, ctl.RDTClass(c)); err != nil {
		return err
	}
	c.ClearPending(RDTController)
	return nil
}

// PostStop is the RDT controller post-stop hook.
func (ctl *rdtctl) PostStopHook(c cache.Container) error {
	return nil
}

// assign assigns the container to the given RDT class.
func (ctl *rdtctl) assign(c cache.Container, class string) error {
	if class == "" {
		return nil
	}

	pod, ok := c.GetPod()
	if !ok {
		return rdtError("failed to get pod of container %s", c.PrettyName())
	}

	pids, err := utils.GetProcessInContainer(pod.GetCgroupParentDir(), c.GetID())
	if err != nil {
		return rdtError("failed to get process list for container %s: %v", c.PrettyName(), err)
	}

	if err := rdt.SetProcessClass(class, pids...); err != nil {
		return rdtError("failed assign container %s to class %s: %v", c.PrettyName(), class, err)
	}

	log.Info("container %s assigned to class %s", c.PrettyName(), class)

	return nil
}

// RDTClass determines the effective RDT class for a container.
func (ctl *rdtctl) RDTClass(c cache.Container) string {
	cclass := c.GetRDTClass()
	if cclass == "" {
		cclass = string(c.GetQOSClass())
	}
	rdtclass, ok := opt.Classes[cclass]
	if !ok {
		if rdtclass, ok = opt.Classes["*"]; !ok {
			rdtclass = cclass
		}
	}

	log.Debug("RDT class for %s (%s): %q", c.PrettyName(), cclass, rdtclass)

	return rdtclass
}

// configNotify is our runtime configuration notification callback.
func (ctl *rdtctl) configNotify(event config.Event, source config.Source) error {
	log.Info("configuration updated")
	return nil
}

// rdtError creates an RDT-controller-specific formatted error message.
func rdtError(format string, args ...interface{}) error {
	return fmt.Errorf("rdt: "+format, args...)
}

// Register us as a controller.
func init() {
	control.Register(RDTController, "RDT controller", getRDTController())
}
