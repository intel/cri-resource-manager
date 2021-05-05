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

package control

import (
	"encoding/json"
	"fmt"
	"github.com/intel/cri-resource-manager/pkg/config"
	"strings"
)

// Options captures our runtime configuration.
type options struct {
	Controllers map[string]mode
}

// Our runtime configuration.
var opt = defaultOptions().(*options)

// mode describes how errors for the controller should be treated.
type mode int

const (
	// Disabled controller are stopped, hooks are not run.
	Disabled mode = iota
	// Required controllers must start, hooks must succeed.
	Required
	// Optional controllers are Disabled if they can't start, otherwise they are Required.
	Optional
	// Relaxed controllers are Disabled if they can't start, hook failures are not errors.
	Relaxed
	// Default mode is Relaxed.
	Default = Relaxed
)

// ControllerMode returns the current mode for the given controller.
func (o *options) ControllerMode(name string) mode {
	if m, ok := o.Controllers[name]; ok {
		return m
	}

	return Default
}

// configNotify is our configuration update notification callback.
func (o *options) configNotify(event config.Event, source config.Source) error {
	log.Infof("configuration updated")
	for name, controller := range controllers {
		controller.mode = o.ControllerMode(name)
	}
	return nil
}

// String returns the string representation of a mode.
func (m mode) String() string {
	switch m {
	case Disabled:
		return "disabled"
	case Required:
		return "required"
	case Optional:
		return "optional"
	case Relaxed:
		return "relaxed"
	default:
		return fmt.Sprintf("<unknown mode %d>", m)
	}
}

// MarshalJSON is the JSON marshaller for mode.
func (m mode) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.String())
}

// UnmarshalJSON is the JSON unmarshaller for mode.
func (m *mode) UnmarshalJSON(raw []byte) error {
	var str string

	if err := json.Unmarshal(raw, &str); err != nil {
		return controlError("failed to unmarshal mode: %v", err)
	}

	switch strings.ToLower(str) {
	case "disabled", "disable":
		*m = Disabled
	case "required", "mandatory":
		*m = Required
	case "optional":
		*m = Optional
	case "relaxed":
		*m = Relaxed
	default:
		return controlError("invalid mode %s", str)
	}
	return nil
}

// defaultOptions returns a new options instance, all initialized to defaults.
func defaultOptions() interface{} {
	return &options{Controllers: make(map[string]mode)}
}

// Register us for configuration handling.
func init() {
	config.Register("resource-manager.control", "Resource control.", opt, defaultOptions,
		config.WithNotify(opt.configNotify))
}
