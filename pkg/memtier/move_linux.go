//go:build linux
// +build linux

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

import "C"

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/unix"
)

func movePagesSyscall(pid int, count uint, pages []uintptr, nodes []int, flags int) (uint, []int, error) {

	// syscall:
	// long move_pages(int pid, unsigned long count, void **pages,
	//                 const int *nodes, int *status, int flags);

	var err error

	if count == 0 {
		return 0, []int{}, nil
	}

	// Go int is 64 bits on a 64-bit system, but C int is only guaranteed to be at least 16 bits, typically 32.
	cNodes := make([]C.int, len(nodes))
	for i := 0; i < len(nodes); i++ {
		if nodes[i] < 0 || nodes[i] > 32767 {
			return 0, []int{}, fmt.Errorf("int value error: %d", nodes[i])
		}
		cNodes[i] = C.int(nodes[i]) // safe downcast
	}

	cStatus := make([]C.int, len(pages))

	nodesPtr := unsafe.Pointer(nil)
	if nodes != nil {
		nodesPtr = unsafe.Pointer(&cNodes[0])
	}

	ret, _, en := unix.Syscall6(unix.SYS_MOVE_PAGES, uintptr(pid), uintptr(count), uintptr(unsafe.Pointer(&pages[0])), uintptr(nodesPtr), uintptr(unsafe.Pointer(&cStatus[0])), uintptr(flags))
	if en != 0 {
		err = unix.Errno(en)
	}

	// log.Debug("move_pages(): pid %d, count %d, pages %v, nodes %v, flags %d: return value %d, status %d, errno %v",
	// 	pid, count, pages, nodes, flags, uint(ret), cStatus, err)

	status := make([]int, count)
	for i := uint(0); i < count; i++ {
		status[i] = int(cStatus[i])
	}

	return uint(ret), status, err
}
