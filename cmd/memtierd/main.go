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

package main

import (
	"bufio"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/intel/cri-resource-manager/pkg/memtier"
)

type Config struct {
	Policy   memtier.PolicyConfig
	Routines []memtier.RoutineConfig
}

func exit(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, fmt.Sprintf("memtierd: "+format+"\n", a...))
	os.Exit(1)
}

func loadConfigFile(filename string) (memtier.Policy, []memtier.Routine) {
	configBytes, err := ioutil.ReadFile(filename)
	if err != nil {
		exit("%s", err)
	}
	var config Config
	err = yaml.Unmarshal(configBytes, &config)
	if err != nil {
		exit("error in %q: %s", filename, err)
	}

	policy, err := memtier.NewPolicy(config.Policy.Name)
	if err != nil {
		exit("%s", err)
	}

	err = policy.SetConfigJson(config.Policy.Config)
	if err != nil {
		exit("%s", err)
	}

	routines := []memtier.Routine{}
	for _, routineCfg := range config.Routines {
		routine, err := memtier.NewRoutine(routineCfg.Name)
		if err != nil {
			exit("%s", err)
		}
		err = routine.SetConfigJson(routineCfg.Config)
		if err != nil {
			exit("routine %s: %s", routineCfg.Name, err)
		}
		routines = append(routines, routine)
	}
	return policy, routines
}

func main() {
	memtier.SetLogger(log.New(os.Stderr, "", 0))
	optPrompt := flag.Bool("prompt", false, "launch interactive prompt (ignore other parameters)")
	optConfig := flag.String("config", "", "launch non-interactive mode with config file")
	optConfigDumpJson := flag.Bool("config-dump-json", false, "dump effective configuration in JSON")
	optDebug := flag.Bool("debug", false, "print debug output")

	flag.Parse()
	memtier.SetLogDebug(*optDebug)

	if *optPrompt {
		prompt := NewPrompt("memtierd> ", bufio.NewReader(os.Stdin), bufio.NewWriter(os.Stdout))
		prompt.Interact()
		return
	}

	var policy memtier.Policy
	var routines []memtier.Routine
	if *optConfig != "" {
		policy, routines = loadConfigFile(*optConfig)
	} else {
		exit("missing -prompt or -config")
	}

	if *optConfigDumpJson {
		fmt.Printf("%s\n", policy.GetConfigJson())
		os.Exit(0)
	}

	if policy != nil {
		if err := policy.Start(); err != nil {
			exit("error in starting policy: %s", err)
		}
	}

	for r, routine := range routines {
		if err := routine.Start(); err != nil {
			exit("error in starting routine %d: %s", r+1, err)
		}
	}

	prompt := NewPrompt("memtierd> ", bufio.NewReader(os.Stdin), bufio.NewWriter(os.Stdout))
	if stdinFileInfo, _ := os.Stdin.Stat(); (stdinFileInfo.Mode() & os.ModeCharDevice) == 0 {
		// Input comes from a pipe.
		// Echo commands after prompt in the interaction to explain outputs.
		prompt.SetEcho(true)
	}
	prompt.SetPolicy(policy)
	if len(routines) > 0 {
		prompt.SetRoutines(routines)
	}
	prompt.Interact()
}
