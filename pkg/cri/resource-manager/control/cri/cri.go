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

package cri

import (
	"fmt"

	"github.com/intel/cri-resource-manager/pkg/cri/client"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/control"
	criapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"

	logger "github.com/intel/cri-resource-manager/pkg/log"
)

const (
	CRIController = cache.CRI
)

// crictl encapsulated the runtime state of our CRI enforcement/controller.
type crictl struct {
	cache  cache.Cache
	client client.Client
}

// Make sure crictl implements the control.Controller interface.
var _ control.Controller = &crictl{}

// Our logger instance.
var log logger.Logger = logger.NewLogger(CRIController)

// Our CRI controller singleton instance.
var singleton *crictl

// getCRIController returns our singleton CRI controller instance.
func getCRIController() control.Controller {
	if singleton == nil {
		singleton = &crictl{}
	}
	return singleton
}

// Start initializes the controller for enforcing decisions.
func (ctl *crictl) Start(cache cache.Cache, client client.Client) error {
	ctl.cache = cache
	ctl.client = client

	return nil
}

// Stop shuts down the controller.
func (ctl *crictl) Stop() {
}

// PreCreateHook is the CRI controller pre-create hook.
func (ctl *crictl) PreCreateHook(c cache.Container) error {
	log.Debug("running pre-create hook for %s...", c.PrettyName())

	if !c.HasPending(CRIController) {
		return nil
	}

	request, ok := c.GetCRIRequest()
	if !ok {
		return criError("pre-create hook: no pending CRI request")
	}
	create, ok := request.(*criapi.CreateContainerRequest)
	if !ok {
		return criError("pre-create hook: pending CRI request of wrong type (%T)", request)
	}

	create.Config.Command = c.GetCommand()
	create.Config.Args = c.GetArgs()
	create.Config.Labels = c.GetLabels()
	create.Config.Annotations = c.GetAnnotations()
	create.Config.Envs = c.GetCRIEnvs()
	create.Config.Mounts = c.GetCRIMounts()
	create.Config.Devices = c.GetCRIDevices()
	create.Config.Linux.Resources = c.GetLinuxResources()

	c.ClearPending(CRIController)

	return nil
}

// PreStartHook is the CRI controller pre-start hook.
func (ctl *crictl) PreStartHook(c cache.Container) error {
	return nil
}

// PostStartHook is the CRI controller post-start hook.
func (ctl *crictl) PostStartHook(c cache.Container) error {
	return nil
}

// PostUpdateHook is the CRI controller post-update hook.
func (ctl *crictl) PostUpdateHook(c cache.Container) error {
	var update *criapi.UpdateContainerResourcesRequest

	log.Debug("running post-update hook for %s...", c.PrettyName())

	if !c.HasPending(CRIController) {
		return nil
	}

	resources := c.GetLinuxResources()
	if resources == nil {
		return nil
	}
	request, ok := c.GetCRIRequest()
	if !ok {
		update = &criapi.UpdateContainerResourcesRequest{
			ContainerId: c.GetID(),
		}
	} else {
		if update, ok = request.(*criapi.UpdateContainerResourcesRequest); !ok {
			return criError("post-update hook: update request of wrong type (%T)", request)
		}
	}
	update.Linux = resources

	c.ClearPending(CRIController)

	return nil
}

// PostStop is the CRI controller post-stop hook.
func (ctl *crictl) PostStopHook(c cache.Container) error {
	return nil
}

// criError creates an CRI-controller-specific formatted error message.
func criError(format string, args ...interface{}) error {
	return fmt.Errorf("cri: "+format, args...)
}

// Register us as a controller.
func init() {
	control.Register(CRIController, "CRI controller", getCRIController())
}
