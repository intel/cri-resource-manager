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

package config

import (
	"github.com/ghodss/yaml"
	"reflect"
	"strings"
)

const (
	// MainModule is the default parent for all configuration.
	MainModule = "main"
)

// GetConfigFn is used to query a module for its default configuration.
type GetConfigFn func() interface{}

// NotifyFn is used to notify a module about configuration changes.
type NotifyFn func(Event, Source) error

// Event describes what triggered an invocation of a configuration notification callback.
type Event string

const (
	// UpdateEvent corresponds to a normal configuration udpate.
	UpdateEvent = "update"
	// RevertEvent corresponds to a configuration rollback in case of errors.
	RevertEvent = "rollback"
)

// Source describes where configuration is originated from.
type Source string

const (
	// ConfigFile is a YAML/JSON file configuration source.
	ConfigFile Source = "configuration file"
	// ConfigExternal is an external configuration source, for instance a node agent.
	ConfigExternal Source = "external configuration"
	// ConfigBackup is a backup of the previous configuration.
	ConfigBackup Source = "backed up configuration"
)

// Module is a logical unit of configuration, declared using Declare().
type Module struct {
	path        string             // fully qualified path in dotted notation, parent.name
	description string             // verbose module description
	help        string             // verbose description/help about this module
	ptr         interface{}        // pointer to module configuration data
	parent      *Module            // parent module
	name        string             // name relative to parent, last part of path
	children    map[string]*Module // modules nested under this module
	getdefault  GetConfigFn        // getter for default configuration
	notifiers   []NotifyFn         // update notification callbacks
	noValidate  bool               // omit data validation
}

// main is the root of our configuration.
var main = &Module{
	path:     MainModule,
	name:     MainModule,
	children: make(map[string]*Module),
}

// GetConfig returns the current configuration.
func GetConfig() (Data, error) {
	return main.getconfig()
}

// SetConfig updates the configuration using data from an external source.
func SetConfig(cfg map[string]string) error {
	data, err := DataFromStringMap(cfg)
	if err != nil {
		return configError("failed to update configuration: %v", err)
	}
	return setconfig(data, ConfigExternal)
}

// SetConfigFromFile updates the configuration from the given file.
func SetConfigFromFile(path string) error {
	data, err := DataFromFile(path)
	if err != nil {
		return configError("failed to apply configuration from file: %v", err)
	}
	return setconfig(data, ConfigFile)
}

// GetModule looks up the module for the given path, implicitly creating it if necessary.
func GetModule(path string) *Module {
	return lookup(path)
}

// AddNotify attaches the given update notification callback to the module.
func (m *Module) AddNotify(fn NotifyFn) error {
	return WithNotify(fn).apply(m)
}

// Register registers a unit of configuration data to be handled by this package.
func Register(path, description string, ptr interface{}, getfn GetConfigFn, opts ...Option) *Module {
	m := lookup(path)

	if !m.isImplicit() {
		log.Fatal("module %s: conflicting module with same path already declared (%s)",
			path, m.description)
	}

	m.setDescription(description)
	m.ptr = ptr
	m.getdefault = getfn

	m.check()

	for _, opt := range opts {
		opt.apply(m)
	}

	return m
}

// setconfig updates the configuration, notifies all modules, and does a rollback if necessary.
func setconfig(data Data, source Source) error {
	snapshot, err := main.getconfig()
	if err != nil {
		return configError("pre-update configuration snapshot failed: %v", err)
	}

	log.Info("validating configuration...")
	err = main.validate(data)
	if err != nil {
		return err
	}

	log.Info("applying configuration...")
	err = main.configure(data, false)
	if err != nil {
		revertconfig(snapshot, false)
		return err
	}

	log.Info("activating configuration...")
	err = main.notify(UpdateEvent, source)
	if err != nil {
		log.Error("configuration rejected: %v", err)
		revertconfig(snapshot, true)
		return err
	}

	return nil
}

// revertconfig reverts configuration using a previously taken snapshot
func revertconfig(snapshot Data, notify bool) {
	err := main.configure(snapshot, true)
	if err != nil {
		log.Error("failed to rever configuration: %v", err)
	}

	if !notify {
		return
	}

	err = main.notify(RevertEvent, ConfigBackup)
	if err != nil {
		log.Error("reverted configuration rejected: %v", err)
	}
}

// getconfig returns the configuration for the given module and its submodules.
func (m *Module) getconfig() (Data, error) {
	var mcfg, ccfg Data
	var err error

	if m.isImplicit() {
		mcfg = make(Data)
	} else {
		mcfg, err = DataFromObject(m.ptr)
		if err != nil {
			return nil, configError("module %s: failed to get confguration: %v",
				m.path, err)
		}
	}

	for name, child := range m.children {
		ccfg, err = child.getconfig()
		if err != nil {
			return nil, configError("module %s: failed to get child configuration for %s: %v",
				m.path, child.path, err)
		}
		mcfg[name] = ccfg
	}

	return mcfg, nil
}

// isImplict returns true if the module has not been explicitly declared.
func (m *Module) isImplicit() bool {
	return m.description == ""
}

// hasChild checks if the module has a child with the given name.
func (m *Module) hasChild(name string) bool {
	_, ok := m.children[name]
	return ok
}

// configure reconfigures the given module and its submodules with the provided data.
func (m *Module) configure(data Data, force bool) error {
	log.Debug("module %s: reconfiguring...", m.path)

	modcfg, subcfg := data.split(m.hasChild)
	if err := m.apply(modcfg); err != nil {
		if !force {
			return err
		}
		log.Error("%v", err)
	}

	for name, child := range m.children {
		childcfg, err := subcfg.pick(name, true)
		if err != nil {
			err = configError("module %s: failed to pick configuration: %v", child.path, err)
			if !force {
				return err
			}
			log.Error("%v", err)
		}
		err = child.configure(childcfg, force)
		if err != nil {
			if !force {
				return err
			}
			log.Error("%v", err)
		}
	}

	return nil
}

// apply applies the given module-local configuration to the module.
func (m *Module) apply(cfg Data) error {
	if m.isImplicit() {
		return nil
	}

	log.Debug("module %s: applying module configuration...", m.path)

	if len(cfg) == 0 {
		defcfg, err := DataFromObject(m.getdefault())
		if err != nil {
			return configError("module %s: failed to use module defaults: %v", m.path, err)
		}
		cfg = defcfg
	}

	raw, err := yaml.Marshal(cfg)
	if err != nil {
		return configError("module %s: failed to marshal configuration: %v", m.path, err)
	}
	if err = yaml.Unmarshal(raw, m.ptr); err != nil {
		return configError("module %s: failed to apply configuration: %v", m.path, err)
	}

	return nil
}

// notify notifies this module and its children about a configuration change.
func (m *Module) notify(event Event, source Source) error {
	for _, child := range m.children {
		if err := child.notify(event, source); err != nil {
			return err
		}
	}

	for _, fn := range m.notifiers {
		if err := fn(event, source); err != nil {
			return configError("module %s rejected %v configuration: %v", m.path, event, err)
		}
	}

	return nil
}

// check performs basic sanity checks on the module.
func (m *Module) check() {
	ptrType := reflect.TypeOf(m.ptr)
	ptr := reflect.ValueOf(m.ptr).Elem()
	if ptrType.Kind() != reflect.Ptr || ptr.Kind() != reflect.Struct {
		log.Fatal("module %s: configuration data must be a pointer to a struct, not %T",
			m.path, m.ptr)
	}

	if m.parent == nil || m.parent.isImplicit() {
		return
	}

	ptr = reflect.ValueOf(m.parent.ptr).Elem()
	for i := 0; i < ptr.NumField(); i++ {
		field := ptr.Type().Field(i)
		if m.name == fieldName(field) {
			log.Fatal("module %s: parent has configuration data with conflicting field", m.name)
		}
	}
}

// validate checks that each field of data refers to either module data or a submodule.
func (m *Module) validate(data Data) error {
	log.Debug("validating data for module %s...", m.path)

	modcfg, subcfg := data.split(m.hasChild)
	fields := map[string]struct{}{}

	if m.isImplicit() {
		if len(modcfg) > 0 {
			names := []string{}
			for name := range modcfg {
				names = append(names, name)
			}
			if !m.noValidate {
				return configError("implicit module %s: given configuration data %s",
					m.path, strings.Join(names, ","))
			}
			log.Error("implicit module %s: given configuration date %s",
				m.path, strings.Join(names, ","))
		}
	} else {
		ptr := reflect.ValueOf(m.ptr).Elem()
		for i := 0; i < ptr.NumField(); i++ {
			field := ptr.Type().Field(i)
			name := fieldName(field)
			fields[name] = struct{}{}
		}
	}

	for field := range modcfg {
		if _, ok := fields[field]; !ok {
			if !m.noValidate {
				return configError("module %s: given unknown configuration data %s", m.path, field)
			}
			log.Error("module %s: given unknown configuration data %s", m.path, field)
		}
	}

	subcfg = subcfg.copy()
	for name, child := range m.children {
		childcfg, err := subcfg.pick(name, true)
		if err != nil {
			return configError("module %s: failed to pick configuration for child: %v",
				m.path, child.path, err)
		}
		err = child.validate(childcfg)
		if err != nil {
			return err
		}
	}

	if len(subcfg) > 0 {
		unconsumed := []string{}
		for name := range subcfg {
			unconsumed = append(unconsumed, name)
		}
		return configError("module %s: no child corresponding to data %s",
			m.path, strings.Join(unconsumed, ","))
	}

	return nil
}

// fieldName returns the name used to refer to the struct field in JSON/YAML encoding.
func fieldName(f reflect.StructField) string {
	val, ok := f.Tag.Lookup("json")
	if !ok {
		return f.Name
	}
	tags := strings.Split(val, ",")
	if len(tags) < 1 {
		return f.Name
	}
	name := tags[0]
	if name == "" {
		return f.Name
	}
	return name
}

// lookup finds/creates a module corresponding to the given split module path.
func lookup(path string) *Module {
	names := strings.Split(path, ".")
	path = ""
	module := main
	for _, name := range names {
		if path != "" {
			path += "." + name
		} else {
			path = name
		}
		m, ok := module.children[name]
		if !ok {
			m = &Module{
				path:     path,
				parent:   module,
				name:     name,
				children: make(map[string]*Module),
			}
			module.children[name] = m
		}
		module = m
	}
	return module
}

// Print prints the current configuration, using the given function or fmt.Printf.
func Print(printfn func(string, ...interface{})) {
	data, err := GetConfig()
	if err != nil {
		log.Error("error: failed to get configuration: %v", err)
		return
	}
	data.Print(printfn)
}
