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
	"google.golang.org/grpc"

	"go.opencensus.io/plugin/ocgrpc"
	"go.opencensus.io/stats/view"
)

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

// registerGrpcViews registers default client and server trace views for gRPC.
func registerGrpcViews() error {
	log.Debugf("registering gRPC trace views...")

	if err := view.Register(ocgrpc.DefaultClientViews...); err != nil {
		return instrumentationError("failed to register default gRPC client views: %v", err)
	}
	if err := view.Register(ocgrpc.DefaultServerViews...); err != nil {
		return instrumentationError("failed to register default gRPC server views: %v", err)
	}

	return nil
}

// unregisterGrpcViews unregisters default client and server trace views for gRPC.
func unregisterGrpcViews() {
	view.Unregister(ocgrpc.DefaultClientViews...)
	view.Unregister(ocgrpc.DefaultServerViews...)
}
