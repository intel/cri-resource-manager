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

package http

import (
	"io"
	"net/http"
	"testing"
)

func TestStartStop(t *testing.T) {
	srv := NewServer()

	if err := srv.Start(":0"); err != nil {
		t.Errorf("failed to start HTTP server: %v", err)
	}

	srv.Stop()

	if err := srv.Start(":0"); err != nil {
		t.Errorf("failed to start HTTP server: %v", err)
	}

	if err := srv.Restart(":0"); err != nil {
		t.Errorf("failed to restart HTTP server on different port: %v", err)
	}

	if err := srv.Reconfigure(srv.GetAddress()); err != nil {
		t.Errorf("failed to reconfigure HTTP server on same port: %v", err)
	}
	if err := srv.Reconfigure(":0"); err != nil {
		t.Errorf("failed to reconfigure HTTP server on different port: %v", err)
	}

	srv.Stop()
}

type urlTest struct {
	pattern  string
	response string
	fallback string
}

func checkURL(t *testing.T, srv *Server, path, response string, status int) {
	url := "http://" + srv.GetAddress() + path

	res, err := http.Get(url)
	if err != nil {
		t.Errorf("http.Get(%s) failed: %v", url, err)
	}

	if res.StatusCode != status {
		t.Errorf("http.Get(%s) status %d, expected %d", url, res.StatusCode, status)
	}

	txt, err := io.ReadAll(res.Body)
	if err != nil {
		t.Errorf("http.Get(%s) failed to read response: %v", url, err)
	}

	if string(txt) != response {
		t.Errorf("http.Get(%s) unexpected response: %v, expected: %v", url, txt, response)
	}
}

type testHandler struct {
	response string
}

func (h *testHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	_, _ = w.Write([]byte(h.response))
}

func TestPatternsp(t *testing.T) {
	srv := NewServer()
	mux := srv.GetMux()

	if err := srv.Start(":0"); err != nil {
		t.Errorf("failed to start HTTP server: %v", err)
	}

	rh := &testHandler{"/"}
	ah := &testHandler{"a"}
	bh := &testHandler{"b"}
	ch := &testHandler{"c"}

	mux.Handle("/a", ah)
	checkURL(t, srv, "/a", "a", 200)

	mux.Handle("/b", bh)
	checkURL(t, srv, "/b", "b", 200)

	mux.Handle("/", rh)
	checkURL(t, srv, "/b", "b", 200)

	mux.Unregister("/b")
	checkURL(t, srv, "/b", "/", 200)

	mux.Handle("/b", ch)
	checkURL(t, srv, "/b", "c", 200)

	mux.Unregister("/a")
	checkURL(t, srv, "/a", "/", 200)

	srv.Stop()
}
