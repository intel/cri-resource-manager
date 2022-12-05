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

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type HeatForecasterStdioConfig struct {
	Command []string
	Stderr  string
	Retry   int
}

type HeatForecasterStdio struct {
	config  *HeatForecasterStdioConfig
	process *exec.Cmd
	stdin   *bufio.Writer
	stdout  *bufio.Reader
	stderr  *os.File
	jsonout *json.Decoder
}

func init() {
	HeatForecasterRegister("stdio", NewHeatForecasterStdio)
}

func NewHeatForecasterStdio() (HeatForecaster, error) {
	return &HeatForecasterStdio{}, nil
}

func (hf *HeatForecasterStdio) SetConfigJson(configJson string) error {
	config := &HeatForecasterStdioConfig{}
	if err := unmarshal(configJson, config); err != nil {
		return err
	}
	return hf.SetConfig(config)
}

func (hf *HeatForecasterStdio) SetConfig(config *HeatForecasterStdioConfig) error {
	if err := hf.startProcess(config); err != nil {
		return err
	}
	hf.config = config
	return nil
}

func (hf *HeatForecasterStdio) GetConfigJson() string {
	configString, err := json.Marshal(hf.config)
	if err != nil {
		return ""
	}
	return string(configString)
}

func (hf *HeatForecasterStdio) Forecast(heats *Heats) (*Heats, error) {
	if heats == nil {
		return nil, nil
	}
	hf.sendCurrentHeats(heats, hf.config.Retry, []byte{})
	log.Debugf("forecast heats for %d processes sent", len(*heats))
	newHeats := &Heats{}
	if err := hf.jsonout.Decode(&newHeats); err != nil {
		log.Errorf("forecaster stdio: failed to read new heats: %s", err)
		return nil, err
	}
	log.Debugf("forecaster stdio: heats for %d processes received", len(*newHeats))
	return newHeats, nil
}

func (hf *HeatForecasterStdio) startProcess(config *HeatForecasterStdioConfig) error {
	if len(config.Command) == 0 {
		return fmt.Errorf("forecaster stdio: config 'Command' missing")
	}
	hf.process = exec.Command(config.Command[0], config.Command[1:]...)
	stdin, err := hf.process.StdinPipe()
	if err != nil {
		return fmt.Errorf("forecaster stdio: stdin failed: %w", err)
	}
	hf.stdin = bufio.NewWriter(stdin)
	stdout, err := hf.process.StdoutPipe()
	if err != nil {
		return fmt.Errorf("forecaster stdio: stdout failed: %w", err)
	}
	hf.stdout = bufio.NewReader(stdout)
	hf.stderr = nil
	if strings.HasPrefix(config.Stderr, "file:") {
		hf.stderr, err = os.OpenFile(config.Stderr[5:], os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			return fmt.Errorf("forecaster stdio: cannot open config.stderr file %q for writing: %w", config.Stderr[5:], err)
		}
	} else if config.Stderr != "" {
		return fmt.Errorf("forecaster stdio: unknown config.stderr directive %s, expecting empty or \"file:/path/to/stderr.log\"", config.Stderr)
	} else {
		// config.Stderr is undefined, copy it to stderr of memtierd.
		hf.process.Stderr = os.Stderr
	}
	err = hf.process.Start()
	if err != nil {
		return fmt.Errorf("forecaster stdio: failed to start config.Command %v: %w", config.Command, err)
	}
	hf.jsonout = json.NewDecoder(hf.stdout)
	// Call Wait() to make sure that hf.process.ProcessState will
	// be updated when process exits.
	go hf.process.Wait()
	return nil
}

func (hf *HeatForecasterStdio) sendCurrentHeats(heats *Heats, triesLeft int, marshaled []byte) error {
	var err error
	if triesLeft < 0 {
		return fmt.Errorf("forecaster stdio: sending heats failed, out of retries")
	}
	data := marshaled
	if len(data) == 0 {
		data, err = json.Marshal(heats)
		if err != nil {
			log.Errorf("forecast stdio: failed to marshal heats into json: %w", err)
			return err
		}
	}
	if n, err := hf.stdin.Write(data); err != nil {
		log.Errorf("forecast stdio: send heat error after %d bytes: %v", n, err)
		if hf.process.ProcessState.Exited() {
			log.Errorf("forecast stdio: process exited, restarting...")
			if err := hf.startProcess(hf.config); err != nil {
				return fmt.Errorf("forecast stdio: failed to restart process: %w", err)
			}
		} else {
			data = data[n:]
			log.Errorf("forecast stdio: %d bytes not yet sent", len(data))
		}
		return hf.sendCurrentHeats(heats, triesLeft-1, data)
	}
	hf.stdin.Write([]byte("\n"))
	hf.stdin.Flush()
	return nil
}

func (hf *HeatForecasterStdio) Dump(args []string) string {
	return "HeatForecasterStdio{config=" + hf.GetConfigJson() + "}"
}
