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

package sysfs

//go:generate ./gen_sst_types.sh

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// cpuMap holds the logical to punit cpu mapping table
var cpuMap = make(map[ID]ID)

// punitCPU returns the PUNIT CPU id corresponding a given Linux logical CPU
func punitCPU(cpu ID) (ID, error) {
	if id, ok := cpuMap[cpu]; ok {
		return id, nil
	}

	id, err := getCPUMapping(cpu)
	if err == nil {
		cpuMap[cpu] = id
	}
	return id, err
}

// isstIoctl is a helper for executing ioctls on the linux isst_if device driver
func isstIoctl(ioctl uintptr, req uintptr) error {
	f, err := os.Open(isstDevPath)
	if err != nil {
		return fmt.Errorf("failed to open isst device %q: %v", isstDevPath, err)
	}
	defer f.Close()

	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(f.Fd()), ioctl, req); errno != 0 {
		return errno
	}

	return nil
}

// getCPUMapping gets mapping of Linux logical CPU numbers to (package-specific)
// PUNIT CPU number for one cpu
func getCPUMapping(cpu ID) (ID, error) {
	req := isstIfCPUMaps{
		Cmd_count: 1,
		Cpu_map: [1]isstIfCPUMap{
			{Logical_cpu: uint32(cpu)},
		},
	}

	if err := isstIoctl(ISST_IF_GET_PHY_ID, uintptr(unsafe.Pointer(&req))); err != nil {
		return -1, fmt.Errorf("failed to get CPU mapping for cpu %d: %v", cpu, err)
	}

	return ID(req.Cpu_map[0].Physical_cpu), nil
}

// sendMboxCmd sends one mailbox command to PUNIT
func sendMboxCmd(cpu ID, cmd uint16, subCmd uint16, reqData uint32) (uint32, error) {
	req := isstIfMboxCmds{
		Cmd_count: 1,
		Mbox_cmd: [1]isstIfMboxCmd{
			{
				Logical_cpu: uint32(cpu),
				Command:     cmd,
				Sub_command: subCmd,
				Req_data:    reqData,
			},
		},
	}

	sstlog.Debug("MBOX SEND cpu: %d cmd: %#02x sub: %#02x data: %#x", cpu, cmd, subCmd, reqData)
	if err := isstIoctl(ISST_IF_MBOX_COMMAND, uintptr(unsafe.Pointer(&req))); err != nil {
		return 0, fmt.Errorf("Mbox command failed with %v", err)
	}
	sstlog.Debug("MBOX RECV data: %#x", req.Mbox_cmd[0].Resp_data)

	return req.Mbox_cmd[0].Resp_data, nil
}

// sendMMIOCmd sends one MMIO command to PUNIT
func sendMMIOCmd(cpu ID, reg uint32) (uint32, error) {
	req := isstIfIoRegs{
		Req_count: 1,
		Io_reg: [1]isstIfIoReg{
			{
				Logical_cpu: uint32(cpu),
				Reg:         reg,
			},
		},
	}
	sstlog.Debug("MMIO SEND cpu: %d reg: %#x", cpu, reg)
	if err := isstIoctl(ISST_IF_IO_CMD, uintptr(unsafe.Pointer(&req))); err != nil {
		return 0, fmt.Errorf("MMIO command failed with %v", err)
	}
	sstlog.Debug("MMIO RECV data: %#x", req.Io_reg[0].Value)

	return req.Io_reg[0].Value, nil
}
