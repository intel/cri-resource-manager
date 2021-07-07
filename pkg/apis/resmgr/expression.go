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

package resmgr

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	logger "github.com/intel/cri-resource-manager/pkg/log"
)

// Evaluable is the interface objects need to implement to be evaluable against Expressions.
type Evaluable interface {
	Eval(string) interface{}
}

// Expression is used to describe a criteria to select objects within a domain.
type Expression struct {
	Key    string   `json:"key"`              // key to check values of/against
	Op     Operator `json:"operator"`         // operator to apply to value of Key and Values
	Values []string `json:"values,omitempty"` // value(s) for domain key
}

const (
	KeyPod       = "pod"
	KeyID        = "id"
	KeyUID       = "uid"
	KeyName      = "name"
	KeyNamespace = "namespace"
	KeyQOSClass  = "qosclass"
	KeyLabels    = "labels"
	KeyTags      = "tags"
)

// Operator defines the possible operators for an Expression.
type Operator string

const (
	// Equals tests for equality with a single value.
	Equals Operator = "Equals"
	// NotEqual test for inequality with a single value.
	NotEqual Operator = "NotEqual"
	// In tests if the key's value is one of the specified set.
	In Operator = "In"
	// NotIn tests if the key's value is not one of the specified set.
	NotIn Operator = "NotIn"
	// Exists evalutes to true if the named key exists.
	Exists Operator = "Exists"
	// NotExist evalutes to true if the named key does not exist.
	NotExist Operator = "NotExist"
	// AlwaysTrue always evaluates to true.
	AlwaysTrue Operator = "AlwaysTrue"
	// Matches tests if the key value matches the only given globbing pattern.
	Matches Operator = "Matches"
	// MatchesNot is true if Matches would be false for the same key and pattern.
	MatchesNot Operator = "MatchesNot"
	// MatchesAny tests if the key value matches any of the given globbing patterns.
	MatchesAny Operator = "MatchesAny"
	// MatchesNone is true if MatchesAny would be false for the same key and patterns.
	MatchesNone Operator = "MatchesNone"
)

// Our logger instance.
var log = logger.NewLogger("expression")

// Validate checks the expression for (obvious) invalidity.
func (e *Expression) Validate() error {
	if e == nil {
		return exprError("nil expression")
	}

	switch e.Op {
	case Equals, NotEqual:
		if len(e.Values) != 1 {
			return exprError("invalid expression, '%s' requires a single value", e.Op)
		}
	case Matches, MatchesNot:
		if len(e.Values) != 1 {
			return exprError("invalid expression, '%s' requires a single value", e.Op)
		}
	case Exists, NotExist:
		if e.Values != nil && len(e.Values) != 0 {
			return exprError("invalid expression, '%s' does not take any values", e.Op)
		}

	case In, NotIn:
	case MatchesAny, MatchesNone:
	case AlwaysTrue:

	default:
		return exprError("invalid expression, unknown operator: %q", e.Op)
	}
	return nil
}

// Evaluate evaluates an expression against a container.
func (e *Expression) Evaluate(subject Evaluable) bool {
	log.Debugf("evaluating %q @ %s...", *e, subject)

	value, ok := e.KeyValue(subject)
	result := false

	switch e.Op {
	case Equals:
		result = ok && (value == e.Values[0] || e.Values[0] == "*")
	case NotEqual:
		result = !ok || value != e.Values[0]
	case Matches, MatchesNot:
		match := false
		if ok {
			match, _ = filepath.Match(e.Values[0], value)
		}
		result = ok && match
		if e.Op == MatchesNot {
			result = !result
		}
	case In, NotIn:
		if ok {
			for _, v := range e.Values {
				if value == v || v == "*" {
					result = true
				}
			}
		}
		if e.Op == NotIn {
			result = !result
		}
	case MatchesAny, MatchesNone:
		if ok {
			for _, pattern := range e.Values {
				if match, _ := filepath.Match(pattern, value); match {
					result = true
					break
				}
			}
		}
		if e.Op == MatchesNone {
			result = !result
		}
	case Exists:
		result = ok
	case NotExist:
		result = !ok
	case AlwaysTrue:
		result = true
	}

	log.Debugf("%q @ %s => %v", *e, subject, result)

	return result
}

// KeyValue extracts the value of the expresssion key from a container.
func (e *Expression) KeyValue(subject Evaluable) (string, bool) {
	log.Debugf("looking up %q @ %s...", e.Key, subject)

	value := ""
	ok := false

	keys, vsep := splitKeys(e.Key)
	if len(keys) == 1 {
		value, ok, _ = ResolveRef(subject, keys[0])
	} else {
		vals := make([]string, 0, len(keys))
		for _, key := range keys {
			v, found, _ := ResolveRef(subject, key)
			vals = append(vals, v)
			ok = ok || found
		}
		value = strings.Join(vals, vsep)
	}

	log.Debugf("%q @ %s => %q, %v", e.Key, subject, value, ok)

	return value, ok
}

func splitKeys(keys string) ([]string, string) {
	// joint key specs have two valid forms:
	//   - ":keylist" (equivalent to ":::<colon-separated-keylist>")
	//   - ":<ksep><vsep><ksep-separated-keylist>"

	if len(keys) < 4 || keys[0] != ':' {
		return []string{keys}, ""
	}

	keys = keys[1:]
	ksep := keys[0:1]
	vsep := keys[1:2]

	if validSeparator(ksep[0]) && validSeparator(vsep[0]) {
		keys = keys[2:]
	} else {
		ksep = ":"
		vsep = ":"
	}

	return strings.Split(keys, ksep), vsep
}

func validSeparator(b byte) bool {
	switch {
	case '0' <= b && b <= '9':
		return false
	case 'a' <= b && b <= 'z':
		return false
	case 'A' <= b && b <= 'Z':
		return false
	case b == '/', b == '.':
		return false
	}
	return true
}

// ResolveRef walks an object trying to resolve a reference to a value.
func ResolveRef(subject Evaluable, spec string) (string, bool, error) {
	var obj interface{}

	log.Debugf("resolving %q @ %s...", spec, subject)

	spec = path.Clean(spec)
	ref := strings.Split(spec, "/")
	if len(ref) == 1 {
		if strings.Index(spec, ".") != -1 {
			ref = []string{"labels", spec}
		}
	}

	obj = subject
	for len(ref) > 0 {
		key := ref[0]

		log.Debugf("resolve walking %q @ %s...", key, obj)
		switch v := obj.(type) {
		case string:
			obj = v
		case map[string]string:
			value, ok := v[key]
			if !ok {
				return "", false, nil
			}
			obj = value
		case error:
			return "", false, exprError("%s: failed to resolve %q: %v", subject, spec, v)
		default:
			e, ok := obj.(Evaluable)
			if !ok {
				return "", false, exprError("%s: failed to resolve %q, unexpected type %T",
					subject, spec, obj)
			}
			obj = e.Eval(key)
		}

		ref = ref[1:]
	}

	str, ok := obj.(string)
	if !ok {
		return "", false, exprError("%s: reference %q resolved to non-string: %T",
			subject, spec, obj)
	}

	log.Debugf("resolved %q @ %s => %s", spec, subject, str)

	return str, true, nil
}

// String returns the expression as a string.
func (e *Expression) String() string {
	return fmt.Sprintf("<%s %s %s>", e.Key, e.Op, strings.Join(e.Values, ","))
}

// DeepCopy creates a deep copy of the expression.
func (e *Expression) DeepCopy() *Expression {
	out := &Expression{}
	e.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the expression into another one.
func (e *Expression) DeepCopyInto(out *Expression) {
	out.Key = e.Key
	out.Op = e.Op
	out.Values = make([]string, len(e.Values))
	copy(out.Values, e.Values)
}

// exprError returns a formatted error specific to expressions.
func exprError(format string, args ...interface{}) error {
	return fmt.Errorf("expression: "+format, args...)
}
