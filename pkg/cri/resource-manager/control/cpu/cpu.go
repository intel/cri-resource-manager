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

package cpu

import (
	"fmt"

	pkgcfg "github.com/intel/cri-resource-manager/pkg/config"
	"github.com/intel/cri-resource-manager/pkg/cri/client"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/control"
	logger "github.com/intel/cri-resource-manager/pkg/log"
	"github.com/intel/goresctrl/pkg/utils"
)

const (
	// ConfigModuleName is the configuration section for the CPU controller.
	ConfigModuleName = "cpu"

	// CPUController is the name of the CPU controller.
	CPUController = cache.CPU
)

// cpuctl encapsulates the runtime state of our CPU enforcement/controller.
type cpuctl struct {
	cache   cache.Cache // resource manager cache
	config  *config
	started bool
}

type config struct {
	Classes map[string]Class `json:"classes"`
}

type Class struct {
	MinFreq                     uint `json:"minFreq"`
	MaxFreq                     uint `json:"maxFreq"`
	EnergyPerformancePreference uint `json:"energyPerformancePreference"`
}

var log logger.Logger = logger.NewLogger(CPUController)

// Ccontroller singleton instance.
var singleton *cpuctl

// getCPUController returns the (singleton) CPU controller instance.
func getCPUController() *cpuctl {
	if singleton == nil {
		singleton = &cpuctl{}
		singleton.config = singleton.defaultOptions().(*config)
	}
	return singleton
}

// Start initializes the controller for enforcing decisions.
func (ctl *cpuctl) Start(cache cache.Cache, client client.Client) error {
	ctl.cache = cache

	// DEBUG: dump the class assignments we have stored in the cache
	log.Debug("retrieved cpu class assignments from cache:\n%s", utils.DumpJSON(getClassAssignments(ctl.cache)))

	if err := ctl.configure(); err != nil {
		// Just print an error. A config update later on may be valid.
		log.Error("failed apply initial configuration: %v", err)
	}

	// TODO: We probably could just remove this and the hooks if they are not used
	pkgcfg.GetModule(ConfigModuleName).AddNotify(getCPUController().configNotify)

	ctl.started = true

	return nil
}

// Stop shuts down the controller.
func (ctl *cpuctl) Stop() {
}

// PreCreateHook handler for the CPU controller.
func (ctl *cpuctl) PreCreateHook(c cache.Container) error {
	return nil
}

// PreStartHook handler for the CPU controller.
func (ctl *cpuctl) PreStartHook(c cache.Container) error {
	return nil
}

// PostStartHook handler for the CPU controller.
func (ctl *cpuctl) PostStartHook(c cache.Container) error {
	return nil
}

// PostUpdateHook handler for the CPU controller.
func (ctl *cpuctl) PostUpdateHook(c cache.Container) error {
	return nil
}

// PostStopHook handler for the CPU controller.
func (ctl *cpuctl) PostStopHook(c cache.Container) error {
	return nil
}

// enforce enforces a class-specific configuration to a cpuset
func (ctl *cpuctl) enforce(class string, cpus ...int) error {
	if _, ok := ctl.config.Classes[class]; !ok {
		return fmt.Errorf("non-existent cpu class %q", class)
	}

	//TODO: configure cpus (sysfs)
	log.Debug("enforcing cpu class %q on %v", class, cpus)

	return nil
}

func (ctl *cpuctl) configure() error {
	// Re-configure CPUs that are assigned to some known class
	assignments := *getClassAssignments(ctl.cache)

	for class, cpus := range assignments {
		if _, ok := ctl.config.Classes[class]; ok {
			// Re-configure cpus (sysfs) according to new class parameters
			if err := ctl.enforce(class, cpus.SortedMembers()...); err != nil {
				log.Error("cpu class enforcement on re-configure failed: %v", err)
			}

		} else {
			// TODO: what should we really do with classes that do not exist in
			// the configuration anymore? Now we remember the CPUs assigned to
			// them. A further config update might re-introduce the class in
			// which case the CPUs will be reconfigured.
			log.Warn("class %q with cpus %v missing from the configuration", class, cpus)
		}
	}

	log.Debug("cpu controller configured")

	return nil
}

// Callback for runtime configuration notifications.
func (ctl *cpuctl) configNotify(event pkgcfg.Event, source pkgcfg.Source) error {
	if !ctl.started {
		// We don't want to configure until the controller has been fully
		// started and initialized. We will configure on Start(), anyway.
		return nil
	}

	log.Info("configuration update, applying new config")
	return ctl.configure()
}

func (ctl *cpuctl) defaultOptions() interface{} {
	return &config{}
}

func (c *config) getClasses() map[string]Class {
	ret := make(map[string]Class, len(c.Classes))
	for k, v := range c.Classes {
		ret[k] = v
	}
	return ret
}

// Register us as a controller.
func init() {
	control.Register(CPUController, "CPU controller", getCPUController())
	pkgcfg.Register(ConfigModuleName, "CPU control", getCPUController().config, getCPUController().defaultOptions)
}
