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

package klogcontrol

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"k8s.io/klog/v2"
)

// Options captures runtime configuration for klog.
type Options map[string]interface{}

// Control implements runtime control for klog.
type Control struct {
	flags *flag.FlagSet
}

// Our singleton klog Control instance.
var ctl *Control

// Get returns our singleton klog Control instance.
func Get() *Control {
	return ctl
}

// CurrentOptions returns the current klog configuration as Options.
func (c *Control) CurrentOptions() Options {
	o := make(Options)
	c.flags.VisitAll(func(f *flag.Flag) {
		o[f.Name] = flag.Lookup(f.Name).Value.(flag.Getter).Get()
	})
	return o
}

// Configure reconfigures klog with the given Options.
func (c *Control) Configure(options Options) error {
	for name, value := range options {
		if err := flag.Set(name, fmt.Sprintf("%v", value)); err != nil {
			return klogError("failed to set klog flag %q to %v: %v", name, value, err)
		}
	}
	return nil
}

// Set sets the value of the given klog flag.
func (c *Control) Set(name, value string) error {
	return flag.Set(name, value)
}

// Get returns the current value of the given klog flag.
func (c *Control) Get(name string) (interface{}, error) {
	if c.flags.Lookup(name) == nil {
		return nil, klogError("unknown klog flag %q", name)
	}
	return flag.Lookup(name).Value.(flag.Getter).Get(), nil
}

// CloneFrom clones src to o.
func (o *Options) CloneFrom(src Options) {
	*o = make(Options)
	for name, value := range src {
		(*o)[name] = value
	}
}

// String returns a string representation of the Options.
func (o *Options) String() string {
	if o == nil {
		return "<nil>"
	}
	str := ""
	sep := ""
	for name, value := range *o {
		str += sep + name + "=" + fmt.Sprintf("%v", value)
		sep = "\n"
	}
	return str
}

// klogflag wraps a klog flag for configuration.
type klogflag struct {
	flag *flag.Flag
}

// Set implements flag.Value.Set() for wrapped klog flags.
func (klogf *klogflag) Set(value string) error {
	if klogf.flag.Name == "stderrthreshold" { // klog expects thresholds in ALL CAPS
		value = strings.ToUpper(value)
	}
	if err := klogf.flag.Value.Set(value); err != nil {
		return err
	}
	return nil
}

// String implements flag.Value.String() for wrapped klog flags.
func (klogf *klogflag) String() string {
	if klogf.flag == nil { // flag.isZeroValue() probing us...
		return ""
	}
	value := klogf.flag.Value.String()
	if klogf.flag.Name == "log_backtrace_at" && value == ":0" {
		value = ""
	}
	return value
}

// Get implements flag.Getter.Get() for wrapped klog flags.
func (klogf *klogflag) Get() interface{} {
	if getter, ok := klogf.flag.Value.(flag.Getter); ok {
		if value := getter.Get(); value != nil {
			return value
		}
	}
	return klogf.String()
}

// boolFlag is identical to the unexported flag.boolFlag interface.
type boolFlag interface {
	IsBoolFlag() bool
}

// IsBoolFlag implements flag.boolFlag.IsBoolFlag() for wrapped klog flags.
func (klogf *klogflag) IsBoolFlag() bool {
	if klogf.flag == nil {
		return false
	}
	if boolf, ok := klogf.flag.Value.(boolFlag); ok {
		return boolf.IsBoolFlag()
	}
	return false
}

// getEnv returns a default value for the flag from the environment.
func (klogf *klogflag) getEnv() (string, string, bool) {
	name := "LOGGER_" + strings.ToUpper(strings.ReplaceAll(klogf.flag.Name, "-", "_"))
	if value, ok := os.LookupEnv(name); ok {
		return name, value, true
	}
	return "", "", false
}

// klogError returns a package-specific formatted error.
func klogError(format string, args ...interface{}) error {
	return fmt.Errorf("klogcontrol: "+format, args...)
}

// wrapKlogFlag wraps and registers the given klog flag.
func wrapKlogFlag(f *flag.Flag) {
	klogf := &klogflag{flag: f}
	flag.Var(klogf, f.Name, f.Usage)

	if name, value, ok := klogf.getEnv(); ok {
		if err := klogf.Set(value); err != nil {
			klog.Errorf("klog flag %q: invalid environment default %s=%q: %v",
				f.Name, name, value, err)
		}
	}
}

// init discovers klog flags and sets up dynamic control for them.
func init() {
	ctl = &Control{flags: flag.NewFlagSet("klog flags", flag.ContinueOnError)}
	ctl.flags.SetOutput(ioutil.Discard)
	klog.InitFlags(ctl.flags)
	ctl.flags.VisitAll(func(f *flag.Flag) {
		wrapKlogFlag(f)
	})
}
