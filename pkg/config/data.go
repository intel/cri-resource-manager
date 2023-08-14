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
	"fmt"
	"os"
	"sigs.k8s.io/yaml"
	"strings"
)

// Data is our internal representation of configuration data.
type Data map[string]interface{}

// DataFromObject remarshals the given object into configuration data.
func DataFromObject(obj interface{}) (Data, error) {
	raw, err := yaml.Marshal(obj)
	if err != nil {
		return nil, configError("failed to marshal object %T to data: %v", obj, err)
	}
	data := make(Data)
	if err = yaml.Unmarshal(raw, &data); err != nil {
		return nil, configError("failed to unmarshal object %T to data: %v", obj, err)
	}
	return data, nil
}

// DataFromStringMap remarshals the given map into configuration data.
func DataFromStringMap(smap map[string]string) (Data, error) {
	data := make(Data)
	for key, val := range smap {
		var obj interface{}
		if err := yaml.Unmarshal([]byte(val), &obj); err != nil {
			return nil, configError("failed to unmarshal data from map: %v", err)
		}
		data[key] = obj
	}
	return data, nil
}

// DataFromFile unmarshals the content of the given file into configuration data.
func DataFromFile(path string) (Data, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, configError("failed to read file %q: %v", path, err)
	}
	data := make(Data)
	if err := yaml.Unmarshal(raw, &data); err != nil {
		return nil, configError("failed to load configuration from file %q: %v", path, err)
	}
	return data, nil
}

// copy does a shallow copy of the given data.
func (d Data) copy() Data {
	data := make(Data)
	for key, value := range d {
		data[key] = value
	}
	return data
}

// split splits up the given data to module- and child-specific parts.
func (d Data) split(hasChild func(string) bool) (Data, Data) {
	mod, sub := make(Data), make(Data)
	for key, val := range d {
		if hasChild(key) || strings.IndexByte(key, '.') != -1 {
			sub[key] = val
		} else {
			mod[key] = val
		}
	}
	return mod, sub
}

// pick picks data for the given key.
func (d Data) pick(key string, removePicked bool) (Data, error) {
	var data Data
	var err error

	if obj, ok := d[key]; ok {
		data, err = DataFromObject(obj)
		if err != nil {
			return nil, err
		}
		if removePicked {
			delete(d, key)
		}
	}

	// pick/remove data for all dotted keys matching the key being picked
	for k, v := range d {
		split := strings.Split(k, ".")
		if len(split) > 1 && split[0] == key {
			if data == nil {
				data = make(Data)
			}
			subkey := strings.Join(split[1:], ".")
			if _, ok := data[subkey]; ok {
				return nil, configError("dotted key %q conflicts with nested key %q", k, subkey)
			}
			data[subkey] = v
			if removePicked {
				delete(d, k)
			}
		}
	}

	return data, nil
}

// String returns configuration data as a string.
func (d Data) String() string {
	raw, err := yaml.Marshal(d)
	if err != nil {
		return fmt.Sprintf("<config.data: failed to marshal: %v>", err)
	}
	return string(raw)
}

// Print prints the configuration data using the given function or fmt.Printf.
func (d Data) Print(fn func(string, ...interface{})) {
	if fn == nil {
		fn = func(format string, args ...interface{}) {
			fmt.Printf(format+"\n", args...)
		}
	}

	for _, line := range strings.Split(d.String(), "\n") {
		fn("%s", line)
	}
}
