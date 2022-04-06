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
	cache  cache.Cache // resource manager cache
	config *config
}

type config struct {
	Classes map[string]Class `json:"classes"`
}

type Class struct {
	MinFreq                     uint `json:"minFreq"`
	MaxFreq                     uint `json:"maxFreq"`
	EnergyPerformancePreference uint `json:"energyPerformancePreference"`
}

// cpuClassAllocations contains the information about how cpus are assigned to
// classes
type cpuClassAssignments map[string]utils.IDSet

const (
	cacheKeyCPUAssignments = "CPUClassAssignments"
)

var log logger.Logger = logger.NewLogger(CPUController)

// Ccontroller singleton instance.
var singleton *cpuctl

// GetClasses returns all available CPU classes.
func GetClasses() map[string]Class {
	return getCPUController().config.getClasses()
}

// Assign assigns a set of cpus to a class.
func Assign(class string, cpus ...int) error {
	// NOTE: no locking implemented anywhere around -> we don't expect multiple parallel callers
	return getCPUController().assign(class, cpus...)
}

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
	log.Debug("retrieved cpu class assignments from cache:\n%s", utils.DumpJSON(*ctl.getClassAssignments()))

	if err := ctl.configure(); err != nil {
		// Just print an error. A config update later on may be valid.
		log.Error("failed apply initial configuration: %v", err)
	}

	// TODO: We probably could just remove this and the hooks if they are not used
	pkgcfg.GetModule(ConfigModuleName).AddNotify(getCPUController().configNotify)

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

// UpdateHWConfig handler for the CPU controller.
func (ctl *cpuctl) UpdateConfig(cache cache.Cache) error {
	return cache.TraversePendingConfig(Assign)
}

// assign assigns a cpuset to a class
func (ctl *cpuctl) assign(class string, cpus ...int) error {
	if _, ok := ctl.config.Classes[class]; !ok {
		return fmt.Errorf("non-existent cpu class %q", class)
	}

	if err := utils.SetCPUsScalingMinFreq(cpus, (int)(ctl.config.Classes[class].MinFreq)); err != nil {
		return fmt.Errorf("Cannot set min freq %d: %w", ctl.config.Classes[class].MinFreq, err)
	}

	if err := utils.SetCPUsScalingMaxFreq(cpus, (int)(ctl.config.Classes[class].MaxFreq)); err != nil {
		return fmt.Errorf("Cannot set max freq %d: %w", ctl.config.Classes[class].MaxFreq, err)
	}

	// Store the class assignment. Assign cpus to a class and remove them from
	// other classes
	assignments := *ctl.getClassAssignments()

	if this, ok := assignments[class]; !ok {
		assignments[class] = utils.NewIDSetFromIntSlice(cpus...)
	} else {
		this.Add(cpus...)
	}

	for k, v := range assignments {
		if k != class {
			v.Del(cpus...)

			// Don't store empty classes, serves as a garbage collector, too
			if v.Size() == 0 {
				delete(assignments, k)
			}
		}
	}

	ctl.setClassAssignments(&assignments)

	return nil
}

func (ctl *cpuctl) configure() error {
	// Re-configure CPUs that are assigned to some known class
	assignments := *ctl.getClassAssignments()

	for class, cpus := range assignments {
		if _, ok := ctl.config.Classes[class]; ok {
			// TODO: re-configure cpus (sysfs) according to new class parameters

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
	log.Info("configuration update, applying new config")
	return ctl.configure()
}

func (ctl *cpuctl) defaultOptions() interface{} {
	return &config{}
}

// Get the state of CPU class assignments from cache
func (ctl *cpuctl) getClassAssignments() *cpuClassAssignments {
	c := &cpuClassAssignments{}

	if !ctl.cache.GetPolicyEntry(cacheKeyCPUAssignments, c) {
		log.Error("no cached state of CPU class assignments found")
	}

	return c
}

// Save the state of CPU class assignments in cache
func (ctl *cpuctl) setClassAssignments(c *cpuClassAssignments) {
	ctl.cache.SetPolicyEntry(cacheKeyCPUAssignments, cache.Cachable(c))
}

func (c *config) getClasses() map[string]Class {
	ret := make(map[string]Class, len(c.Classes))
	for k, v := range c.Classes {
		ret[k] = v
	}
	return ret
}

// Set the value of cached cpuClassAssignments
func (c *cpuClassAssignments) Set(value interface{}) {
	switch value.(type) {
	case cpuClassAssignments:
		*c = value.(cpuClassAssignments)
	case *cpuClassAssignments:
		cp := value.(*cpuClassAssignments)
		*c = *cp
	}
}

// Get cached cpuClassAssignments
func (c *cpuClassAssignments) Get() interface{} {
	return *c
}

// Register us as a controller.
func init() {
	control.Register(CPUController, "CPU controller", getCPUController())
	pkgcfg.Register(ConfigModuleName, "CPU control", getCPUController().config, getCPUController().defaultOptions)
}
