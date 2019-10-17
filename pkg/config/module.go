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
	"flag"
	"fmt"
	"github.com/ghodss/yaml"
	"io/ioutil"
	"path/filepath"
	"strings"
)

const (
	// MainModule is the name of the main configuration module (with unprefixed variables).
	MainModule = "main"
)

// Module is a collection of configuration data.
type Module struct {
	name        string
	description string
	parent      *Config
	onError     ErrorHandling
	keepUnset   bool
	notify      []NotifyFn
	*flag.FlagSet
}

// yamlVar is the type for variables set from a YAML file or external YAML data.
type yamlVar struct {
	value   flag.Value // associated flag.Value
	path    string     // path if value from a file
	content string     // cached file content
}

// Register creates a new configuration module adding it into a configuration.
func Register(name, description string, opts ...Option) *Module {
	config := parentName(DefaultRuntimeConfig)
	for _, opt := range opts {
		if opt.apply(&config) == nil {
			break
		}
	}
	parent := GetConfig(string(config))

	if description == "" {
		description = "<no description for module " + parent.Name() + "." + name + ">"
	}

	m := parent.GetModule(name)
	if !m.isPending() {
		log.Fatal("%s: can't register module %s (%s), already registered (%s)",
			parent, name, description, m.description)
	}
	m.description = description

	for _, opt := range opts {
		if err := opt.apply(m); err != nil {
			log.Error("%v", err)
		}
	}

	return m
}

// YamlVar creates a variable which takes its value from the content of a file.
func (m *Module) YamlVar(value flag.Value, name string, usage string) {
	m.Var(newYamlVar(value), name, usage)
}

// SetVar sets the named variable to the given value.
func (m *Module) SetVar(name, value string) error {
	var f *flag.Flag

	log.Debug("setting %s.%s = %s", m.name, name, value)

	if f = m.Lookup(name); f == nil {
		return configError("module '%s': no variable '%s'", m.name, name)
	}
	if err := f.Value.Set(value); err != nil {
		return configError("module %s: failed to set '%s': %v", m.name, name, err)
	}

	return nil
}

// WatchUpdates adds a notifier function to the module.
func (m *Module) WatchUpdates(fn NotifyFn) {
	WithNotify(fn).apply(m)
}

// Notify notifies configuration changes through all registered module notifiers.
func (m *Module) Notify(event Event, source Source) error {
	var err error
	var onError ErrorHandling

	if source == ConfigBackup {
		onError = ContinueOnError
	} else {
		onError = m.onError
	}

	for _, fn := range m.notify {
		if e := fn(event, source); e != nil {
			err = configError("%s: configuration rejected: %v", m.name, e)
			switch onError {
			case ContinueOnError:
				log.Error("%v", err)
			case ExitOnError:
				log.Fatal("%v", err)
			case PanicOnError:
				log.Panic("%v", err)
			case StopOnError:
				return err
			case IgnoreErrors:
				log.Warning("%v", err)
				err = nil
			}
		}
	}

	return err
}

// Reset resets the configuration of all module variables to their default.
func (m *Module) Reset() error {
	var err error

	m.VisitAll(func(f *flag.Flag) {
		if e := f.Value.Set(f.DefValue); e != nil {
			err = configError("%s: failed to reset %s to '%s': %v", f.Name, f.DefValue, e)
			log.Error("%v", err)
		}
	})

	return err
}

// Backup returns a snapshot of the current values of configuration variables.
func (m *Module) Backup() map[string]Any {
	values := make(map[string]Any)

	m.VisitAll(func(f *flag.Flag) {
		values[f.Name] = f.Value.String()
	})

	return values
}

// Restore restores a previously taken snapshot.
func (m *Module) Restore(snapshot *Snapshot) (bool, error) {
	var err error

	values, ok := snapshot.values[m.name]
	if !ok {
		return false, nil
	}

	updated := make(map[string]struct{})
	for name, val := range values {
		updated[name] = struct{}{}
		value := ""
		switch val.(type) {
		case string:
			value = val.(string)
		case int, int8, int16, int32, int64:
			value = fmt.Sprintf("%d", val)
		case uint, uint8, uint16, uint32, uint64:
			value = fmt.Sprintf("%d", val)
		case float64:
			value = fmt.Sprintf("%f", val)
		case bool:
			value = fmt.Sprintf("%v", val)
		default:
			raw, e := yaml.Marshal(val)
			if e != nil {
				log.Error("failed to restore/set %s.%s: %v", m.name, name, e)
				err = configError("failed to restore/set %s.%s: %v", m.name, name, e)
				continue
			}
			value = string(raw)
		}
		if e := m.SetVar(name, value); e != nil {
			log.Error("failed to restore/set %s.%s: %v", m.name, name, e)
			err = configError("failed to restore/set %s.%s: %v", m.name, name, e)
		}
	}

	if m.keepUnset {
		return true, err
	}

	m.VisitAll(func(f *flag.Flag) {
		if _, ok := updated[f.Name]; ok {
			return
		}
		if e := f.Value.Set(f.DefValue); e != nil {
			log.Error("failed to restore/reset %s.%s: %v", m.name, f.Name, e)
			err = configError("%failed to restore/reset %s.%s: %v", m.name, f.Name, e)
		}
	})

	return true, err
}

// isPending returns true if the module has not been fully created yet.
func (m *Module) isPending() bool {
	return m.description == ""
}

// newYamlVar creates a new variable to be set from a YAML file or external YAML data.
func newYamlVar(value flag.Value) flag.Value {
	return &yamlVar{value: value}
}

// readFile sets the variable value to the content of a file.
func (yv *yamlVar) readFile(value string) error {
	path, err := filepath.Abs(value)
	if err != nil {
		return configError("YamlVar: failed to read file '%s': %v", value, err)
	}

	raw, err := ioutil.ReadFile(path)
	if err != nil {
		return configError("YamlVar: failed to read file '%s': %v", path, err)
	}

	yv.path = path
	yv.content = string(raw)
	return nil
}

// Set sets the value of the given YAML variable.
func (yv *yamlVar) Set(value string) error {
	switch {
	case yv == nil:
		return nil

	case value == "":
		yv.path, yv.content = "", ""
		return nil

	case strings.HasPrefix(value, "file://"):
		path := strings.TrimPrefix(value, "file://")
		if err := yv.readFile(path); err != nil {
			return err
		}
		value = yv.content

	case strings.Count(value, " ") == 0 && strings.Count(value, "\n") == 0:
		if err := yv.readFile(value); err != nil {
			return err
		}
		value = yv.content
	}

	return yv.value.Set(value)
}

// String() returns the current value of the YAML variable.
func (yv *yamlVar) String() string {
	if yv == nil {
		return ""
	}

	if yv.content != "" {
		return yv.content
	}

	return yv.path
}
