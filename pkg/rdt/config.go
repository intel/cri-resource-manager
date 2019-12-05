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

package rdt

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/ghodss/yaml"
	pkgcfg "github.com/intel/cri-resource-manager/pkg/config"
)

// ResctrlGroupConfig represents configuration of one CTRL group in the Linux
// resctrl interface
type ResctrlGroupConfig struct {
	L3Schema L3Schema `json:"l3Schema,omitempty"`
	MBSchema MBSchema `json:"mbSchema,omitempty"`
}

// SchemaOptions contains the common settings for all resctrl groups
type SchemaOptions struct {
	L3 L3Options `json:"l3"`
	MB MBOptions `json:"mb"`
}

// L3Options contains the common settings for L3 cache allocation
type L3Options struct {
	Optional bool `json:"optional,omitempty"`
}

// MBOptions contains the common settings for memory bandwidth allocation
type MBOptions struct {
	Optional bool `json:"optional,omitempty"`
}

// L3Schema represents an L3 part of the schemata of a resctrl group
type L3Schema struct {
	Allocations map[uint64]L3Allocation
}

// MBSchema represents an MB part of the schemata of a resctrl group
type MBSchema struct {
	Allocations map[uint64]uint64
}

// L3Allocation represents the L3 cache allocation configuration for one cache id
type L3Allocation struct {
	Unified CacheBitmask `json:"unified"`
	Code    CacheBitmask `json:"code"`
	Data    CacheBitmask `json:"data"`
}

// L3SchemaType represents different L3 cache allocation schemes
type L3SchemaType string

const (
	// L3SchemaTypeUnified is the schema type when CDP is not enabled
	L3SchemaTypeUnified = ""
	// L3SchemaTypeCode is the 'code' part of CDP schema
	L3SchemaTypeCode = "CODE"
	// L3SchemaTypeData is the 'data' part of CDP schema
	L3SchemaTypeData = "DATA"
)

// IsNil returns true if the schema is empty
func (s *L3Schema) IsNil() bool {
	return s.Allocations == nil
}

// ToStr returns the L3 schema in a format accepted by the Linux kernel
// resctrl (schemata) interface
func (s *L3Schema) ToStr(typ L3SchemaType) string {
	if s.IsNil() {
		return s.DefaultStr(typ)
	}

	schema := "L3" + string(typ) + ":"
	sep := ""

	// We get cache ids but that doesn't matter
	for id, masks := range s.Allocations {
		bitmask := masks.Unified
		// Use Unified as the default/fallback for Code and Data
		switch typ {
		case L3SchemaTypeCode:
			if masks.Code != 0 {
				bitmask = masks.Code
			}
		case L3SchemaTypeData:
			if masks.Data != 0 {
				bitmask = masks.Data
			}
		}
		schema += fmt.Sprintf("%s%d=%x", sep, id, bitmask)
		sep = ";"
	}

	return schema + "\n"
}

// DefaultStr returns the L3 default schema
func (s *L3Schema) DefaultStr(typ L3SchemaType) string {
	schema := "L3" + string(typ) + ":"
	sep := ""

	mask := rdtInfo.l3FullMask()

	for _, id := range rdtInfo.cacheIds {
		// Set all to full mask (i.e. 100%)
		schema += fmt.Sprintf("%s%d=%x", sep, id, mask)
		sep = ";"
	}

	return schema + "\n"
}

// UnmarshalJSON implements the Unmarshaler interface of "encoding/json"
func (s *L3Schema) UnmarshalJSON(b []byte) error {
	var allocations map[string]L3Allocation

	err := yaml.Unmarshal(b, &allocations)
	if err != nil {
		return err
	}

	s.Allocations = map[uint64]L3Allocation{}

	// Set default allocations
	fullMask := CacheBitmask(rdtInfo.l3FullMask())
	defaultAllocation := L3Allocation{Unified: fullMask, Code: fullMask, Data: fullMask}
	if val, ok := allocations["all"]; ok {
		defaultAllocation = val
		delete(allocations, "all")
	}

	for _, i := range rdtInfo.cacheIds {
		s.Allocations[i] = defaultAllocation
	}

	// Parse per-cacheId allocations
	for key, allocation := range allocations {
		ids, err := listStrToArray(key)
		if err != nil {
			return err
		}
		for _, id := range ids {
			if _, ok := s.Allocations[uint64(id)]; ok {
				s.Allocations[uint64(id)] = allocation
			}
		}
	}

	return nil
}

// UnmarshalJSON implements the Unmarshaler interface of "encoding/json"
func (a *L3Allocation) UnmarshalJSON(b []byte) error {
	type l3Intermediate L3Allocation
	var tmp l3Intermediate

	err := yaml.Unmarshal(b, &tmp)
	if err != nil {
		return err
	}

	if tmp.Unified == 0 {
		return fmt.Errorf("'unified' not specified in l3Schema %s", b)
	}
	if tmp.Code != 0 && tmp.Data == 0 {
		return fmt.Errorf("'code' specified but missing 'data' from l3Schema %s", b)
	}
	if tmp.Code == 0 && tmp.Data != 0 {
		return fmt.Errorf("'data' specified but missing 'code' from l3Schema %s", b)
	}

	*a = L3Allocation(tmp)
	return nil
}

// IsNil returns true if the schema is empty
func (s *MBSchema) IsNil() bool {
	return s.Allocations == nil
}

// ToStr returns the MB schema in a format accepted by the Linux kernel
// resctrl (schemata) interface
func (s *MBSchema) ToStr() string {
	if s.IsNil() {
		return s.DefaultStr()
	}

	schema := "MB:"
	sep := ""

	// We get cache ids but that doesn't matter
	for id, percentage := range s.Allocations {
		schema += fmt.Sprintf("%s%d=%d", sep, id, percentage)
		sep = ";"
	}

	return schema + "\n"
}

// DefaultStr returns the L3 default schema
func (s *MBSchema) DefaultStr() string {
	schema := "MB:"
	sep := ""

	for _, id := range rdtInfo.cacheIds {
		// Set all to 100 percent
		schema += fmt.Sprintf("%s%d=100", sep, id)
		sep = ";"
	}

	return schema + "\n"
}

// UnmarshalJSON implements the Unmarshaler interface of "encoding/json"
func (s *MBSchema) UnmarshalJSON(b []byte) error {
	var allocations map[string]uint64

	err := yaml.Unmarshal(b, &allocations)
	if err != nil {
		return err
	}

	s.Allocations = map[uint64]uint64{}

	// Set default allocations
	defaultVal, ok := allocations["all"]
	if !ok {
		// Set to 100 if "all" is not specified
		defaultVal = 100
	}
	delete(allocations, "all")

	for _, i := range rdtInfo.cacheIds {
		s.Allocations[i] = defaultVal
	}

	// Parse per-cacheId allocations
	for key, val := range allocations {
		ids, err := listStrToArray(key)
		if err != nil {
			return err
		}
		for _, id := range ids {
			if _, ok := s.Allocations[uint64(id)]; ok {
				s.Allocations[uint64(id)] = val
			}
		}
	}

	return nil
}

// listStrToArray parses a string containing a human-readable list of numbers
// into an integer array
func listStrToArray(str string) ([]int, error) {
	a := []int{}

	// Empty list
	if len(str) == 0 {
		return a, nil
	}

	ranges := strings.Split(str, ",")
	for _, ran := range ranges {
		split := strings.SplitN(ran, "-", 2)

		// We limit to 8 bits in order to avoid accidental super long slices
		num, err := strconv.ParseInt(split[0], 10, 8)
		if err != nil {
			return a, rdtError("invalid integer %q: %v", str, err)
		}

		if len(split) == 1 {
			a = append(a, int(num))
		} else {
			endNum, err := strconv.ParseInt(split[1], 10, 8)
			if err != nil {
				return a, rdtError("invalid integer in range %q: %v", str, err)
			}
			if endNum <= num {
				return a, rdtError("invalid integer range %q in %q", ran, str)
			}
			for i := num; i <= endNum; i++ {
				a = append(a, int(i))
			}
		}
	}
	sort.Ints(a)
	return a, nil
}

// Options captures our configurable parameters.
type options struct {
	// Config is our RDT configuration.
	Config config `json:"config,omitempty"`
}

// Our runtime configuration.
var opt = defaultOptions().(*options)

// defaultOptions returns a new config instance, all initialized to defaults.
func defaultOptions() interface{} {
	return &options{}
}

// Register us for configuration handling.
func init() {
	pkgcfg.Register("rdt", "RDT control", opt, defaultOptions)
}
