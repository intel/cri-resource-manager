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

package log

import (
	"fmt"
	"strconv"
)

// Delayed implements delayed evaluation (can lower the overhead of suppressed log.Debug).
type Delayed interface {
	String() string
}

// delay implements Delayed.
type delay struct {
	o interface{}
}

// Delay wraps its argument for delayed .String() evaluation.
func Delay(o interface{}) Delayed {
	return &delay{o: o}
}

// String implements stringification of its delayed argument.
func (d *delay) String() string {
	o := d.o
	switch o.(type) {
	case func() string:
		return o.(func() string)()
	case func() interface{}:
		o = o.(func() interface{})()
	}

	switch o.(type) {
	case string:
		return o.(string)
	case int8:
		return strconv.FormatInt(int64(o.(int8)), 10)
	case int16:
		return strconv.FormatInt(int64(o.(int16)), 10)
	case int32:
		return strconv.FormatInt(int64(o.(int32)), 10)
	case int64:
		return strconv.FormatInt(o.(int64), 10)
	case uint8:
		return strconv.FormatUint(uint64(o.(uint8)), 10)
	case uint16:
		return strconv.FormatUint(uint64(o.(uint16)), 10)
	case uint32:
		return strconv.FormatUint(uint64(o.(uint32)), 10)
	case uint64:
		return strconv.FormatUint(o.(uint64), 10)
	case float32:
		return strconv.FormatFloat(float64(o.(float32)), 'f', -1, 32)
	case float64:
		return strconv.FormatFloat(o.(float64), 'f', -1, 64)
	case bool:
		return strconv.FormatBool(o.(bool))
	default:
		return fmt.Sprintf("%v", o)
	}
}
