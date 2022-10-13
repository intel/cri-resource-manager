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

package config

import (
	"fmt"
	"io/ioutil"
	"reflect"

	"sigs.k8s.io/yaml"
)

// Fragment must be implemented by registered configuration fragments.
type Fragment interface {
	// Reset the fragment to its zero or default values.
	Reset()
	// Describe the configuration fragment for an 'end user'.
	Describe() string
}

// FragmentValidator is an optional Fragment validation interface.
type FragmentValidator interface {
	// Validate the fragment after a configuration change.
	Validate() error
}

var (
	root = newNode(makePath(""), nil)
)

// Register a configuration fragment at a given path in the configuration.
func Register(path string, cfg interface{}) error {
	if root.cfgValue.IsValid() {
		return fmt.Errorf("can't register config data %q %T: %w", path, cfg,
			fmt.Errorf("config struct already compiled"))
	}

	err := validateUserPtr(cfg)
	if err != nil {
		return fmt.Errorf("can't register config data %q %T: %w", path, cfg, err)
	}

	err = validateFragmentPath(path)
	if err != nil {
		return fmt.Errorf("can't register config data %q %T: %w", path, cfg, err)
	}

	err = root.add(makePath(path), cfg)
	if err != nil {
		return fmt.Errorf("can't register config data %q %T: %w", path, cfg, err)
	}

	return nil
}

// SetFromConfigMap sets the configuration from the given ConfigMap data.
func SetFromConfigMap(data map[string]string) error {
	//
	// Notes:
	//   A ConfigMap is simply a map[string]string. Our configuration data
	//   is hierarchical. It is represented by multiple lines when encoded
	//   in YAML. Therefore it can't be put into a ConfigMap without using
	//   multiline YAML entries. Because each multiline YAML entry becomes
	//   a single ConfigMap entry we need to jump through a few extra hoops
	//   to get back the original YAML data that we want to unmarshal into
	//   our configuration.
	//
	//   First we marshal the ConfigMap into a map[string]interface{} then
	//   we marshal it again into YAML. This should reproduce our original
	//   data with the desired hierarchy which we then use to unmarshal to
	//   our configuration data.
	//

	cfg := map[string]interface{}{}
	for key, value := range data {
		var obj interface{}
		err := yaml.Unmarshal([]byte(value), &obj)
		if err != nil {
			return fmt.Errorf("failed to unmarshal ConfigMap data %s: %s: %w",
				key, value, err)
		}
		cfg[key] = obj
	}

	raw, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal ConfigMap: %w", err)
	}

	return root.SetYAML(raw)
}

// SetFromFile sets the configuration from the given file.
func SetFromFile(path string) error {
	raw, err := ioutil.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to set read file %q: %w", path, err)
	}
	return root.SetYAML(raw)
}

// SetYAML sets the configuration from the given YAML data.
func SetYAML(raw []byte) error {
	return root.SetYAML(raw)
}

// GetYAML returns the current configuration as YAML data.
func GetYAML() ([]byte, error) {
	return root.GetYAML()
}

// GetConfig returns the configuration fragment registered for the given path.
func GetConfig(path string) (interface{}, bool) {
	return root.GetConfig(path)
}

// Compile a go struct to represent our registered configuration.
func Compile() error {
	return root.compile()
}

// Print the configuration.
func Print() {
	fmt.Printf("%s", root.dump(0, true))
}

// Describe the configuration fragment for the given path.
func Describe(names ...string) {
	fmt.Printf("XXX TODO: config.Describe()\n")
	Print()
	return
}

// Dump the configuration with or without current data.
func Dump(dumpData bool) string {
	return root.dump(0, dumpData)
}

// Verify the validity of a user-supplied fragment data pointer.
func validateUserPtr(ptr interface{}) error {
	if ptr == nil {
		return fmt.Errorf("data should be a non-nil pointer (to a struct)")
	}

	f, ok := ptr.(Fragment)
	if !ok {
		v := (*Fragment)(nil)
		return fmt.Errorf("type %T does not implement type %s.%s", ptr,
			reflect.TypeOf(v).Elem().PkgPath(), reflect.TypeOf(v).Elem().Name())
	}
	f.Reset()

	if kind := reflect.TypeOf(ptr).Kind(); kind != reflect.Ptr {
		return fmt.Errorf("data should be a pointer (to a struct), not a %s", kind)
	}

	if reflect.ValueOf(ptr).IsNil() {
		return fmt.Errorf("data should be a non-nil pointer (to a struct)")
	}

	if kind := reflect.ValueOf(ptr).Elem().Kind(); kind != reflect.Struct {
		return fmt.Errorf("data should be a pointer to a struct (not %s %T)", kind, ptr)
	}

	return nil
}

// Verify the validity of a user-supplied fragment path.
func validateFragmentPath(path string) error {
	// XXX TODO: If possible it would be better to reorganise
	// the code, mostly the range makePath()... loops, so that
	// this extra check wouldn't be necessary by instead we'd
	// just let makePath fail for the empty path "".
	if path == "" {
		return fmt.Errorf("config fragment path should not be empty")
	}
	return nil
}
