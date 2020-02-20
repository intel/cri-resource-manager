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

package instrumentation

import (
	"fmt"
	"net/http"

	"go.opencensus.io/plugin/ocgrpc"
	"google.golang.org/grpc"

	logger "github.com/intel/cri-resource-manager/pkg/log"
)

const (
	// ServiceName is our service name in external tracing and metrics services.
	ServiceName = "CRI-RM"
)

// Our logger instance.
var log = logger.NewLogger("instrumentation")

// Our instrumentation service instance.
var service = createService()

// Get drop-in mux for external services.
func GetHTTPMux() *http.ServeMux {
	if service == nil {
		return nil
	}
	return service.ServeMux
}

// TracingEnabled returns true if the Jaeger tracing sampler is not disabled.
func TracingEnabled() bool {
	if service == nil {
		return false
	}
	return service.TracingEnabled()
}

// Start our internal instrumentation services.
func Start() error {
	if service == nil {
		return instrumentationError("cannot start, no instrumentation service instance")
	}
	return service.Start()
}

// Stop stops our internal instrumentation services.
func Stop() {
	if service != nil {
		service.Stop()
	}
}

// Restart restarts our internal instrumentation services.
func Restart() error {
	if service == nil {
		return instrumentationError("cannot restart, no instrumentation service instance")
	}
	return service.Restart()
}

// InjectGrpcClientTrace injects gRPC dial options for instrumentation if necessary.
func InjectGrpcClientTrace(opts ...grpc.DialOption) []grpc.DialOption {
	extra := grpc.WithStatsHandler(&ocgrpc.ClientHandler{})

	if len(opts) > 0 {
		opts = append(opts, extra)
	} else {
		opts = []grpc.DialOption{extra}
	}

	return opts
}

// InjectGrpcServerTrace injects gRPC server options for instrumentation if necessary.
func InjectGrpcServerTrace(opts ...grpc.ServerOption) []grpc.ServerOption {
	extra := grpc.StatsHandler(&ocgrpc.ServerHandler{})

	if len(opts) > 0 {
		opts = append(opts, extra)
	} else {
		opts = []grpc.ServerOption{extra}
	}

	return opts
}

// instrumentationError produces a formatted instrumentation-specific error.
func instrumentationError(format string, args ...interface{}) error {
	return fmt.Errorf("instrumentation: "+format, args...)
}
