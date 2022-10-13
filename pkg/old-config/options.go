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

// WithNotify specifies a notification function to be called after configuration updates.
func WithNotify(fn NotifyFn) Option {
	return newFuncOption(func(o interface{}) error {
		switch o.(type) {
		case *Module:
			m := o.(*Module)
			m.notifiers = append(m.notifiers, fn)
		default:
			return configError("WithNotify is not valid for object of type %T", o)
		}
		return nil
	})
}

// WithoutDataValidation specifies that data passed to this module should not be validated.
func WithoutDataValidation() Option {
	return newFuncOption(func(o interface{}) error {
		switch o.(type) {
		case *Module:
			m := o.(*Module)
			m.noValidate = true
		default:
			return configError("WithoutDataValidation is not valid for object of type %T", o)
		}
		return nil
	})
}

// Option is the generic interface for any option applicable to a Module or Config.
type Option interface {
	apply(interface{}) error
}

// funcOption is a generic functional option.
type funcOption struct {
	f func(interface{}) error
}

// apply applies a functional option to an object.
func (fo *funcOption) apply(o interface{}) error {
	return fo.f(o)
}

// newFuncOption creates a new option instance.
func newFuncOption(f func(interface{}) error) *funcOption {
	return &funcOption{
		f: f,
	}
}
