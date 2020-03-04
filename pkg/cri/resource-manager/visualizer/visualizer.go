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

package visualizer

import (
	"fmt"
	//"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	logger "github.com/intel/cri-resource-manager/pkg/log"
)

const (
	// HTTP URI prefix to register all visualizer implementations under.
	visualizerPrefix = "/ui"
)

// Our logger instance.
var log = logger.NewLogger("visualizer")

// visualizer captures our runtime state.
type visualizer struct {
	sync.Mutex
	internal map[string]http.FileSystem
	external map[string]http.FileSystem
}

var visualizers = &visualizer{
	internal: map[string]http.FileSystem{},
	external: map[string]http.FileSystem{},
}

// RegisterInternal registers an internal visualizer implementation.
func RegisterInternal(name string, dir http.FileSystem) {
	visualizers.register(name, dir, true)
}

// RegisterExternal registers an external visualizer implementation.
func RegisterExternal(name string, dir http.FileSystem) {
	visualizers.register(name, dir, false)
}

// Setup sets up the given multiplexer to serve visualization implementations.
func Setup(mux *http.ServeMux) error {
	log.Info("activating visualization interface...")

	mux.Handle("/", http.RedirectHandler("/ui/index.html", http.StatusFound))
	mux.Handle("/ui", http.RedirectHandler("/ui/index.html", http.StatusFound))
	mux.Handle("/ui/internal/", http.FileServer(visualizers))
	mux.Handle("/ui/external/", http.FileServer(visualizers))
	mux.HandleFunc("/ui/index.html", visualizers.genIndexHTML)

	return nil
}

// Open is the http.Dir implementation for our visualizers.
func (v *visualizer) Open(path string) (http.File, error) {
	var dir http.FileSystem
	var ok bool

	log.Debug("HTTP request for '%s'", path)

	rpath, err := filepath.Rel(visualizerPrefix+"/", path)
	if err != nil {
		log.Error("failed to resolve path for %s: %v", path, err)
		return nil, visualizerError("failed to resolve path for %s: %v", path, err)
	}

	log.Debug("%s => %s", path, rpath)

	split := strings.Split(rpath, "/")
	if len(split) < 1 {
		return nil, visualizerError("failed to resolve relative path %s", rpath)
	}

	kind := split[0]

	switch {
	case len(split) > 1 && kind == "internal":
		dir, ok = v.internal[split[1]]
	case len(split) > 1 && kind == "external":
		dir, ok = v.external[split[1]]
	default:
		ok = false
	}

	if !ok {
		return nil, visualizerError("failed to resolve relative path %s", rpath)
	}

	return dir.Open(filepath.Join(split[2:]...))
}

// register registers a visualizer implementation.
func (v *visualizer) register(name string, dir http.FileSystem, isInternal bool) {
	var m map[string]http.FileSystem
	var kind string

	if isInternal {
		m = v.internal
		kind = "internal"
	} else {
		m = v.external
		kind = "external"
	}

	log.Info("registered %s visualizer %s...", kind, name)

	if _, ok := m[name]; ok {
		log.Error("%s visualizer '%s' already registered", kind, name)
		return
	}

	m[name] = dir
}

// discoverExternal discover external visualizer implementations.
func (v *visualizer) discoverExternal(dir string) {
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || info.Name() != "index.html" {
			return nil
		}

		dir := filepath.Dir(path)
		dir, err = filepath.Abs(dir)
		if err != nil {
			log.Error("failed to determine absolute path for '%s': %v", dir, err)
			return nil
		}
		name := v.uniqueExternalName(dir)

		log.Debug("found external visualizer '%s' (%s)", name, dir)
		v.register(name, http.FileSystem(http.Dir(dir)), false)

		return nil
	})
}

// uniqueExternalName generate a unique name for the external visualizer.
func (v *visualizer) uniqueExternalName(dir string) string {
	base := filepath.Base(dir)
	if base == "assets" {
		base = filepath.Base(filepath.Dir(dir))
	}
	cnt := 0
	name := base
	for {
		if cnt > 0 {
			name = base + fmt.Sprintf("-%d", cnt)
		}
		if _, ok := v.external[name]; !ok {
			return name
		}
		cnt++
	}
}

// genIndexHTML returns an index page to access all known visualizers.
func (v *visualizer) genIndexHTML(w http.ResponseWriter, req *http.Request) {
	fmt.Fprintf(w,
		"<html>\n"+
			"<head>\n"+
			"<title>CRI Resource Manager - Workload Placement Visualization</title>\n"+
			"</head>\n"+
			"<body>\n"+
			"<ul>\n")

	names := []string{}
	for name := range v.internal {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(w, `<li><a href="/ui/internal/%s">%s</a>`, name, name)
	}

	names = []string{}
	for name := range v.external {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(w, `<li><a href="/ui/external/%s">%s (external)</a>`, name, name)
	}

	fmt.Fprintf(w,
		"</ul>\n"+
			"</body>\n"+
			"</html>")
	fmt.Fprintf(w, "\n\r")
}

// visualizerError returns a formatted package-specific error.
func visualizerError(format string, args ...interface{}) error {
	return fmt.Errorf("visualizer: "+format, args...)
}
