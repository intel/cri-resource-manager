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

// WithNotify injects an update notification callback into a configuration or module.
func WithNotify(fn NotifyFn) Option {
	return newFuncOption(func(o interface{}) error {
		switch o.(type) {
		case *Config:
			c := o.(*Config)
			c.notify = append(c.notify, fn)
		case *Module:
			m := o.(*Module)
			m.notify = append(m.notify, fn)
		default:
			return configError("WithNotify is not valid option for object of type %T", o)
		}
		return nil
	})
}

// parentName is a type for digging out the parent config name from module options.
type parentName string

// WithConfig specifies which configuration collection a module should be inserted into.
func WithConfig(name string) Option {
	return newFuncOption(func(o interface{}) error {
		switch o.(type) {
		case *Module:
			o.(*Module).parent = GetConfig(name)
		case *parentName:
			*o.(*string) = name
		default:
			return configError("WithConfig is not valid option for object of type %T", o)
		}
		return nil
	})
}

// WithErrorHandling sets the error handling strategy for *Parse() and Restore().
func WithErrorHandling(eh ErrorHandling) Option {
	return newFuncOption(func(o interface{}) error {
		switch o.(type) {
		case *Config:
			o.(*Config).onError = eh
		case *Module:
			o.(*Module).onError = eh
		default:
			return configError("WithErrorHandling is not valid option for object of type %T", o)
		}
		return nil
	})
}

// WithUnsetVarsKept() requests a module to keep its unset values across configuration changes.
func WithUnsetVarsKept() Option {
	return newFuncOption(func(o interface{}) error {
		switch o.(type) {
		case *Module:
			o.(*Module).keepUnset = true
		case *Config:
			o.(*Config).keepUnset = true
		default:
			return configError("WithUnsetVarsKept is not valid for object of type %T", o)
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
