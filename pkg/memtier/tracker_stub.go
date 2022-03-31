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

type TrackerStub struct {
}

func init() {
	TrackerRegister("stub", NewTrackerStub)
}

func NewTrackerStub() (Tracker, error) {
	return &TrackerStub{}, nil
}

func (t *TrackerStub) SetConfigJson(configJson string) error {
	return nil
}

func (t *TrackerStub) GetConfigJson() string {
	return ""
}

func (t *TrackerStub) AddPids(pids []int) {
}

func (t *TrackerStub) RemovePids(pids []int) {
}

func (t *TrackerStub) Start() error {
	return nil
}

func (t *TrackerStub) Stop() {
}

func (t *TrackerStub) ResetCounters() {
}

func (t *TrackerStub) GetCounters() *TrackerCounters {
	return nil
}

func (t *TrackerStub) Dump([]string) string {
	return ""
}
