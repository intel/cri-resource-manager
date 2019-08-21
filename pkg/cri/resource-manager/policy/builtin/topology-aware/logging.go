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

package topologyaware

import (
	"fmt"

	logger "github.com/intel/cri-resource-manager/pkg/log"
)

// Create our logger instance.
var log logger.Logger = logger.NewLogger(PolicyName)

// indent produces an indentation string for the given level.
const (
	IndentDepth = 4
)

func indent(prefix string, level ...int) string {
	if len(level) < 1 {
		return prefix
	}

	depth := level[0] * IndentDepth
	return prefix + fmt.Sprintf("%*.*s", depth, depth, "")
}
