//go:build linux
// +build linux

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

package memtier

import "C"

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

func PidfdOpenSyscall(pid int, flags uint) (int, error) {
	var err error
	// syscall:
	// int syscall(SYS_pidfd_open, pid_t pid, unsigned int flags);
	ret, _, en := unix.Syscall(unix.SYS_PIDFD_OPEN, uintptr(pid), uintptr(flags), 0)
	if en != 0 {
		err = unix.Errno(en)
	}
	return int(ret), err
}

func PidfdCloseSyscall(pidfd int) error {
	return unix.Close(pidfd)
}

type cIovec struct {
	iovBase uint64
	iovLen  C.size_t
}

func ProcessMadviseSyscall(pidfd int, ranges []AddrRange, advise int, flags uint) (int, syscall.Errno, error) {

	// syscall:
	// ssize_t syscall(SYS_process_madvise, int pidfd,
	//                 const struct iovec *iovec, size_t vlen, int advise,
	//                 unsigned int flags);
	// where:
	// struct iovec {
	//         void  *iov_base;    /* Starting address */
	//         size_t iov_len;     /* Length of region */
	// };

	var err error

	iovec := make([]cIovec, len(ranges))
	for i, r := range ranges {
		iovec[i].iovBase = r.addr
		iovec[i].iovLen = C.size_t(r.length * constUPagesize)
	}
	iovecPtr := uintptr(unsafe.Pointer(&iovec[0]))
	iovecLen := uintptr(len(iovec))

	ret, _, en := unix.Syscall6(unix.SYS_PROCESS_MADVISE, uintptr(pidfd), iovecPtr, iovecLen, uintptr(advise), uintptr(flags), 0)
	if en != 0 {
		err = unix.Errno(en)
	}

	return int(ret), en, err
}
