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
	"reflect"
	"sort"
	"strings"
)

// Describe provides help about configuration of the given modules.
func Describe(names ...string) {
	modules := findModules(names, nil)

	if len(modules) == 0 {
		fmt.Printf("No matching modules found.\n")
		return
	}

	for _, m := range modules {
		m.showHelp()
		fmt.Printf("\n\n")
	}
}

func (m *Module) setDescription(description string) {
	description = strings.Trim(description, "\n")

	if description == "" {
		m.description = "Module " + m.path + " has no description."
		return
	}

	if strings.IndexByte(description, '\n') == -1 {
		m.description = description
	} else {
		lines := strings.Split(description, "\n")
		m.description = lines[0]
		m.help = strings.Trim(strings.Join(lines[1:], "\n"), "\n")
	}
}

func (m *Module) showHelp() {
	kind := "module"
	if m.isImplicit() {
		kind = "implicit module"
	}
	fmt.Printf("- %s %s: %s\n", kind, m.name, m.description)
	fmt.Printf("  full path: %s\n", m.path)
	if len(m.children) > 0 {
		submodules, sep := "", ""
		for _, child := range m.children {
			submodules += sep + child.path
			sep = ", "
		}
		fmt.Printf("  sub-modules: %s\n", submodules)
	}
	fmt.Printf("  description:\n")
	if m.help != "" {
		fmt.Printf("\n")
		for _, line := range strings.Split(m.help, "\n") {
			fmt.Printf("    %s\n", line)
		}
	} else {
		m.describeData()
	}
}

func (m *Module) describeData() {
	if m.isImplicit() {
		return
	}

	cfg := reflect.ValueOf(m.ptr).Elem()
	fmt.Printf("    No runtime configuration documentation for this package...\n")
	fmt.Printf("    Package runtime configuration data type: %s %s.\n",
		cfg.Type().Kind().String(), cfg.Type().String())
}

func findModules(names []string, m *Module) []*Module {
	if m == nil {
		m = main
	}

	matches := []*Module{}

	if len(names) == 0 {
		matches = append(matches, m)
	} else {
		for _, name := range names {
			switch {
			case name == m.name || name == m.path:
				matches = append(matches, m)
			case name[0] == '.' && name[len(name)-1] == '.' && strings.Index(m.path, name) > 0:
				matches = append(matches, m)
			case name[0] == '.' && strings.HasSuffix(m.path, name):
				matches = append(matches, m)
			case name[len(name)-1] == '.' && strings.HasPrefix(m.path, name):
				matches = append(matches, m)
			}
		}
	}

	children := []*Module{}
	for _, child := range m.children {
		children = append(children, child)
	}
	sort.Slice(children,
		func(i, j int) bool {
			return strings.Compare(children[i].path, children[j].path) < 0
		},
	)
	for _, child := range children {
		matches = append(matches, findModules(names, child)...)
	}

	return matches
}
