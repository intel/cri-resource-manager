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

// This file is used for auto-generation of sst_types_priv.go
package sysfs

// #include "tools/power/x86/intel-speed-select/isst.h"
//
import "C"

const (
	// TDP (perf profile) related commands
	CONFIG_TDP                        = C.CONFIG_TDP
	CONFIG_TDP_GET_LEVELS_INFO        = C.CONFIG_TDP_GET_LEVELS_INFO
	CONFIG_TDP_GET_TDP_CONTROL        = C.CONFIG_TDP_GET_TDP_CONTROL
	CONFIG_TDP_SET_TDP_CONTROL        = C.CONFIG_TDP_SET_TDP_CONTROL
	CONFIG_TDP_GET_TDP_INFO           = C.CONFIG_TDP_GET_TDP_INFO
	CONFIG_TDP_GET_PWR_INFO           = C.CONFIG_TDP_GET_PWR_INFO
	CONFIG_TDP_GET_TJMAX_INFO         = C.CONFIG_TDP_GET_TJMAX_INFO
	CONFIG_TDP_GET_CORE_MASK          = C.CONFIG_TDP_GET_CORE_MASK
	CONFIG_TDP_GET_TURBO_LIMIT_RATIOS = C.CONFIG_TDP_GET_TURBO_LIMIT_RATIOS
	CONFIG_TDP_SET_LEVEL              = C.CONFIG_TDP_SET_LEVEL
	CONFIG_TDP_GET_UNCORE_P0_P1_INFO  = C.CONFIG_TDP_GET_UNCORE_P0_P1_INFO
	CONFIG_TDP_GET_P1_INFO            = C.CONFIG_TDP_GET_P1_INFO
	CONFIG_TDP_GET_MEM_FREQ           = C.CONFIG_TDP_GET_MEM_FREQ

	CONFIG_TDP_GET_FACT_HP_TURBO_LIMIT_NUMCORES = C.CONFIG_TDP_GET_FACT_HP_TURBO_LIMIT_NUMCORES
	CONFIG_TDP_GET_FACT_HP_TURBO_LIMIT_RATIOS   = C.CONFIG_TDP_GET_FACT_HP_TURBO_LIMIT_RATIOS
	CONFIG_TDP_GET_FACT_LP_CLIPPING_RATIO       = C.CONFIG_TDP_GET_FACT_LP_CLIPPING_RATIO

	CONFIG_TDP_PBF_GET_CORE_MASK_INFO = C.CONFIG_TDP_PBF_GET_CORE_MASK_INFO
	CONFIG_TDP_PBF_GET_P1HI_P1LO_INFO = C.CONFIG_TDP_PBF_GET_P1HI_P1LO_INFO
	CONFIG_TDP_PBF_GET_TJ_MAX_INFO    = C.CONFIG_TDP_PBF_GET_TJ_MAX_INFO
	CONFIG_TDP_PBF_GET_TDP_INFO       = C.CONFIG_TDP_PBF_GET_TDP_INFO

	// CLOS related commands
	CONFIG_CLOS        = C.CONFIG_CLOS
	CLOS_PM_QOS_CONFIG = C.CLOS_PM_QOS_CONFIG
	// The following are unusable
	//CLOS_PQR_ASSOC     = C.CLOS_PQR_ASSOC
	//CLOS_PM_CLOS       = C.CLOS_PM_CLOS
	//CLOS_STATUS        = C.CLOS_STATUS

	// PM commands
	READ_PM_CONFIG  = C.READ_PM_CONFIG
	WRITE_PM_CONFIG = C.WRITE_PM_CONFIG
	PM_FEATURE      = C.PM_FEATURE

	PM_QOS_INFO_OFFSET   = C.PM_QOS_INFO_OFFSET
	PM_QOS_CONFIG_OFFSET = C.PM_QOS_CONFIG_OFFSET
	PM_CLOS_OFFSET       = C.PM_CLOS_OFFSET
	PQR_ASSOC_OFFSET     = C.PQR_ASSOC_OFFSET
)
