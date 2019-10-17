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

package main

import (
	"flag"
	"os"
	"strings"
	"time"

	"github.com/intel/cri-resource-manager/pkg/config"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager"
	"github.com/intel/cri-resource-manager/pkg/instrumentation"

	logger "github.com/intel/cri-resource-manager/pkg/log"
	version "github.com/intel/cri-resource-manager/pkg/version"
)

func main() {
	log := logger.Default()

	if err := config.ParseCmdline(); err != nil {
		if err == flag.ErrHelp {
			os.Exit(0)
		}
		log.Error("failed to parse command line options: %v", err)
		os.Exit(1)
	}

	if len(config.Args()) != 0 {
		args := config.Args()
		if args[0] != "help" {
			log.Error("unknown command-line arguments: %s", strings.Join(config.Args(), ","))
			config.Usage()
			os.Exit(1)
		}
		config.Help(args[1:]...)
		os.Exit(1)
	}

	if opt.configFile != "" {
		if err := config.ParseYAMLFile(opt.configFile); err != nil {
			log.Error("failed to parse configuration file %s: %v", opt.configFile, err)
			os.Exit(1)
		}
	}

	log.Info("cri-resmgr (version %s, build %s) starting...", version.Version, version.Build)

	if err := instrumentation.Setup("CRI-RM"); err != nil {
		log.Fatal("failed to set up instrumentation: %v", err)
	}
	defer instrumentation.Finish()

	m, err := resmgr.NewResourceManager()
	if err != nil {
		log.Fatal("failed to create resource manager instance: %v", err)
	}

	if err := m.Start(); err != nil {
		log.Fatal("failed to start resource manager: %v", err)
	}

	for {
		time.Sleep(15 * time.Second)
	}
}
