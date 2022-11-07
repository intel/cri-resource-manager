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

package log

import (
	"encoding/json"
	"os"
	"strings"

	pkgcfg "github.com/intel/cri-resource-manager/pkg/config"
	"github.com/intel/cri-resource-manager/pkg/log/klogcontrol"
	"github.com/intel/cri-resource-manager/pkg/utils"
)

const (
	// DefaultLevel is the default logging severity level.
	DefaultLevel = LevelInfo
	// debugEnvVar is the environment variable used to seed debugging flags.
	debugEnvVar = "LOGGER_DEBUG"
	// configModule is our module name in the runtime configuration.
	configModule = "logger"
)

// options capture our runtime configuration.
type options struct {
	// Klog contains klog-specific options.
	Klog klogcontrol.Options
	// Debug defines which sources produce debug messages.
	Debug srcmap
	// LogSource determines if messages are prefixed with the logger source
	LogSource bool
}

// srcmap tracks debugging settings for sources.
type srcmap map[string]bool

var (
	// Runtime logging configuration.
	opt *options
	// Default debugging configuration.
	defaultDebugFlags srcmap
	// Default klog configuration.
	defaultKlogFlags klogcontrol.Options
	// klog control
	klogctl *klogcontrol.Control
)

// parse parses the given string and updates the srcmap accordingly.
func (m *srcmap) parse(value string) error {
	if *m == nil {
		*m = make(srcmap)
	}
	if value = strings.TrimSpace(value); value == "" {
		return nil
	}

	prev, state, src := "", "", ""
	for _, entry := range strings.Split(value, ",") {
		if entry = strings.TrimSpace(entry); entry == "" {
			continue
		}
		statesrc := strings.Split(entry, ":")
		switch len(statesrc) {
		case 2:
			state, src = statesrc[0], strings.TrimSpace(statesrc[1])
		case 1:
			state, src = "", strings.TrimSpace(statesrc[0])
		default:
			return loggerError("invalid state spec '%s' in source map", entry)
		}
		if state != "" {
			prev = state
		} else {
			state = prev
			if state == "" {
				state = "on"
			}
		}

		if src == "all" {
			src = "*"
		}

		enabled, err := utils.ParseEnabled(state)
		if err != nil {
			return loggerError("invalid state '%s' in source map", state)
		}
		(*m)[src] = enabled
	}

	return nil
}

// String returns a string representation of the srcmap.
func (m *srcmap) String() string {
	off := ""
	on := ""
	for src, state := range *m {
		if state {
			if on == "" {
				on = src
			} else {
				on += "," + src
			}
		} else {
			if off == "" {
				off = src
			} else {
				off += "," + src
			}
		}
	}

	switch {
	case on == "" && off == "":
		return ""
	case off == "":
		return "on:" + on
	case on == "":
		return "off:" + off
	}
	return "on:" + on + "," + "off:" + off
}

// MarshalJSON is the JSON marshaller for srcmap.
func (m srcmap) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.String())
}

// UnmarshalJSON is the JSON unmarshaller for srcmap.
func (m *srcmap) UnmarshalJSON(raw []byte) error {
	cfgstr := ""
	if err := json.Unmarshal(raw, &cfgstr); err != nil {
		return loggerError("failed to unmarshal source map '%s': %v", string(raw), err)
	}
	if err := m.parse(cfgstr); err != nil {
		return loggerError("failed to unmarshal source map '%s': %v", string(raw), err)
	}
	return nil
}

// cloneFrom state from another srcmap.
func (m *srcmap) cloneFrom(o srcmap) {
	*m = make(srcmap)
	for src, state := range o {
		(*m)[src] = state
	}
}

// clone returns a copy of the srcmap.
func (m srcmap) clone() srcmap {
	if m == nil {
		return nil
	}
	o := make(srcmap)
	for src, state := range m {
		o[src] = state
	}
	return o
}

// apply applies the options to logging.
func (o *options) apply() error {
	log.Lock()
	defer log.Unlock()

	prefix := o.LogSource
	if logToStderr, ok := o.Klog["logtostderr"]; ok && logToStderr.(bool) {
		if skipHeaders, ok := o.Klog["skip_headers"]; ok && skipHeaders.(bool) {
			prefix = true
		}
	}

	log.setDbgMap(o.Debug.clone())
	log.setPrefix(prefix)

	return klogctl.Configure(o.Klog)
}

// defaultOptions returns our current default runtime options.
func defaultOptions() interface{} {
	o := &options{}

	o.Debug.cloneFrom(defaultDebugFlags)
	if defaultKlogFlags != nil {
		o.Klog.CloneFrom(defaultKlogFlags)
	} else {
		o.Klog = klogctl.CurrentOptions()
	}

	return o
}

const (
	// ConfigDescription describes our configuration fragment.
	ConfigDescription = "logging control" // XXX TODO
)

func (o *options) Describe() string {
	return ConfigDescription
}

func (o *options) Reset() {
	*o = options{}

	o.Debug.cloneFrom(defaultDebugFlags)
	if defaultKlogFlags != nil {
		o.Klog.CloneFrom(defaultKlogFlags)
	} else {
		o.Klog = klogctl.CurrentOptions()
	}
}

func (o *options) Validate() error {
	deflog.Info("new logger configuration")
	deflog.Info(" * debugging: %s", o.Debug.String())
	deflog.Info(" * log source: %v", o.LogSource)
	deflog.InfoBlock(" * klog: ", "%s", o.Klog.String())

	// On the first configuration update event, we record the current values
	// of klog flags as the runtime defaults. Effectively this allows one to
	// override the built-in defaults using klog command line options (or
	// environment variables as interpreted by klogcontrol). The recorded
	// defaults will also reflect any potential programmatic changes done by
	// (mis-)using flag.Set() but there's not much we can do about that.
	if defaultKlogFlags == nil {
		defaultKlogFlags = klogctl.CurrentOptions()
	}

	if o.Klog == nil {
		o.Klog = make(klogcontrol.Options)
	}

	// The behavior of the options.Klog map across updates is difficult
	// to understand. To make it more user friendly we fill in runtime
	// defaults for each unset entry (klog flags) here.
	for flag, value := range defaultKlogFlags {
		if _, ok := o.Klog[flag]; !ok {
			o.Klog[flag] = value
		}
	}

	return o.apply()
}

// Set up klog control, set pkg/config logger, register us for configuration handling.
func init() {
	klogctl = klogcontrol.Get()
	opt = defaultOptions().(*options)
	opt.apply()

	cfglog := log.get("config")
	pkgcfg.SetLogger(pkgcfg.Logger{
		DebugEnabled: cfglog.DebugEnabled,
		Debug:        cfglog.Debug,
		Info:         cfglog.Info,
		Warning:      cfglog.Warn,
		Error:        cfglog.Error,
		Fatal:        cfglog.Fatal,
		Panic:        cfglog.Panic,
	})

	defaultDebugFlags = make(srcmap)
	if value, ok := os.LookupEnv(debugEnvVar); ok {
		if err := defaultDebugFlags.parse(value); err != nil {
			Default().Error("failed to parse %s %q: %v", debugEnvVar,
				value, err)
		} else {
			log.setDbgMap(defaultDebugFlags)
			Default().Info("seeded debug flags ($%s): %s", debugEnvVar,
				defaultDebugFlags.String())
		}
	}

	pkgcfg.Register(configModule, "log control", opt, defaultOptions)
}
