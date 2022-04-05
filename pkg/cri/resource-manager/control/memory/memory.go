// Copyright 2019-2020 Intel Corporation. All Rights Reserved.
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

package memory

import (
	"fmt"
	"os"
	"strconv"

	"github.com/intel/cri-resource-manager/pkg/cgroups"
	"github.com/intel/cri-resource-manager/pkg/cri/client"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/control"
	logger "github.com/intel/cri-resource-manager/pkg/log"
)

const (
	// MemoryController is the name of the memory controller.
	MemoryController = cache.Memory

	// memoryCgroupPath is the path to the root of the memory cgroup.
	memoryCgroupPath = "/sys/fs/cgroup/memory"
	// toptierSoftLimitControl is the memory cgroup entry to set top tier soft limit.
	toptierSoftLimitControl = "memory.toptier_soft_limit_in_bytes"
)

// memctl encapsulates the runtime state of our memory enforcement/controller.
type memctl struct {
	cache    cache.Cache // resource manager cache
	disabled bool        // true, if kernel lacks the necessary cgroup controls
}

// Our logger instance.
var log logger.Logger = logger.NewLogger(MemoryController)

// Our singleton memory controller instance.
var singleton *memctl

// getMemoryController returns our singleton memory controller instance.
func getMemoryController() *memctl {
	if singleton == nil {
		singleton = &memctl{}
	}
	return singleton
}

// Start initializes the controller for enforcing decisions.
func (ctl *memctl) Start(cache cache.Cache, client client.Client) error {
	// Let's keep this off for now so we can exercise this without a patched kernel...
	/*if !ctl.checkToptierLimitSupport() {
		return memctlError("cgroup top tier memory limit control not available")
	}*/
	ctl.cache = cache
	return nil
}

// Stop shuts down the controller.
func (ctl *memctl) Stop() {
}

// PreCreateHook is the memory controller pre-create hook.
func (ctl *memctl) PreCreateHook(c cache.Container) error {
	return nil
}

// PreStartHook is the memory controller pre-start hook.
func (ctl *memctl) PreStartHook(c cache.Container) error {
	return nil
}

// PostStartHook is the memory controller post-start hook.
func (ctl *memctl) PostStartHook(c cache.Container) error {
	if !c.HasPending(MemoryController) {
		return nil
	}

	if err := ctl.setToptierLimit(c); err != nil {
		return err
	}

	c.ClearPending(MemoryController)

	return nil
}

// PostUpdateHook is the memory controller post-update hook.
func (ctl *memctl) PostUpdateHook(c cache.Container) error {
	if !c.HasPending(MemoryController) {
		return nil
	}

	if err := ctl.setToptierLimit(c); err != nil {
		return err
	}

	c.ClearPending(MemoryController)

	return nil
}

// PostStop is the memory controller post-stop hook.
func (ctl *memctl) PostStopHook(c cache.Container) error {
	return nil
}

func (ctl *memctl) UpdateConfig(cache cache.Cache) error {
	return nil
}

// Check if memory cgroup controller supports top tier soft limits.
func (ctl *memctl) checkToptierLimitSupport() bool {
	_, err := os.Stat(memoryCgroupPath + "/" + toptierSoftLimitControl)
	if err != nil && os.IsNotExist(err) {
		log.Warn("cgroup top tier memory limit control not available")
		ctl.disabled = true
	}
	return !ctl.disabled
}

// setToptierLimit sets the top tier memory (soft) limit for the container.
func (ctl *memctl) setToptierLimit(c cache.Container) error {
	dir := c.GetCgroupDir()
	if dir == "" {
		return memctlError("%q: failed to determine cgroup directory",
			c.PrettyName())
	}

	limit := strconv.FormatInt(c.GetToptierLimit(), 10)
	group := cgroups.Memory.Group(dir)
	entry := toptierSoftLimitControl

	if err := group.Write(entry, limit+"\n"); err != nil {
		return err
	}

	log.Info("%q: memory toptier soft limit set to %v", c.PrettyName(), limit)

	return nil
}

// memctlError creates a memory I/O-controller-specific formatted error message.
func memctlError(format string, args ...interface{}) error {
	return fmt.Errorf("memory: "+format, args...)
}

// init registers this controller.
func init() {
	control.Register(MemoryController, "memory toptier controller", getMemoryController())
}
