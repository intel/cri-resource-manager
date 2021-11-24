// Copyright 2021 Intel Corporation. All Rights Reserved.
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

package memtier

import (
	"os"
)

const (
	// Requirements for pagemap bits
	PMPresentSet uint64 = 1 << iota
	PMPresentCleared
	PMExclusiveSet
	PMExclusiveCleared
	PMFileSet
	PMFileCleared
	PMSwapSet
	PMSwapCleared
	PMDirtySet
	PMDirtyCleared

	// addressRangeAttributes
	RangeIsAnonymous = 1 << iota
	RangeIsHeap

	// move_pages syscall flags
	// MPOL_MF_MOVE - only move pages exclusive to this process.
	MPOL_MF_MOVE = 1 << 1
)

var constPagesize int64 = int64(os.Getpagesize())
var constUPagesize uint64 = uint64(constPagesize)
