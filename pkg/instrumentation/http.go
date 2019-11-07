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

package instrumentation

import (
	"context"
	"net/http"
)

// Notes: currently there is no proper locking here for the singletons.

// Our singleton HTTP server.
var srv *http.Server

// Our singleton HTTP request multiplexer.
var mux *http.ServeMux

// GetHTTPMux get our singleton HTTP request multiplexer for instrumentation.
func GetHTTPMux() *http.ServeMux {
	if mux == nil {
		mux = http.NewServeMux()
	}
	return mux
}

// GetHTTPServer returns our singleton HTTP server.
func GetHTTPServer() *http.Server {
	if srv == nil {
		srv = &http.Server{Handler: GetHTTPMux()}
	}
	return srv
}

// HTTPStart starts our HTTP server.
func HTTPStart() error {
	log.Debug("starting HTTP server...")

	srv := GetHTTPServer()
	srv.Addr = opt.Metrics
	go srv.ListenAndServe()
	return nil
}

// HTTPClose Close()'s our HTTP server.
func HTTPClose() error {
	srv = GetHTTPServer()
	err := srv.Close()
	srv = nil
	return err
}

// HTTPShutdown does a graceful Shutdown() of our HTTP server.
func HTTPShutdown() error {
	srv = GetHTTPServer()
	err := srv.Shutdown(context.Background())
	srv = nil
	return err
}
