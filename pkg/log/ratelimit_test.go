// Copyright 2019-2020 Intel Corporation. All Rights Reserved.
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

package log

import (
	"fmt"
	"testing"
	"time"

	goxrate "golang.org/x/time/rate"
)

func TestRateLimit(t *testing.T) {
	ratelimit := RateLimit(Default(), Rate{Window: MinimumWindow, Limit: Every(time.Second)})
	rl := ratelimit.(*ratelimited)

	limiters := make(map[string]*goxrate.Limiter)

	// fill message window, store limiters for checking
	messages := make([]string, 0, MinimumWindow)
	for idx := 0; idx < cap(messages); idx++ {
		msg := fmt.Sprintf("message #%d", idx)
		messages = append(messages, msg)
		limiters[msg] = rl.getMessageLimit(msg)
	}

	// check looked up vs. stored limters
	for msg, limiter := range limiters {
		if rl.getMessageLimit(msg) != limiter {
			t.Errorf("unexpected new limiter for message %s", msg)
		}
	}

	// create more messages, store limiters for checking
	recent := make([]string, 0, MinimumWindow/5)
	for i := 0; i < cap(recent); i++ {
		msg := fmt.Sprintf("message #%d", len(messages)+i)
		recent = append(recent, msg)
		limiters[msg] = rl.getMessageLimit(msg)
	}

	// check looked up vs. stored limiters
	for _, msg := range recent {
		if rl.getMessageLimit(msg) != limiters[msg] {
			t.Errorf("unexpected new limiter for recent message %s", msg)
		}
	}

	// check in-window part of old messages
	for idx := len(recent); idx < len(messages); idx++ {
		msg := messages[idx]
		l := rl.getMessageLimit(msg)
		if l != limiters[msg] {
			t.Errorf("unexpected new limiter for old message %s", msg)
		}
	}

	// check shifted out part of old messages
	for idx := 0; idx < len(recent); idx++ {
		msg := messages[idx]
		l := rl.getMessageLimit(msg)
		if l == limiters[msg] {
			t.Errorf("unexpected old limiter for old message %s", msg)
		}
	}
}
