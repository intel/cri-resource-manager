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

package dump

import (
	"github.com/intel/cri-resource-manager/pkg/config"
)

// Our configuration module.
var cfg *config.Module

// Create default dumper, register our configuration flags.
func init() {
	defaultDumper, _ = NewDumper(DefaultDumpConfig)

	cfg = config.Register("dump", "CRI message dumper")

	cfg.Var(defaultDumper, "messages",
		"value is a dump message specification of the format [target:]message[,...].\n"+
			"The possible targets are:\n"+DefaultHandlerFlagHelp("    "))
	cfg.StringVar(&dumpFileName, "file", "",
		"file to also save message dumps to")
	cfg.DurationVar(&cnt.Period, "period", DefaultPeriod,
		"period for the 'count' dump target")
}
