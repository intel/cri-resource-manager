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

	"google.golang.org/grpc/grpclog"
)

// SetGrpcLogger sets up a logger for (google.golang.org/)grpc.
func SetGrpcLogger(source string, rate *Rate) {
	var l Logger

	if source == "" {
		l = Default()
	} else {
		l = log.get(source)
	}

	if rate != nil {
		l = RateLimit(l, *rate)
	}

	grpclog.SetLoggerV2(&grpclogger{Logger: l})
}

// grpclogger implements grpclog.LoggerV2 interface for our logger.
type grpclogger struct {
	Logger
}

func (g grpclogger) Info(args ...interface{}) {
	g.Logger.Debug("%s", fmt.Sprint(args...))
}

func (g grpclogger) Infoln(args ...interface{}) {
	g.Logger.Debug("%s", fmt.Sprint(args...))
}

func (g grpclogger) Infof(format string, args ...interface{}) {
	g.Logger.Debug(format, args...)
}

func (g grpclogger) Warning(args ...interface{}) {
	g.Logger.Warn("%s", fmt.Sprint(args...))
}

func (g grpclogger) Warningln(args ...interface{}) {
	g.Logger.Warn("%s", fmt.Sprint(args...))
}

func (g grpclogger) Warningf(format string, args ...interface{}) {
	g.Logger.Warn(format, args...)
}

func (g grpclogger) Error(args ...interface{}) {
	g.Logger.Error("%s", fmt.Sprint(args...))
}

func (g grpclogger) Errorln(args ...interface{}) {
	g.Logger.Error("%s", fmt.Sprint(args...))
}

func (g grpclogger) Errorf(format string, args ...interface{}) {
	g.Logger.Error(format, args...)
}

func (g grpclogger) Fatal(args ...interface{}) {
	g.Logger.Fatal("%s", fmt.Sprint(args...))
}

func (g grpclogger) Fatalln(args ...interface{}) {
	g.Logger.Fatal("%s", fmt.Sprint(args...))
}

func (g grpclogger) Fatalf(format string, args ...interface{}) {
	g.Logger.Fatal(format, args...)
}

func (g grpclogger) V(_ int) bool {
	return true
}
