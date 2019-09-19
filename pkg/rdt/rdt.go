/*
Copyright 2019 Intel Corporation

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package rdt

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ghodss/yaml"

	logger "github.com/intel/cri-resource-manager/pkg/log"
	"github.com/intel/cri-resource-manager/pkg/utils"
)

const resctrlGroupPrefix = "cri-resmgr."

// Control is the interface managing Intel RDT resources
type Control interface {
	// GetClasses returns the names of RDT classes (or resctrl control groups)
	// available
	GetClasses() []string

	// SetProcessClass assigns a set of processes to a RDT class
	SetProcessClass(string, ...string) error

	// SetConfig re-configures RDT resources
	SetConfig(string) error
}

var rdtInfo Info

type control struct {
	logger.Logger

	conf config
}

type config struct {
	Options       SchemaOptions                 `json:"options,omitempty"`
	ResctrlGroups map[string]ResctrlGroupConfig `json:"resctrlGroups"`
}

// NewControl returns new instance of the RDT Control interface
func NewControl(resctrlpath string, config string) (Control, error) {
	var err error
	r := &control{Logger: logger.NewLogger("rdt")}

	// Get info from the resctrl filesystem
	rdtInfo, err = getRdtInfo(resctrlpath)
	if err != nil {
		return nil, err
	}

	// Configure resctrl
	r.conf, err = parseConfData([]byte(config))
	if err != nil {
		return nil, err
	}
	if err := r.configureResctrl(r.conf); err != nil {
		return nil, rdtError("configuration failed: %v", err)
	}

	return r, nil
}

func (r *control) GetClasses() []string {
	ret := make([]string, len(r.conf.ResctrlGroups))

	i := 0
	for k := range r.conf.ResctrlGroups {
		ret[i] = k
		i++
	}
	sort.Strings(ret)
	return ret
}

func (r *control) SetProcessClass(class string, pids ...string) error {
	if _, ok := r.conf.ResctrlGroups[class]; !ok {
		return rdtError("unknown RDT class %q", class)
	}

	path := filepath.Join(r.resctrlGroupPath(class), "tasks")
	for _, pid := range pids {
		if err := ioutil.WriteFile(path, []byte(pid), 0644); err != nil {
			return rdtError("failed to assign process %s to class %q: %v", pid, class, err)
		}
	}
	return nil
}

func (r *control) SetConfig(newConfRaw string) error {
	newConf, err := parseConfData([]byte(newConfRaw))
	if err != nil {
		return err
	}

	err = r.configureResctrl(newConf)
	if err != nil {
		// Try to roll-back
		r.Error("failed to configure resctrl: %v", err)
		r.Error("attempting configuration roll-back")
		if err := r.configureResctrl(r.conf); err != nil {
			r.Error("rollback failed: %v", err)
		}
		return rdtError("resctrl confuguration failed: %v", err)
	}

	r.conf = newConf

	return nil
}

func (r *control) configureResctrl(conf config) error {
	r.Debug("applying new configuration:\n%s", utils.DumpJSON(conf))

	// Remove stale resctrl groups
	existingGroups, err := r.getResctrlGroups()
	if err != nil {
		return err
	}

	for _, name := range existingGroups {
		if _, ok := conf.ResctrlGroups[name]; !ok {
			path := r.resctrlGroupPath(name)
			tasks, err := r.resctrlGroupTasks(name)
			if err != nil {
				return rdtError("failed to get resctrl group tasks: %v", err)
			}
			if len(tasks) > 0 {
				return rdtError("refusing to remove non-empty resctrl group %q", path)
			}
			err = os.Remove(path)
			if err != nil {
				return rdtError("failed to remove resctrl group %q: %v", path, err)
			}
		}
	}

	// Try to apply given configuration
	for name, grpConf := range conf.ResctrlGroups {
		err := r.configureResctrlGroup(name, grpConf, conf.Options)
		if err != nil {
			return err
		}
	}

	return nil
}

func parseConfData(raw []byte) (config, error) {
	conf := config{}

	err := yaml.Unmarshal(raw, &conf)
	if err != nil {
		return conf, rdtError("failed to parse configuration: %v", err)
	}
	return conf, nil
}

func (r *control) configureResctrlGroup(name string, config ResctrlGroupConfig, options SchemaOptions) error {
	path := r.resctrlGroupPath(name)
	if err := os.Mkdir(path, 0755); err != nil && !os.IsExist(err) {
		return err
	}

	schemata := ""
	// Handle L3 cache allocation
	switch {
	case rdtInfo.l3.Supported():
		if !config.L3CodeSchema.IsNil() && !options.L3Code.Optional {
			return rdtError("separate L3 code path schema for %q requested but CDP not enabled", name)
		}
		if !config.L3DataSchema.IsNil() && !options.L3Data.Optional {
			return rdtError("separate L3 data path schema for %q requested but CDP not enabled", name)
		}
		r.Debug("configuring L3 schema for %q", name)
		schemata += config.L3Schema.ToStr(L3SchemaTypeUnified)
	case rdtInfo.l3data.Supported() || rdtInfo.l3code.Supported():
		if !config.L3CodeSchema.IsNil() {
			r.Debug("using specific L3 code schema for %q", name)
			schemata += config.L3CodeSchema.ToStr(L3SchemaTypeCode)
		} else {
			// No L3 code schema was specified -> use the "unified" L3 schema
			r.Debug("using unified L3 schema in code path for %q", name)
			schemata += config.L3Schema.ToStr(L3SchemaTypeCode)
		}
		if !config.L3DataSchema.IsNil() {
			r.Debug("using specific L3 data schema for %q", name)
			schemata += config.L3DataSchema.ToStr(L3SchemaTypeData)
		} else {
			// No L3 data schema was specified -> use the "unified" L3 schema
			r.Debug("using unified L3 schema in data path for %q", name)
			schemata += config.L3Schema.ToStr(L3SchemaTypeData)
		}
	default:
		if (!config.L3Schema.IsNil() && !options.L3.Optional) ||
			(!config.L3CodeSchema.IsNil() && !options.L3Code.Optional) ||
			(!config.L3DataSchema.IsNil() && !options.L3Data.Optional) {
			return rdtError("L3 cache allocation for %q specified in configuration but not supported by system", name)
		}
	}

	// Handle memory bandwidth allocation
	switch {
	case rdtInfo.mb.Supported():
		schemata += config.MBSchema.ToStr()
	default:
		if !config.MBSchema.IsNil() && !options.MB.Optional {
			return rdtError("memory bandwidth allocation specified in configuration but not supported by system")
		}
	}

	if len(schemata) > 0 {
		r.Debug("writing schemata %q", schemata)
		if err := ioutil.WriteFile(filepath.Join(path, "schemata"), []byte(schemata), 0644); err != nil {
			return err
		}
	} else {
		r.Debug("empty schemata")
	}

	return nil
}

func (r *control) resctrlGroupPath(name string) string {
	return filepath.Join(rdtInfo.resctrlPath, resctrlGroupPrefix+name)
}

func (r *control) getResctrlGroups() ([]string, error) {

	files, err := ioutil.ReadDir(rdtInfo.resctrlPath)
	if err != nil {
		return nil, err
	}
	groups := make([]string, 0, len(files))
	for _, file := range files {
		fullName := file.Name()
		if strings.HasPrefix(fullName, resctrlGroupPrefix) {
			groups = append(groups, fullName[len(resctrlGroupPrefix):])
		}
	}
	return groups, nil
}

func (r *control) resctrlGroupTasks(name string) ([]string, error) {
	path := filepath.Join(r.resctrlGroupPath(name), "tasks")
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return []string{}, err
	}
	split := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(split[0]) > 0 {
		return split, nil
	}
	return []string{}, nil
}

func rdtError(format string, args ...interface{}) error {
	return fmt.Errorf("rdt: "+format, args...)
}
