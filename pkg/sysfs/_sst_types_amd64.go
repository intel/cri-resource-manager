// +build amd64
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

// This file is used for auto-generation of sst_types_amd64.go
package sysfs

// #include <linux/isst_if.h>
// #include <linux/ioctl.h>
//
import "C"

const (
	ISST_IF_GET_PHY_ID   = C.ISST_IF_GET_PHY_ID
	ISST_IF_IO_CMD       = C.ISST_IF_IO_CMD
	ISST_IF_MBOX_COMMAND = C.ISST_IF_MBOX_COMMAND
)

type isstIfCPUMaps C.struct_isst_if_cpu_maps
type isstIfCPUMap C.struct_isst_if_cpu_map

type isstIfIoReg C.struct_isst_if_io_reg
type isstIfIoRegs C.struct_isst_if_io_regs

type isstIfMboxCmd C.struct_isst_if_mbox_cmd
type isstIfMboxCmds C.struct_isst_if_mbox_cmds
