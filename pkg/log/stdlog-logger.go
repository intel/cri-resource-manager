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
	stdlog "log"
)

// stdlogger implements an io.Writer to redirect logging by the stock log package.
type stdlogger struct {
	l Logger
}

// SetStdLogger sets up a logger for the standard log package.
func SetStdLogger(source string) {
	var l Logger

	if source == "" {
		l = Default()
	} else {
		l = log.get(source)
	}

	stdlog.SetPrefix("")
	stdlog.SetFlags(0)
	stdlog.SetOutput(&stdlogger{l: l})
}

// Write implements io.Writer for stdlogger.
func (s *stdlogger) Write(p []byte) (int, error) {
	s.l.Debug("%s", string(p))
	return len(p), nil
}
