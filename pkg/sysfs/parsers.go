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
	"io/ioutil"
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

// PparseNumberic parses a numeric string into integer of the right size.
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
		if err != nil {
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
