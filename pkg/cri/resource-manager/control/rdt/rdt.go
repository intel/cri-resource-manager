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
	"github.com/intel/cri-resource-manager/pkg/utils"
	"github.com/intel/goresctrl/pkg/rdt"

	pkgcfg "github.com/intel/cri-resource-manager/pkg/config"
	logger "github.com/intel/cri-resource-manager/pkg/log"
	"github.com/intel/cri-resource-manager/pkg/metrics"
)

const (
	// ConfigModuleName is the configuration section for RDT
	ConfigModuleName = "rdt"

	// RDTController is the name of the RDT controller.
	RDTController = cache.RDT

	resctrlGroupPrefix = "cri-resmgr."
)

// rdtctl encapsulates the runtime state of our RTD enforcement/controller.
type rdtctl struct {
	cache cache.Cache // resource manager cache
	idle  bool        // true if we run without any classes configured
	opt   *config
}

type config struct {
	rdt.Config

	Options struct {
		rdt.Options

		MonitoringDisabled bool `json:"monitoringDisabled"`
	} `json:"options"`
}

// Our logger instance.
var log logger.Logger = logger.NewLogger(RDTController)

// our RDT controller singleton instance.
var singleton *rdtctl

// getRDTController returns our singleton RDT controller instance.
func getRDTController() *rdtctl {
	if singleton == nil {
		singleton = &rdtctl{}
		singleton.opt = singleton.defaultOptions().(*config)
	}
	return singleton
}

// Start initializes the controller for enforcing decisions.
func (ctl *rdtctl) Start(cache cache.Cache, client client.Client) error {
	if err := rdt.Initialize(resctrlGroupPrefix); err != nil {
		return rdtError("failed to initialize RDT controls: %v", err)
	}

	if err := rdt.SetConfig(&ctl.opt.Config); err != nil {
		// Just print an error. A config update later on may be valid.
		log.Error("failed apply initial configuration: %v", err)
	}

	ctl.idle = ctl.checkIdle()

	rdt.RegisterCustomPrometheusLabels("pod_name", "container_name")
	err := metrics.RegisterCollector("rdt", rdt.NewCollector)
	if err != nil {
		log.Error("failed register rdt collector: %v", err)
	}

	ctl.cache = cache

	ctl.assignAll()

	pkgcfg.GetModule(ConfigModuleName).AddNotify(getRDTController().configNotify)

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
	if !c.HasPending(RDTController) {
		return nil
	}

	if err := ctl.assign(c); err != nil {
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

	if err := ctl.assign(c); err != nil {
		return err
	}

	c.ClearPending(RDTController)

	return nil
}

// PostStop is the RDT controller post-stop hook.
func (ctl *rdtctl) PostStopHook(c cache.Container) error {
	if err := ctl.stopMonitor(c); err != nil {
		return rdtError("%q: failed to remove monitoring group: %v", c.PrettyName(), err)
	}
	return nil
}

// checkIdle checks if we run without any classes confiured
func (ctl *rdtctl) checkIdle() bool {
	return len(rdt.GetClasses()) <= 1
}

// assign assigns all processes/threads in a container to an RDT class.
func (ctl *rdtctl) assign(c cache.Container) error {
	class := c.GetRDTClass()
	if class == "" {
		class = rdt.RootClassName
	}

	if ctl.idle && cache.IsPodQOSClassName(class) {
		return nil
	}

	cls, ok := rdt.GetClass(class)
	if !ok {
		return rdtError("%q: unknown RDT class %q", c.PrettyName(), class)
	}

	pod, ok := c.GetPod()
	if !ok {
		return rdtError("%q: failed to get pod", c.PrettyName())
	}

	pids, err := utils.GetTasksInContainer(pod.GetCgroupParentDir(), c.GetID())
	if err != nil {
		return rdtError("%q: failed to get process list: %v", c.PrettyName(), err)
	}

	if err := cls.AddPids(pids...); err != nil {
		return rdtError("%q: failed to assign to class %q: %v", c.PrettyName(), class, err)
	}

	pretty := c.PrettyName()
	if _, ok := cls.GetMonGroup(pretty); !ok || ctl.opt.Options.MonitoringDisabled {
		ctl.stopMonitor(c)
	}

	if !ctl.opt.Options.MonitoringDisabled {
		pname, name, id := pod.GetName(), c.GetName(), c.GetID()
		if err := ctl.monitor(cls, pname, name, id, pretty, pids); err != nil {
			return err
		}
	}
	log.Info("%q: assigned to class %q", pretty, class)

	return nil
}

// monitor starts monitoring a container.
func (ctl *rdtctl) monitor(cls rdt.CtrlGroup, pod, name, id, pretty string, pids []string) error {
	if !rdt.MonSupported() {
		return nil
	}

	annotations := map[string]string{"pod_name": pod, "container_name": name}
	if mg, err := cls.CreateMonGroup(id, annotations); err != nil {
		log.Warn("%q: failed to create monitoring group: %v", pretty, err)
	} else {
		if err := mg.AddPids(pids...); err != nil {
			return rdtError("%q: failed to assign to monitoring group %q: %v",
				pretty, cls.Name()+"/"+mg.Name(), err)
		}
		log.Info("%q: assigned to monitoring group %q", pretty, cls.Name()+"/"+mg.Name())
	}
	return nil
}

// stopMonitor stops monitoring a container.
func (ctl *rdtctl) stopMonitor(c cache.Container) error {
	name := c.PrettyName()
	for _, cls := range rdt.GetClasses() {
		if mg, ok := cls.GetMonGroup(name); ok {
			if err := cls.DeleteMonGroup(name); err != nil {
				return err
			}
			log.Info("%q: removed monitoring group %q",
				c.PrettyName(), cls.Name()+"/"+mg.Name())
		}
	}
	return nil
}

func (ctl *rdtctl) assignAll() {
	for _, c := range ctl.cache.GetContainers() {
		if err := ctl.assign(c); err != nil {
			log.Warn("failed to assign rdt class of %q: %v", c.PrettyName(), err)
		}
	}
}

// configNotify is our runtime configuration notification callback.
func (ctl *rdtctl) configNotify(event pkgcfg.Event, source pkgcfg.Source) error {
	log.Info("configuration update, applying new config")
	log.Debug("rdt monitoring %s", map[bool]string{true: "disabled", false: "enabled"}[ctl.opt.Options.MonitoringDisabled])

	ctl.idle = ctl.checkIdle()

	// Copy goresctrl specific part from our extended options
	ctl.opt.Config.Options = ctl.opt.Options.Options
	if err := rdt.SetConfig(&ctl.opt.Config); err != nil {
		return err
	}

	ctl.assignAll()

	return nil
}

func (ctl *rdtctl) defaultOptions() interface{} {
	return &config{}
}

// GetClasses returns all available RDT classes
func GetClasses() []rdt.CtrlGroup {
	return rdt.GetClasses()
}

// rdtError creates an RDT-controller-specific formatted error message.
func rdtError(format string, args ...interface{}) error {
	return fmt.Errorf("rdt: "+format, args...)
}

// Register us as a controller.
func init() {
	control.Register(RDTController, "RDT controller", getRDTController())
	pkgcfg.Register(ConfigModuleName, "RDT control", getRDTController().opt, getRDTController().defaultOptions)
}
