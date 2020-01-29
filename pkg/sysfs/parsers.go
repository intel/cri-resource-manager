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

package sysfs

import (
	"errors"
	"io/ioutil"
	"math"
	"sort"
	"strconv"
	"strings"
)

// unit multipliers
const (
	k = (int64(1) << 10)
	M = (int64(1) << 20)
	G = (int64(1) << 30)
	T = (int64(1) << 40)
)

// unit name to multiplier mapping
var units = map[string]int64{
	"k": k, "kB": k,
	"M": M, "MB": M,
	"G": G, "GB": G,
	"T": T, "TB": T,
}

// PickEntryFn picks a given input line apart into an entry of key and value.
type PickEntryFn func(string) (string, string, error)

// SplitFieldsFn picks a given input line apart into a set of key-value pair fields.
type SplitFieldsFn func(string) ([]*Field, error)

// PickFieldFn returns a pointer corresponding to the value of the field.
type PickFieldFn func(int, *Field) (interface{}, error)

// Field is a single field split out by a SplitFieldsFn or to be picked by a PickFieldFn.
type Field struct {
	Index int
	Key   string
	Value string
}

// ErrSkip is used as a error return value for skipping an entry instead of picking/splitting it.
var ErrSkip = errors.New("skip parsing this entry")

// splitNumericAndUnit splits a string into a numeric and a unit part.
func splitNumericAndUnit(path string, value string) (string, int64, error) {
	fields := strings.Fields(value)

	switch len(fields) {
	case 1:
		return fields[0], 1, nil
	case 2:
		num := fields[0]
		unit, ok := units[fields[1]]
		if !ok {
			return "", -1, sysfsError(path, "failed to parse '%s', invalid unit '%s'",
				value, num, unit)
		}
		return num, unit, nil
	}

	return "", -1, sysfsError(path, "invalid numeric value %s", value)
}

// parseNumeric parses a numeric string into integer of the right size.
func parseNumeric(path, value string, ptr interface{}) error {
	var numstr string
	var num, unit int64
	var f float64
	var err error

	if numstr, unit, err = splitNumericAndUnit(path, value); err != nil {
		return err
	}

	switch ptr.(type) {
	case *int:
		num, err = strconv.ParseInt(numstr, 0, strconv.IntSize)
		*ptr.(*int) = int(num * unit)
	case *int8:
		num, err = strconv.ParseInt(numstr, 0, 8)
		*ptr.(*int8) = int8(num * unit)
	case *int16:
		num, err = strconv.ParseInt(numstr, 0, 16)
		*ptr.(*int16) = int16(num * unit)
	case *int32:
		num, err = strconv.ParseInt(numstr, 0, 32)
		*ptr.(*int32) = int32(num * unit)
	case *int64:
		num, err = strconv.ParseInt(numstr, 0, 64)
		*ptr.(*int64) = int64(num * unit)
	case *uint:
		num, err = strconv.ParseInt(numstr, 0, strconv.IntSize)
		*ptr.(*uint) = uint(num * unit)
	case *uint8:
		num, err = strconv.ParseInt(numstr, 0, 8)
		*ptr.(*uint8) = uint8(num * unit)
	case *uint16:
		num, err = strconv.ParseInt(numstr, 0, 16)
		*ptr.(*uint16) = uint16(num * unit)
	case *uint32:
		num, err = strconv.ParseInt(numstr, 0, 32)
		*ptr.(*uint32) = uint32(num * unit)
	case *uint64:
		num, err = strconv.ParseInt(numstr, 0, 64)
		*ptr.(*uint64) = uint64(num * unit)
	case *float32:
		f, err = strconv.ParseFloat(numstr, 32)
		*ptr.(*float32) = float32(f) * float32(unit)
	case *float64:
		f, err = strconv.ParseFloat(numstr, 64)
		*ptr.(*float64) = f * float64(unit)

	default:
		err = sysfsError(path, "can't parse numeric value '%s' into type %T", value, ptr)
	}

	return err
}

// parseIntWithUnit parses an integer multiplied by a unit.
func parseIntWithUnit(path, val string, unit int64) (int64, error) {
	num, err := strconv.ParseInt(val, 0, 64)
	if err != nil {
		return 0, sysfsError(path, "can't parse numeric value '%s': %v", val, err)
	}
	return num * unit, nil
}

// parseUintWithUnit parses an unsigned integer multiplied by a unit.
func parseUintWithUnit(path, val string, unit int64) (uint64, error) {
	num, err := strconv.ParseUint(val, 0, 64)
	if err != nil {
		return 0, sysfsError(path, "can't parse numeric unsigned value '%s': %v", val, err)
	}
	return num * uint64(unit), nil
}

// parseFloatWithUnit parses an integer multiplied by a unit.
func parseFloatWithUnit(path, val string, unit int64) (float64, error) {
	num, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return 0.0, sysfsError(path, "can't parse numeric value '%s': %v", val, err)
	}
	return num * float64(unit), nil
}

// parseNumericMap parses a numeric string into integer of the right size.
func parseNumericMap(path, key, value string, vmap interface{}) error {
	var numstr string
	var num, unit int64
	var unum uint64
	var f float64
	var err error

	if numstr, unit, err = splitNumericAndUnit(path, value); err != nil {
		return err
	}

	switch vm := vmap.(type) {
	case map[string]int8:
		typeName, min, max := "int8", int64(math.MinInt8), int64(math.MaxInt8)
		if num, err = parseIntWithUnit(path, numstr, unit); err != nil {
			return err
		}
		if num < min || num > max {
			return sysfsError(path, "numeric value %s overflows %s", value, typeName)
		}
		vm[key] = int8(num)
	case map[string]uint8:
		typeName, max := "uint8", uint64(math.MaxUint8)
		if unum, err = parseUintWithUnit(path, numstr, unit); err != nil {
			return err
		}
		if unum > max {
			return sysfsError(path, "numeric value %s overflows %s", value, typeName)
		}
		vm[key] = uint8(unum)
	case map[string]int16:
		typeName, min, max := "int16", int64(math.MinInt16), int64(math.MaxInt16)
		if num, err = parseIntWithUnit(path, numstr, unit); err != nil {
			return err
		}
		if num < min || num > max {
			return sysfsError(path, "numeric value %s overflows %s", value, typeName)
		}
		vm[key] = int16(num)
	case map[string]uint16:
		typeName, max := "uint16", uint64(math.MaxUint16)
		if unum, err = parseUintWithUnit(path, numstr, unit); err != nil {
			return err
		}
		if unum > max {
			return sysfsError(path, "numeric value %s overflows %s", value, typeName)
		}
		vm[key] = uint16(unum)
	case map[string]int32:
		typeName, min, max := "int32", int64(math.MinInt32), int64(math.MaxInt32)
		if num, err = parseIntWithUnit(path, numstr, unit); err != nil {
			return err
		}
		if num < min || num > max {
			return sysfsError(path, "numeric value %s overflows %s", value, typeName)
		}
		vm[key] = int32(num)
	case map[string]uint32:
		typeName, max := "uint32", uint64(math.MaxUint32)
		if unum, err = parseUintWithUnit(path, numstr, unit); err != nil {
			return err
		}
		if unum > max {
			return sysfsError(path, "numeric value %s overflows %s", value, typeName)
		}
		vm[key] = uint32(unum)
	case map[string]int64:
		typeName, min, max := "int64", int64(math.MinInt64), int64(math.MaxInt64)
		if num, err = parseIntWithUnit(path, numstr, unit); err != nil {
			return err
		}
		if num < min || num > max {
			return sysfsError(path, "numeric value %s overflows %s", value, typeName)
		}
		vm[key] = num
	case map[string]uint64:
		typeName, max := "uint64", uint64(math.MaxUint64)
		if unum, err = parseUintWithUnit(path, numstr, unit); err != nil {
			return err
		}
		if unum > max {
			return sysfsError(path, "numeric value %s overflows %s", value, typeName)
		}
		vm[key] = unum

	case map[string]float32:
		typeName, min, max := "float32", math.SmallestNonzeroFloat32, math.MaxFloat32
		if f, err = parseFloatWithUnit(path, numstr, unit); err != nil {
			return err
		}
		if f < min || f > max {
			return sysfsError(path, "numeric value %s overflows %s", value, typeName)
		}
		vm[key] = float32(f)
	case map[string]float64:
		typeName, min, max := "float64", math.SmallestNonzeroFloat64, math.MaxFloat64
		if f, err = parseFloatWithUnit(path, numstr, unit); err != nil {
			return err
		}
		if f < min || f > max {
			return sysfsError(path, "numeric value %s overflows %s", value, typeName)
		}
		vm[key] = f

	default:
		err = sysfsError(path, "can't parse '%s' as numeric map value for map %T", value, vmap)
	}

	return err
}

// ParseFileEntries parses a sysfs files for the given entries.
func ParseFileEntries(path string, values map[string]interface{}, pickFn PickEntryFn) error {
	var err error

	data, err := ioutil.ReadFile(path)
	if err != nil {
		sysfsError(path, "failed to read file: %v", err)
	}

	left := len(values)
	for _, line := range strings.Split(string(data), "\n") {
		key, value, err := pickFn(line)
		switch {
		case err == ErrSkip:
			continue
		case err != nil:
			return err
		}

		ptr, ok := values[key]
		if !ok {
			continue
		}

		switch ptr.(type) {
		case *int, *int8, *int32, *int16, *int64, *uint, *uint8, *uint16, *uint32, *uint64:
			if err = parseNumeric(path, value, ptr); err != nil {
				return err
			}
		case *float32, *float64:
			if err = parseNumeric(path, value, ptr); err != nil {
				return err
			}
		case *string:
			*ptr.(*string) = value
		case *bool:
			*ptr.(*bool), err = strconv.ParseBool(value)
			if err != nil {
				return sysfsError(path, "failed to parse line %s, value '%s' for boolean key '%s'",
					line, value, key)
			}
		default:
			return sysfsError(path, "don't know how to parse key '%s' of type %T", key, ptr)

		}

		left--
		if left == 0 {
			break
		}
	}

	return nil
}

// ParseFileByLines parses a sysfs files for the given entries.
func ParseFileByLines(path string, splitFn SplitFieldsFn, pickFn PickFieldFn) error {
	var err error

	data, err := ioutil.ReadFile(path)
	if err != nil {
		sysfsError(path, "failed to read file: %v", err)
	}

	for _, line := range strings.Split(string(data), "\n") {
		fields, err := splitFn(line)
		switch {
		case err == ErrSkip:
			continue
		case err != nil:
			return nil
		}
		// Sort according to caller-provided Index which indicates in which order the caller
		// wants to consume (usually because the caller statefully combines the fields).
		sort.Slice(fields, func(i, j int) bool { return fields[i].Index <= fields[j].Index })

		for idx, f := range fields {
			ptr, err := pickFn(idx, f)
			switch {
			case err == ErrSkip:
				continue
			case err != nil:
				return err
			}

			switch ptr.(type) {
			case *int, *int8, *int32, *int16, *int64, *uint, *uint8, *uint16, *uint32, *uint64:
				if err = parseNumeric(path, f.Value, ptr); err != nil {
					return err
				}
			case *float32, *float64:
				if err = parseNumeric(path, f.Value, ptr); err != nil {
					return err
				}
			case *string:
				*ptr.(*string) = f.Value
			case *bool:
				*ptr.(*bool), err = strconv.ParseBool(f.Value)
				if err != nil {
					return sysfsError(path, "failed to parse field %s, value %s as boolean",
						f.Key, f.Value)
				}

			case map[string]string:
				ptr.(map[string]string)[f.Key] = f.Value

			case map[string]bool:
				var b bool
				ptr.(map[string]string)[f.Key] = f.Value
				b, err = strconv.ParseBool(f.Value)
				if err != nil {
					return sysfsError(path, "failed to parse field %s, value %s as boolean",
						f.Key, f.Value)
				}
				ptr.(map[string]bool)[f.Key] = b

			default:
				if err = parseNumericMap(path, f.Key, f.Value, ptr); err != nil {
					return sysfsError(path, "failed to parse field %s = %s: %v",
						f.Key, f.Value, err)
				}
			}
		}
	}

	return nil
}
