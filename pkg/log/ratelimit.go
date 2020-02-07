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
	"sync"
	"time"

	goxrate "golang.org/x/time/rate"
)

// Rate specifies maximum per-message logging rate.
type Rate struct {
	// rate limit
	Limit goxrate.Limit
	// allowed bursts
	Burst int
	// optional message window size
	Window int
}

// ratelimited implements rate-limited logging.
type ratelimited struct {
	Logger
	sync.Mutex
	rate   Rate
	window []string
	limits map[string]*goxrate.Limiter
}

const (
	// DefaultWindow is the default message window size for rate limiting.
	DefaultWindow = 256
	// MinimumWindow is the smalled message window size for rate limiting.
	MinimumWindow = 32
)

// Every defines a rate limit for the given interval.
func Every(interval time.Duration) goxrate.Limit {
	return goxrate.Every(interval)
}

// Interval returns a Rate for the given interval.
func Interval(interval time.Duration) Rate {
	return Rate{Limit: Every(interval), Burst: 1}
}

// RateLimit returns a ratelimited version of the given logger.
func RateLimit(log Logger, rate Rate) Logger {
	switch {
	case rate.Window == 0:
		rate.Window = DefaultWindow
	case rate.Window < MinimumWindow:
		rate.Window = MinimumWindow
	}
	if rate.Burst < 1 {
		rate.Burst = 1
	}
	return &ratelimited{
		Logger: log,
		rate:   rate,
		limits: make(map[string]*goxrate.Limiter),
		window: make([]string, 0, rate.Window),
	}
}

func (rl *ratelimited) Debug(format string, args ...interface{}) {
	if msg := rl.filter(format, args...); msg != "" {
		rl.Logger.Debug("<rate-limited> %s", msg)
	}
}

func (rl *ratelimited) Info(format string, args ...interface{}) {
	if msg := rl.filter(format, args...); msg != "" {
		rl.Logger.Info("<rate-limited> %s", msg)
	}
}

func (rl *ratelimited) Warn(format string, args ...interface{}) {
	if msg := rl.filter(format, args...); msg != "" {
		rl.Logger.Warn("<rate-limited> %s", msg)
	}
}

func (rl *ratelimited) Error(format string, args ...interface{}) {
	if msg := rl.filter(format, args...); msg != "" {
		rl.Logger.Error("<rate-limited> %s", msg)
	}
}

func (rl *ratelimited) filter(format string, args ...interface{}) string {
	rl.Lock()
	defer rl.Unlock()

	msg := fmt.Sprintf(format, args...)
	lim, ok := rl.limits[msg]

	if !ok {
		if len(rl.window) > 0 {
			rl.window = rl.window[1:]
		}
		rl.window = append(rl.window, msg)
		lim = goxrate.NewLimiter(rl.rate.Limit, rl.rate.Burst)
		rl.limits[msg] = lim
	}

	if !lim.Allow() {
		msg = ""
	}
	return msg
}
