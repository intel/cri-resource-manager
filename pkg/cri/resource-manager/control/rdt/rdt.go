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

	corev1 "k8s.io/api/core/v1"

	pkgcfg "github.com/intel/cri-resource-manager/pkg/config"
	"github.com/intel/cri-resource-manager/pkg/cri/client"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/control"
	logger "github.com/intel/cri-resource-manager/pkg/log"
	"github.com/intel/cri-resource-manager/pkg/metrics"
	"github.com/intel/cri-resource-manager/pkg/utils"
	"github.com/intel/goresctrl/pkg/rdt"
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
	cache        cache.Cache   // resource manager cache
	noQoSClasses bool          // true if we run without any classes configured
	mode         OperatingMode // track the mode here to capture mode changes
	opt          *config
}

type config struct {
	rdt.Config

	Options struct {
		rdt.Options

		Mode               OperatingMode `json:"mode"`
		MonitoringDisabled bool          `json:"monitoringDisabled"`
	} `json:"options"`
}

type OperatingMode string

const (
	OperatingModeDisabled  OperatingMode = "Disabled"
	OperatingModeDiscovery OperatingMode = "Discovery"
	OperatingModeFull      OperatingMode = "Full"
)

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

	ctl.cache = cache

	if err := ctl.configure(); err != nil {
		// Just print an error. A config update later on may be valid.
		log.Error("failed apply initial configuration: %v", err)
	}

	rdt.RegisterCustomPrometheusLabels("pod_name", "container_name")
	err := metrics.RegisterCollector("rdt", rdt.NewCollector)
	if err != nil {
		log.Error("failed register rdt collector: %v", err)
	}

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

// assign assigns all processes/threads in a container to the correct class
func (ctl *rdtctl) assign(c cache.Container) error {
	if ctl.opt.Options.Mode == OperatingModeDisabled {
		return nil
	}

	class := c.GetRDTClass()
	switch class {
	case "":
		class = rdt.RootClassName
	case cache.RDTClassPodQoS:
		if ctl.noQoSClasses {
			class = rdt.RootClassName
		} else {
			class = string(c.GetQOSClass())
		}
	}

	err := ctl.assignClass(c, class)
	if err != nil && class != rdt.RootClassName {
		log.Warn("%v; falling back to system root class", err)
		return ctl.assignClass(c, rdt.RootClassName)
	}
	return err
}

// assignClass assigns all processes/threads in a container to the specified class
func (ctl *rdtctl) assignClass(c cache.Container, class string) error {
	cls, ok := rdt.GetClass(class)
	if !ok {
		return rdtError("%q: unknown RDT class %q", c.PrettyName(), class)
	}

	pod, ok := c.GetPod()
	if !ok {
		return rdtError("%q: failed to get pod", c.PrettyName())
	}

	pids, err := utils.GetTasksInContainer(pod.GetCgroupParentDir(), c.GetPodID(), c.GetID())
	if err != nil {
		return rdtError("%q: failed to get process list: %v", c.PrettyName(), err)
	}

	if err := cls.AddPids(pids...); err != nil {
		return rdtError("%q: failed to assign to class %q: %v", c.PrettyName(), class, err)
	}

	pretty := c.PrettyName()
	if _, ok := cls.GetMonGroup(pretty); !ok || ctl.monitoringDisabled() {
		ctl.stopMonitor(c)
	}

	if !ctl.monitoringDisabled() {
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

// stopMonitorAll removes all monitoring groups
func (ctl *rdtctl) stopMonitorAll() error {
	for _, cls := range rdt.GetClasses() {
		if err := cls.DeleteMonGroups(); err != nil {
			return err
		}
	}
	return nil
}

func (ctl *rdtctl) assignAll(forceClass string) {
	// Assign all containers
	for _, c := range ctl.cache.GetContainers() {
		var err error
		if forceClass != "" {
			err = ctl.assignClass(c, forceClass)
		} else {
			err = ctl.assign(c)
		}
		if err != nil {
			log.Warn("failed to assign rdt class of %q: %v", c.PrettyName(), err)
		}
	}

}

func (ctl *rdtctl) monitoringDisabled() bool {
	return ctl.mode == OperatingModeDisabled || ctl.opt.Options.MonitoringDisabled
}

func (ctl *rdtctl) configure() error {
	// Apply RDT configuration, depending on the operating mode
	switch ctl.opt.Options.Mode {
	case OperatingModeDisabled:
		if ctl.mode != ctl.opt.Options.Mode {
			ctl.stopMonitorAll()
			// Drop all cri-resctrl specific groups by applying an empty config
			if err := rdt.SetConfig(&rdt.Config{}, true); err != nil {
				return rdtError("failed apply empty rdt config: %v", err)
			}
			ctl.noQoSClasses = true
			ctl.mode = ctl.opt.Options.Mode
			ctl.assignAll(rdt.RootClassName)
		}
	case OperatingModeDiscovery:
		if ctl.mode != ctl.opt.Options.Mode {
			ctl.stopMonitorAll()
			// Drop all cri-resctrl specific groups by applying an empty config
			if err := rdt.SetConfig(&rdt.Config{}, true); err != nil {
				return rdtError("failed apply empty rdt config: %v", err)
			}
		}
		// Run Initialize with empty prefix to discover existing resctrl groups
		if err := rdt.DiscoverClasses(""); err != nil {
			return rdtError("failed to discover classes from fs: %v", err)
		}

		// Set idle to true if none of the Pod QoS class equivalents exist
		ctl.noQoSClasses = true
		cs := []corev1.PodQOSClass{corev1.PodQOSBestEffort, corev1.PodQOSBurstable, corev1.PodQOSGuaranteed}
		for c := range cs {
			if _, ok := rdt.GetClass(string(c)); ok {
				ctl.noQoSClasses = false
				break
			}
		}

		ctl.mode = ctl.opt.Options.Mode
		ctl.assignAll("")
	case OperatingModeFull:
		if ctl.mode != ctl.opt.Options.Mode {
			ctl.stopMonitorAll()
		}

		// Copy goresctrl specific part from our extended options
		ctl.opt.Config.Options = ctl.opt.Options.Options
		if err := rdt.SetConfig(&ctl.opt.Config, true); err != nil {
			return err
		}
		ctl.noQoSClasses = len(rdt.GetClasses()) <= 1
		ctl.mode = ctl.opt.Options.Mode
		ctl.assignAll("")
	default:
		return rdtError("invalid mode %q", ctl.opt.Options.Mode)
	}

	log.Debug("rdt controller operating mode set to %q", ctl.mode)

	if ctl.opt.Options.Mode != OperatingModeDisabled {
		log.Debug("rdt monitoring %s", map[bool]string{true: "disabled", false: "enabled"}[ctl.monitoringDisabled()])
	}

	return nil
}

// configNotify is our runtime configuration notification callback.
func (ctl *rdtctl) configNotify(event pkgcfg.Event, source pkgcfg.Source) error {
	log.Info("configuration update, applying new config")
	return ctl.configure()
}

func (ctl *rdtctl) defaultOptions() interface{} {
	c := &config{}
	c.Options.Mode = OperatingModeFull
	return c
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
