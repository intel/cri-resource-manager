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

package balloons

import (
	"bytes"
	"fmt"
	"strconv"
)

// Limit is an integer where negative numbers have special meanings.
type Limit int

const (
	Unlimited            Limit = -1
	greatestInvalidLimit       = -2
)

var limitToString = map[Limit]string{
	Unlimited: "unlimited",
}

// String stringifies a Limit
func (lm Limit) String() string {
	if lmStr, ok := limitToString[lm]; ok {
		return lmStr
	}
	return fmt.Sprintf("%d", int(lm))
}

// MarshalJSON marshals a Limit as a quoted json string
func (lm Limit) MarshalJSON() ([]byte, error) {
	buffer := bytes.NewBufferString("\"" + lm.String() + "\"")
	return buffer.Bytes(), nil
}

// UnmarshalJSON unmarshals a JSON string to Limit
func (lm *Limit) UnmarshalJSON(b []byte) error {
	if len(b) == 0 {
		return balloonsError("invalid limit (empty string)")
	}
	if b[0] >= '0' && b[0] <= '9' {
		lmInt, err := strconv.Atoi(string(b))
		*lm = Limit(lmInt)
		return err
	}
	bStr := string(b[1 : len(b)-1])
	for lmConst, lmString := range limitToString {
		if lmString == bStr {
			*lm = lmConst
			return nil
		}
	}
	return balloonsError("invalid limit %q", string(b))
}
