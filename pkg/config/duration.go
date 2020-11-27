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

package config

import (
	"fmt"
	"time"
)

// Duration is a time.Duration which implements JSON marshalling/unmarshalling.
type Duration time.Duration

// MarshalJSON is the JSON marshaller for (time.)Duration.
func (d Duration) MarshalJSON() ([]byte, error) {
	return []byte("\"" + time.Duration(d).String() + "\""), nil
}

// UnmarshalJSON is the JSON unmarshaller for (time.)Duration.
func (d *Duration) UnmarshalJSON(data []byte) error {
	if len(data) < 2 {
		return fmt.Errorf("invalid Duration data")
	}
	parsed, err := time.ParseDuration(string(data[1 : len(data)-1]))
	if err != nil {
		return err
	}
	*d = Duration(parsed)
	return nil
}

// String returns the value of Duration as a string.
func (d *Duration) String() string {
	return time.Duration(*d).String()
}
