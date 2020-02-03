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
	"syscall"

	pkgcfg "github.com/intel/cri-resource-manager/pkg/config"
	logger "github.com/intel/cri-resource-manager/pkg/log"
	"github.com/intel/cri-resource-manager/pkg/utils"
)

const (
	resctrlGroupPrefix = "cri-resmgr."
	// rootClassName is the name we use in our config for the special class
	// that configures the "root" resctrl group of the system
	rootClassName = "SYSTEM_DEFAULT"
)

// Control is the interface managing Intel RDT resources
type Control interface {
	// GetClasses returns the names of RDT classes (or resctrl control groups)
	// available
	GetClasses() []string

	// SetProcessClass assigns a set of processes to a RDT class
	SetProcessClass(string, ...string) error
}

var rdtInfo Info

var log logger.Logger = logger.NewLogger("rdt")

type control struct {
	logger.Logger

	conf config
}

// NewControl returns new instance of the RDT Control interface
func NewControl(resctrlpath string) (Control, error) {
	var err error
	r := &control{Logger: log}

	// Get info from the resctrl filesystem
	rdtInfo, err = getRdtInfo(resctrlpath)
	if err != nil {
		return nil, err
	}

	// Configure resctrl
	r.conf, err = opt.resolve()
	if err != nil {
		return nil, rdtError("invalid configuration: %v", err)
	}

	if err := r.configureResctrl(r.conf); err != nil {
		return nil, rdtError("configuration failed: %v", err)
	}

	pkgcfg.GetModule("rdt").AddNotify(r.configNotify)

	return r, nil
}

func (r *control) GetClasses() []string {
	ret := make([]string, len(r.conf.Classes))

	i := 0
	for k := range r.conf.Classes {
		ret[i] = k
		i++
	}
	sort.Strings(ret)
	return ret
}

func (r *control) SetProcessClass(class string, pids ...string) error {
	if _, ok := r.conf.Classes[class]; !ok {
		return rdtError("unknown RDT class %q", class)
	}

	path := filepath.Join(r.resctrlGroupPath(class), "tasks")
	f, err := os.OpenFile(path, os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	for _, pid := range pids {
		if _, err := f.WriteString(pid + "\n"); err != nil {
			unwrapped := err
			if pathError, ok := err.(*os.PathError); ok {
				unwrapped = pathError.Unwrap()
			}
			if unwrapped == syscall.ESRCH {
				r.Debug("no task %s", pid)
			} else {
				return rdtError("failed to assign processes %v to class %q: %v", pids, class, r.cmdError(err))
			}
		}
	}
	return nil
}

func (r *control) configNotify(event pkgcfg.Event, source pkgcfg.Source) error {
	r.Info("configuration %s", event)

	conf, err := opt.resolve()
	if err != nil {
		return rdtError("invalid configuration: %v", err)
	}

	err = r.configureResctrl(conf)
	if err != nil {
		return rdtError("resctrl configuration failed: %v", err)
	}

	r.conf = conf
	r.Info("configuration finished")

	return nil
}

func (r *control) configureResctrl(conf config) error {
	r.DebugBlock("  applying ", "%s", utils.DumpJSON(conf))

	// Remove stale resctrl groups
	existingClasses, err := r.getClasses()
	if err != nil {
		return err
	}

	for _, name := range existingClasses {
		if _, ok := conf.Classes[name]; !ok {
			tasks, err := r.getClassTasks(name)
			if err != nil {
				return rdtError("failed to get resctrl group tasks: %v", err)
			}
			path := r.resctrlGroupPath(name)
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
	for name, class := range conf.Classes {
		partition := conf.Partitions[class.Partition]
		err := r.configureResctrlGroup(name, class, partition, conf.Options)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *control) configureResctrlGroup(name string, class classConfig,
	partition partitionConfig, options schemaOptions) error {
	if err :=
		os.Mkdir(r.resctrlGroupPath(name), 0755); err != nil && !os.IsExist(err) {
		return err
	}

	schemata := ""
	// Handle L3 cache allocation
	switch {
	case rdtInfo.l3.Supported():
		schema, err := class.L3Schema.ToStr(l3SchemaTypeUnified, partition.L3)
		if err != nil {
			return err
		}
		schemata += schema
	case rdtInfo.l3data.Supported() || rdtInfo.l3code.Supported():
		schema, err := class.L3Schema.ToStr(l3SchemaTypeCode, partition.L3)
		if err != nil {
			return err
		}
		schemata += schema

		schema, err = class.L3Schema.ToStr(l3SchemaTypeData, partition.L3)
		if err != nil {
			return err
		}
		schemata += schema
	default:
		if class.L3Schema != nil && !options.L3.Optional {
			return rdtError("L3 cache allocation for %q specified in configuration but not supported by system", name)
		}
	}

	// Handle memory bandwidth allocation
	switch {
	case rdtInfo.mb.Supported():
		schemata += class.MBSchema.ToStr(partition.MB)
	default:
		if class.MBSchema != nil && !options.MB.Optional {
			return rdtError("memory bandwidth allocation specified in configuration but not supported by system")
		}
	}

	if len(schemata) > 0 {
		r.Debug("writing schemata %q to %q", schemata, r.resctrlGroupDirName(name))
		dirName := r.resctrlGroupDirName(name)
		if err := r.writeRdtFile(filepath.Join(dirName, "schemata"), []byte(schemata)); err != nil {
			return err
		}
	} else {
		r.Debug("empty schemata")
	}

	return nil
}

func (r *control) resctrlGroupDirName(name string) string {
	if name == rootClassName {
		return ""
	}

	return resctrlGroupPrefix + name
}

func (r *control) resctrlGroupPath(name string) string {
	return filepath.Join(rdtInfo.resctrlPath, r.resctrlGroupDirName(name))
}

func (r *control) getClasses() ([]string, error) {

	files, err := ioutil.ReadDir(rdtInfo.resctrlPath)
	if err != nil {
		return nil, err
	}
	classes := make([]string, 0, len(files))
	for _, file := range files {
		fullName := file.Name()
		if strings.HasPrefix(fullName, resctrlGroupPrefix) {
			classes = append(classes, fullName[len(resctrlGroupPrefix):])
		}
	}
	return classes, nil
}

func (r *control) getClassTasks(name string) ([]string, error) {
	data, err := r.readRdtFile(filepath.Join(r.resctrlGroupDirName(name), "tasks"))
	if err != nil {
		return []string{}, err
	}
	split := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(split[0]) > 0 {
		return split, nil
	}
	return []string{}, nil
}

func (r *control) readRdtFile(rdtPath string) ([]byte, error) {
	return ioutil.ReadFile(filepath.Join(rdtInfo.resctrlPath, rdtPath))
}

func (r *control) writeRdtFile(rdtPath string, data []byte) error {
	if err := ioutil.WriteFile(filepath.Join(rdtInfo.resctrlPath, rdtPath), data, 0644); err != nil {
		return r.cmdError(err)
	}
	return nil
}

func (r *control) cmdError(origErr error) error {
	errData, readErr := r.readRdtFile(filepath.Join("info", "last_cmd_status"))
	if readErr != nil {
		return origErr
	}
	cmdStatus := strings.TrimSpace(string(errData))
	if len(cmdStatus) > 0 && cmdStatus != "ok" {
		return fmt.Errorf("%s", cmdStatus)
	}
	return origErr
}

func rdtError(format string, args ...interface{}) error {
	return fmt.Errorf("rdt: "+format, args...)
}
