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
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

const (
	// DefaultRuntimeConfig is the default runtime configuration.
	DefaultRuntimeConfig = "runtime-config"
)

// Config is a configuration collection, basically a set of configuration Modules.
type Config struct {
	name        string
	description string
	onError     ErrorHandling
	keepUnset   bool
	notify      []NotifyFn
	modules     map[string]*Module
	modOrder    []string
	flagset     *flag.FlagSet
	args        []string
}

// ErrorHandling defines how Parse*() and Restore() behaves when an error is encountered.
type ErrorHandling flag.ErrorHandling

const (
	ContinueOnError = iota + 1
	ExitOnError
	PanicOnError
	StopOnError
	IgnoreErrors
)

// Source describes where configuration data has been acquired from.
type Source string

const (
	// CommandLine is the command line configuration source.
	CommandLine Source = "command line configuration"
	// ConfigFile is a YAML/JSON file configuration source.
	ConfigFile Source = "configuration file"
	// External is an external configuration source.
	External Source = "external configuration"
	// ConfigBackup is a Snapshot, a backup of a previous configuration.
	ConfigBackup Source = "configuration backup"
)

// NotifyFn is the type of a configuration change notification functions.
type NotifyFn func(Event, Source) error

// Event describes the reason why a notification callback has been invoked.
type Event string

const (
	// UpdateEvent is the event type for a configuration update.
	UpdateEvent Event = "updated"
	// RevertEvent is the event type for a ocnfiguration rollback.
	RevertEvent Event = "reverted"
)

// Snapshot holds a snapshot of configuration data, used for backup/rollback.
type Snapshot struct {
	values map[string]map[string]Any
}

// Any is the type we use to hammer all module data efectively into map[string]string.
type Any interface{}

// configs is used to look up configuration collections by name
var configs = make(map[string]*Config)

// aliasen is a mapping of alternative to canonical module and argument names.
var aliasen = make(map[string]string)

// NewConfig creates a new configuration.
func NewConfig(name, description string, opts ...Option) *Config {
	c := GetConfig(name)
	if !c.isPending() {
		log.Panic("can't create configuration collection %s, already exists (%s)",
			name, c.description)
	}

	if description == "" {
		c.description = "<no description provided for configuration collection " + name + ">"
	} else {
		c.description = description
	}
	for _, opt := range opts {
		opt.apply(c)
	}

	return c
}

// GetConfig looks up the named configuration.
func GetConfig(name string) *Config {
	if c, ok := configs[name]; ok {
		return c
	}

	c := &Config{
		name:    name,
		modules: make(map[string]*Module),
		notify:  []NotifyFn{},
	}
	configs[name] = c

	genAliasen(name)

	return c
}

// GetModule looks up the named module within the configuration.
func (c *Config) GetModule(name string) *Module {
	if m, ok := c.modules[name]; ok {
		return m
	}

	m := &Module{
		name:    name,
		parent:  c,
		FlagSet: flag.NewFlagSet(name, flag.ContinueOnError),
	}
	c.modules[name] = m
	c.modOrder = nil

	genAliasen(name)

	return m
}

// sortModules sorts the names of modules by order of preference for resolving arguments.
func (c *Config) sortModules() []string {
	if c.modOrder != nil {
		return c.modOrder
	}
	c.modOrder = make([]string, 0, len(c.modules))
	for name := range c.modules {
		c.modOrder = append(c.modOrder, name)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(c.modOrder)))

	log.Debug("* module resolve order: %s", strings.Join(c.modOrder, ","))
	return c.modOrder
}

// Notify notifies configuration changes through all registered notifiers.
func (c *Config) Notify(event Event, source Source) error {
	var err error
	var onError ErrorHandling

	if source == ConfigBackup {
		onError = ContinueOnError
	} else {
		onError = c.onError
	}

	for mod, m := range c.modules {
		if e := m.Notify(event, source); e != nil {
			err = configError("%s: configuration rejected by module %s: %v", c.name, mod, e)
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
			}
		}
	}

	for _, fn := range c.notify {
		if e := fn(event, source); e != nil {
			err = configError("%s: configuration rejected: %v", c.name, e)
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
			}
		}
	}

	return nil
}

// SetVar sets the given variable of the configuration collection.
func (c *Config) SetVar(name, value string) error {
	m, n, v := c.resolve(name)
	if m == nil {
		return configError("%s: failed to resolve %s to module", c.name, name)
	}
	if err := m.SetVar(n, v); err != nil {
		return configError("%s: failed to %s.%s to %s: %v", c.name, m.name, n, v, err)
	}

	return nil
}

// SetModuleVar sets the given module variable value in the configuration collection.
func (c *Config) SetModuleVar(mod, name, value string) error {
	m, ok := c.modules[mod]
	if !ok {
		return configError("%s: failed to set %s.%s to %s: no such module",
			c.name, mod, name, value)
	}

	if err := m.SetVar(name, value); err != nil {
		return configError("%s: failed to set %s.%s to %s: %v", c.name, mod, name, value)
	}

	return nil
}

// Reset resets the configuration to its defaults.
func (c *Config) Reset() error {
	var err error

	for mod, m := range c.modules {
		if e := m.Reset(); e != nil {
			err = configError("failed to reset module %s: %v", mod, e)
			log.Error("%v", err)
		}
	}

	return err
}

// SnapshotFromData creates a snapshot from external configuration.
func SnapshotFromData(raw []byte) (*Snapshot, error) {
	snapshot := &Snapshot{}
	if err := yaml.Unmarshal(raw, &snapshot.values); err != nil {
		return nil, configError("failed to load snapshot from configuration data: %v", err)
	}
	snapshot.unalias()

	log.Debug("snapshot created from data:")
	for mod, values := range snapshot.values {
		log.Debug("  module %s:", mod)
		for key, val := range values {
			log.Debug("    %s: %v", key, val)
		}
	}

	return snapshot, nil
}

// Backup returns a snapshot of the current configuration.
func (c *Config) Backup() *Snapshot {
	snapshot := &Snapshot{values: make(map[string]map[string]Any)}
	for mod, m := range c.modules {
		snapshot.values[mod] = m.Backup()
	}
	return snapshot
}

// Restore restores a previous snapshot.
func (c *Config) Restore(snapshot *Snapshot, operation string) error {
	var err error

	for mod, m := range c.modules {
		log.Info("%s module %s...", operation, mod)
		found, e := m.Restore(snapshot)
		if e != nil {
			log.Error("failed to %s module %s: %v", operation, mod, e)
			err = configError("failed to %s module %s: %v", operation, mod, e)
			continue
		}

		if !found && !c.keepUnset && !m.keepUnset {
			if e := m.Reset(); e != nil {
				log.Error("failed to %s/reset module %s: %v", operation, mod, e)
				err = configError("failed to %s/reset module %s: %v", operation, mod, e)
			}
		}
	}

	return err
}

// Unalias unaliases all module and variable names in a snapshot.
func (s *Snapshot) unalias() {
	for mod, values := range s.values {
		delete(s.values, mod)
		for name, value := range values {
			delete(values, name)
			values[unalias(name)] = value
		}
		s.values[unalias(mod)] = values
	}
}

// ParseArgList parses the given arguments list updating the configuration.
func (c *Config) ParseArgList(args []string, source Source, extra *flag.FlagSet) error {
	var err error

	snapshot := c.Backup()

	args, modules := c.preprocess(args)
	flagset := c.flagSet(extra)
	if err = flagset.Parse(args); err != nil {
		if source == CommandLine && err == flag.ErrHelp {
			return err
		}
		c.Restore(snapshot, "reconfigure")
		return err
	}

	c.args = flagset.Args()

	for _, m := range c.modules {
		if _, ok := modules[m.name]; !ok && !c.keepUnset && !m.keepUnset {
			if err = m.Reset(); err != nil {
				return err
			}
		}
	}

	if err = c.Notify(UpdateEvent, source); err != nil {
		c.Restore(snapshot, "revert")
		c.Notify(RevertEvent, ConfigBackup)
		return err
	}

	return nil
}

// ParseCmdline parses the command line and updates the configuration.
func (c *Config) ParseCmdline() error {
	return c.ParseArgList(os.Args[1:], CommandLine, flag.CommandLine)
}

// ParseYAMLFile parses the given YAML file and updates the configuration.
func (c *Config) ParseYAMLFile(path string) error {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	raw, err := ioutil.ReadFile(path)
	if err != nil {
		return configError("failed to read configuration file %s: %v", path, err)
	}

	return c.ParseYAMLData(raw, ConfigFile)
}

// ParseYAMLData parses the given YAML data and updates the configuration.
func (c *Config) ParseYAMLData(raw []byte, source Source) error {
	var err error

	snapshot, err := SnapshotFromData(raw)
	if err != nil {
		return err
	}

	backup := c.Backup()
	if err = c.Restore(snapshot, "reconfigure"); err != nil {
		c.Restore(backup, "revert")
		return err
	}

	if err = c.Notify(UpdateEvent, source); err != nil {
		c.Restore(backup, "revert")
		c.Notify(RevertEvent, ConfigBackup)
		return err
	}

	return nil
}

// Name returns the name string of the configuration.
func (c *Config) Name() string {
	return c.name
}

// Description returns the description string of the configuration.
func (c *Config) Description() string {
	return c.description
}

// Usage prints help on usage of this configuration from the command line.
func (c *Config) Usage() {
	c.flagSet(nil).Usage()
}

// Help prints help on usage.
func (c *Config) Help(args ...string) {
	for _, arg := range args {
		m, ok := c.modules[arg]
		if !ok {
			log.Error("I can't give help on '%s'.", arg)
			continue
		}
		fmt.Printf("Module %s\n\n", m.name)
		fmt.Printf("Description:\n")
		fmt.Printf("  %s\n\n", m.description)
		m.Usage()
		fmt.Printf("\n\n")
	}
}

// Args returns the non-flag arguments passed to this configuration.
func (c *Config) Args() []string {
	return c.args
}

// isPending returns true if the configuration has not been fully created yet.
func (c *Config) isPending() bool {
	return c.description == ""
}

// FlagSet prepares and returns a flag.FlagSet for the configuration collection.
func (c *Config) flagSet(extra *flag.FlagSet) *flag.FlagSet {
	if c.flagset == nil {
		c.flagset = flag.NewFlagSet(c.name, c.ErrorHandling())
		for name, m := range c.modules {
			genAliasen(name)
			if name != "" {
				name += "."
			}
			m.VisitAll(func(f *flag.Flag) {
				genAliasen(f.Name)
				c.flagset.Var(f.Value, name+f.Name, f.Usage)
			})
		}
	}

	if extra == nil {
		return c.flagset
	}

	fs := flag.NewFlagSet(c.name, c.ErrorHandling())
	extra.VisitAll(func(f *flag.Flag) {
		fs.Var(f.Value, f.Name, f.Usage)
	})
	c.flagset.VisitAll(func(f *flag.Flag) {
		fs.Var(f.Value, f.Name, f.Usage)
	})

	return fs
}

// ErrorHandling returns a flag.ErrorHandling approximation of c.onError.
func (c *Config) ErrorHandling() flag.ErrorHandling {
	switch c.onError {
	case ContinueOnError:
		return flag.ContinueOnError
	case ExitOnError:
		return flag.ExitOnError
	case PanicOnError:
		return flag.PanicOnError
	default:
		return flag.ContinueOnError
	}
}

// preprocess resolves each argument in a list to the canonical --module.name[=value] form.
func (c *Config) preprocess(orig []string) ([]string, map[string]struct{}) {
	modules := make(map[string]struct{})
	args := make([]string, 0, len(orig))
	for _, arg := range orig {
		if !strings.HasPrefix(arg, "-") {
			args = append(args, arg)
			continue
		}

		m, name, value := c.resolve(arg)
		if m == nil && value == "" {
			args = append(args, arg)
			continue
		}

		modules[m.name] = struct{}{}
		if m.name != "" {
			name = unalias(m.name) + "." + unalias(name)
		}
		if value != "" {
			args = append(args, "--"+name+"="+value)
		} else {
			args = append(args, "--"+name)
		}
	}

	log.Debug("* preprocessed '%s': '%s'", strings.Join(orig, ","), strings.Join(args, ","))

	return args, modules
}

// resolve resolves single argument splitting it into a module, a name, and a value.
func (c *Config) resolve(arg string) (*Module, string, string) {
	c.sortModules()
	stripped := ""
	switch {
	case strings.HasPrefix(arg, "--"):
		arg = arg[2:]
		stripped = "--"
	case strings.HasPrefix(arg, "-"):
		arg = arg[1:]
		stripped = "-"
	default:
		return nil, arg, ""
	}

	name, value := "", ""
	nameval := strings.SplitN(arg, "=", 2)
	name = nameval[0]
	if len(nameval) == 2 {
		value = nameval[1]
	}

	if strings.Contains(name, ".") {
		split := strings.SplitN(name, ".", 2)
		mod, name := split[0], split[1]
		if m, ok := c.modules[mod]; ok {
			return m, name, value
		}
	}

	for _, mod := range c.modOrder {
		m := c.modules[mod]
		if mod == MainModule || mod == "" {
			if f := m.Lookup(name); f != nil {
				return m, name, value
			}
			continue
		}

		log.Debug("* preprocess: checking %s (%s, %s) against module %s", arg, name, value, mod)

		if strings.HasPrefix(name, mod+"-") {
			return m, strings.TrimPrefix(name, mod+"-"), value
		}
	}

	return nil, stripped, ""
}

// genAliasen generates camelCase and CamelCase aliasen for camel-case names.
func genAliasen(name string) {
	var r rune
	var err error

	if name == "" {
		return
	}

	sr, sb := strings.NewReader(name), strings.Builder{}
	dash := false
	for r, _, err = sr.ReadRune(); err == nil; r, _, err = sr.ReadRune() {
		if r == '-' {
			dash = true
		} else {
			if dash {
				_, err = sb.WriteRune(unicode.ToUpper(r))
				dash = false
			} else {
				_, err = sb.WriteRune(unicode.ToLower(r))
			}
			if err != nil {
				log.Error("failed to generate aliases for '%s': %v", name, err)
				return
			}
		}
	}

	alias := sb.String()
	if alias != name {
		aliasen[alias] = name
	}

	alias = strings.ToUpper(alias[:1]) + alias[1:]
	aliasen[alias] = name
}

// unalias returns the canonical form of a given name.
func unalias(name string) string {
	if canonical, ok := aliasen[name]; ok {
		return canonical
	}

	normalized := normalize(name)

	if canonical, ok := aliasen[normalized]; ok {
		aliasen[name] = canonical
		return canonical
	}

	return name
}

// normalize replaces any sequence of uppercase runes with a camelcase/capitalized sequence.
func normalize(name string) string {
	sr, sb := strings.NewReader(name), strings.Builder{}
	low := true

	for {
		r, _, err := sr.ReadRune()
		if err != nil {
			if err == io.EOF {
				break
			}
			return name
		}

		if unicode.IsUpper(r) {
			if !low {
				r = unicode.ToLower(r)
			} else {
				low = false
			}
		} else {
			low = true
		}
		if _, err := sb.WriteRune(r); err != nil {
			return name
		}
	}

	return sb.String()
}
