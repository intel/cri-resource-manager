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
	"encoding/json"
	"fmt"
)

// FillMethod specifies the order in which balloon instances should be filled.
type FillMethod int

const (
	FillUnspecified FillMethod = iota
	// FillBalanced: put a container into the balloon with most
	// free CPU without changing the size of the balloon.
	FillBalanced
	// FillBalancedInflate: put a container into the balloon with
	// most free CPU when the balloon is inflated to the maximum
	// size.
	FillBalancedInflate
	// FillPacked: put a container into a balloon so that it
	// minimizes the amount of currently unused CPUs in the
	// balloon.
	FillPacked
	// FillPackedInflate: put a container into a balloon so that
	// it minimizes the amount of unused CPUs if the balloon is
	// inflated to the maximum size.
	FillPackedInflate
	// FillSameNamespace: put a container into a balloon that already
	// includes another container from the same namespace
	FillSameNamespace
	// FillSamePod: put a container into a balloon that already
	// includes another container from the same pod.
	FillSamePod
	// FillNewBalloon: create a new balloon, if possible, and put
	// a container into it.
	FillNewBalloon
	// FillNewBalloonMust: create a new balloon for a container,
	// but refuse to run the container if the balloon cannot be
	// created.
	FillNewBalloonMust
	// FillReservedBalloon: put a container into the reserved
	// balloon.
	FillReservedBalloon
	// FillDefaultBalloon: put a container into the default
	// balloon.
	FillDefaultBalloon
)

var fillMethodNames = map[FillMethod]string{
	FillUnspecified:     "unspecified",
	FillBalanced:        "balanced",
	FillBalancedInflate: "balanced-inflate",
	FillPacked:          "packed",
	FillPackedInflate:   "packed-inflate",
	FillSameNamespace:   "same-namespace",
	FillSamePod:         "same-pod",
	FillNewBalloon:      "new-balloon",
	FillNewBalloonMust:  "new-balloon-must",
	FillDefaultBalloon:  "default-balloon",
	FillReservedBalloon: "reserved-balloon",
}

// String stringifies a FillMethod
func (fm FillMethod) String() string {
	if fmn, ok := fillMethodNames[fm]; ok {
		return fmn
	}
	return fmt.Sprintf("#UNNAMED-FILLMETHOD(%d)", int(fm))
}

// MarshalJSON marshals a FillMethod as a quoted json string
func (fm FillMethod) MarshalJSON() ([]byte, error) {
	buffer := bytes.NewBufferString(fmt.Sprintf("%q", fm))
	return buffer.Bytes(), nil
}

// UnmarshalJSON unmarshals a FillMethod quoted json string to the enum value
func (fm *FillMethod) UnmarshalJSON(b []byte) error {
	var fillMethodName string
	err := json.Unmarshal(b, &fillMethodName)
	if err != nil {
		return err
	}
	for fmID, fmName := range fillMethodNames {
		if fmName == fillMethodName {
			*fm = fmID
			return nil
		}
	}
	return balloonsError("invalid fill method %q", fillMethodName)
}
