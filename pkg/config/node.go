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
	"reflect"
	"strings"
	"unicode"

	"github.com/hashicorp/go-multierror"
	"sigs.k8s.io/yaml"
)

// A node in our configuration tree.
//
// Leaf nodes represent registered configuration data fragments.
// Other nodes link the fragments together into a tree structure
// according to the names/pathes given to these fragments.
type node struct {
	path     Path             // path from root configuration
	ptr      interface{}      // config fragment registered for this node
	children map[string]*node // child nodes (other fragments with same prefix)
	cfgType  reflect.Type     // constructed struct for this subtree of configuration
	cfgValue reflect.Value    // instantiated value for subtree of configuration
}

// Reset all configuration fragments.
func (n *node) Reset() {
	for _, c := range n.children {
		c.Reset()
	}
	if n.ptr != nil {
		n.ptr.(Fragment).Reset()
	}
}

// Validate all configuration fragments.
func (n *node) Validate() error {
	var errors *multierror.Error

	for _, c := range n.children {
		if err := c.Validate(); err != nil {
			errors = multierror.Append(errors, err)
		}
	}

	if n.ptr != nil {
		if v, ok := n.ptr.(FragmentValidator); ok {
			if err := v.Validate(); err != nil {
				errors = multierror.Append(errors, fmt.Errorf("%q: %w", n.path.String(), err))
			}
		}
	}

	return errors.ErrorOrNil()
}

// SetYAML sets the configuration from the given YAML data.
func (n *node) SetYAML(raw []byte) error {
	if err := n.compile(); err != nil {
		return err
	}

	n.Reset()
	if err := yaml.UnmarshalStrict(raw, n.cfgValue.Interface()); err != nil {
		return err
	}

	if err := n.Validate(); err != nil {
		return err
	}

	return nil
}

// GetYAML returns the current configuration as YAML data.
func (n *node) GetYAML() ([]byte, error) {
	if err := n.compile(); err != nil {
		return nil, err
	}
	return yaml.Marshal(n.cfgValue.Interface())
}

// GetConfig returns the configuration fragment registered for the given path.
func (n *node) GetConfig(path string) (interface{}, bool) {
	p := n
	for _, name := range makePath(path).Canonical() {
		var ok bool
		p, ok = p.children[name]
		if !ok {
			return nil, false
		}
	}
	return p.ptr, p.ptr != nil
}

func newNode(path Path, ptr interface{}) *node {
	return &node{
		path:     path.Clone(),
		ptr:      ptr,
		children: map[string]*node{},
	}
}

func (n *node) depthFirst(fn func(*node, int) error) error {
	if n == nil {
		return nil
	}
	return n.dfWalk(fn, 0)
}

func (n *node) dfWalk(fn func(*node, int) error, level int) error {
	for _, c := range n.children {
		if err := c.dfWalk(fn, level+1); err != nil {
			return err
		}
	}
	return fn(n, level)
}

func (n *node) breadthFirst(fn func(*node, int) error) error {
	if n == nil {
		return nil
	}
	return n.bfWalk(fn, 0)
}

func (n *node) bfWalk(fn func(*node, int) error, level int) error {
	if err := fn(n, level); err != nil {
		return err
	}
	for _, c := range n.children {
		if err := c.bfWalk(fn, level+1); err != nil {
			return err
		}
	}
	return nil
}

func (n *node) isLeaf() bool {
	return len(n.children) == 0
}

func (n *node) add(path Path, ptr interface{}) error {
	if err := path.Validate(); err != nil {
		return err
	}

	p := n
	for idx, name := range path.Canonical() {
		c, ok := p.children[name]
		if !ok {
			c = newNode(path.Sub(0, idx+1), nil)
			p.children[name] = c
		}
		p = c
	}

	if p.ptr != nil {
		return fmt.Errorf("conflict with %q %T", p.path.String(), p.ptr)
	}

	p.path = path
	p.ptr = ptr

	return nil
}

func (n *node) get(path string) *node {
	p := n
	for _, name := range makePath(path).Canonical() {
		c, ok := p.children[name]
		if !ok {
			return nil
		}
		p = c
	}
	return p
}

func (n *node) compile() error {
	var err error

	//
	// Use reflect to construct a pointer to a struct representing the (sub-)
	// configuration at this node. The constructed struct is a straightforward
	// representation of how the registered paths span the fragments into a
	// tree, with the only non-trivial complication being how internal nodes
	// with coniciding registered fragments are represented.
	//
	// The config struct is constructed recursively, bottom-up. For leaf-nodes
	// the registered fragment struct type and pointer are used as such. For
	// internal nodes struct pointer fields are generated for each child node's
	// configuration struct. Additionally, for internal nodes with a coinciding
	// fragment 'embedding' the fragment struct is emulated by generating
	// corresponding pointer struct fields to each field of the fragment itself.
	//

	// construct the struct once (and refuse fragment registration ever after)
	if n.cfgValue.IsValid() {
		return nil
	}

	// leaf node: use the config fragment as such
	if n.isLeaf() {
		n.cfgType = reflect.TypeOf(n.ptr).Elem()
		n.cfgValue = reflect.ValueOf(n.ptr)
		return nil
	}

	// internal node, compile child nodes first
	for _, c := range n.children {
		if err = c.compile(); err != nil {
			return err
		}
	}

	fields := []reflect.StructField{}

	// has fragment: emulate embedding by generating field pointers
	if n.ptr != nil {
		ptrType := reflect.TypeOf(n.ptr).Elem()
		for _, f := range reflect.VisibleFields(ptrType) {
			if !f.IsExported() {
				continue
			}

			if f.Name == "" { // can this happen ?
				return fmt.Errorf("can't handle nameless field #%d (%s, %s.%s) of %q %T",
					f.Index, f.Type.Kind(), f.Type.PkgPath(), f.Type.Name(),
					n.path.String(), n.ptr)
			}

			// use pointer fields as such
			if f.Type.Kind() == reflect.Pointer {
				fields = append(fields, reflect.StructField{
					Name: f.Name,
					Type: f.Type,
					Tag:  f.Tag,
				})
				continue
			}

			// for non-pointer fields, use a pointer to the field
			fields = append(fields, reflect.StructField{
				Name: f.Name,
				Type: reflect.PointerTo(f.Type),
				Tag:  f.Tag,
			})
		}
	}

	// generate fields for child nodes
	for name, c := range n.children {
		fields = append(fields, reflect.StructField{
			Name: name,
			Type: reflect.PointerTo(c.cfgType),
			Tag:  reflect.StructTag(c.path.StructTags()),
		})
	}

	// construct our struct, instantiate it
	n.cfgType = reflect.StructOf(fields)
	n.cfgValue = reflect.New(n.cfgType)

	// populate fields for 'embedded' fragment
	if n.ptr != nil {
		t := reflect.TypeOf(n.ptr).Elem()
		v := reflect.ValueOf(n.ptr).Elem()
		for _, f := range reflect.VisibleFields(t) {
			if !f.IsExported() {
				continue
			}

			if f.Type.Kind() == reflect.Pointer {
				n.cfgValue.Elem().FieldByName(f.Name).Set(v.FieldByName(f.Name))
			} else {
				n.cfgValue.Elem().FieldByName(f.Name).Set(v.FieldByName(f.Name).Addr())
			}
		}
	}

	// populate fields for child nodes
	for name, c := range n.children {
		n.cfgValue.Elem().FieldByName(name).Set(c.cfgValue)
	}

	return nil
}

func (n *node) dump(level int, dumpData bool) string {
	str := ""

	if n.ptr != nil {
		str = fmt.Sprintf("%s%T", indent(level), n.ptr)
		if dumpData {
			data, err := yaml.Marshal(n.ptr)
			if err != nil {
				str += fmt.Sprintf("\n%s| failed to marshal data (%v)\n", indent(level+2), err)
			} else {
				for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
					str += fmt.Sprintf("\n%s| %s", indent(level+2), line)
				}
			}
		}
	}

	for name, c := range n.children {
		str += fmt.Sprintf("%s%s:\n", indent(level), name)
		str += fmt.Sprintf("%s\n", c.dump(level+2, dumpData))
	}

	return str
}

func (n *node) describe() string {
	str := ""
	n.breadthFirst(func(p *node, level int) error {
		if p.ptr != nil {
			str += indent(level) + p.ptr.(Fragment).Describe()
		}
		return nil
	})
	return str
}

func indent(level int) string {
	return fmt.Sprintf("%*s", level, "")
}

const (
	pathSep = "."
	wordSep = "-"
)

type Path []string

func makePath(s string) Path {
	if s == "" {
		return []string{}
	}
	return strings.Split(s, pathSep)
}

func (p Path) Validate() error {
	for _, name := range p {
		if name == "" {
			return fmt.Errorf("invalid path %q, has name", p.String())
		}
	}
	return nil
}

func (p Path) String() string {
	return strings.Join(p, pathSep)
}

func (p Path) Clone() Path {
	c := make([]string, p.Len())
	copy(c, p)
	return Path(c)
}

func (p Path) Sub(beg, end int) Path {
	return Path(p[beg:end])
}

func (p Path) Parent() Path {
	return p.Sub(0, p.Len()-1)
}

func (p Path) Name() string {
	return p[p.Len()-1]
}

func (p Path) FieldName() string {
	return goName(p.Name())
}

func (p Path) StructTags() string {
	return fmt.Sprintf(`json:"%s,omitempty"`, p.FieldName())
}

func (p Path) Canonical() Path {
	c := []string{}
	for _, word := range p {
		c = append(c, goName(word))
	}
	return Path(c)
}

func (p Path) Len() int {
	return len(p)
}

func goName(name string) string {
	b := strings.Builder{}
	for _, p := range strings.Split(name, wordSep) {
		if p == "" {
			continue
		}
		b.WriteByte(byte(unicode.ToUpper(rune(p[0]))))
		b.WriteString(p[1:])
	}
	return b.String()
}
