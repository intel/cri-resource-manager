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

package config

//  RawConfig represents the resource manager config data in unparsed form, as
//  received from the agent
type RawConfig struct {
	NodeName string
	Data     map[string]string
}

// HasIdenticalData returns true if RawConfig has identical data to the supplied one.
func (c *RawConfig) HasIdenticalData(data map[string]string) bool {
	if c == nil && data == nil {
		return true
	}
	if c == nil || data == nil {
		return false
	}

	if len(c.Data) != len(data) {
		return false
	}

	for k, v := range c.Data {
		if dv, found := data[k]; !found || dv != v {
			return false
		}
	}

	for dk, dv := range data {
		if v, found := c.Data[dk]; !found || v != dv {
			return false
		}
	}

	return true
}
