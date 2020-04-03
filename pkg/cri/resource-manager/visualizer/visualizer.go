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
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	xhttp "github.com/intel/cri-resource-manager/pkg/instrumentation/http"
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
	builtin map[string]http.FileSystem
}

// Visualizer singleton instance.
var visualizers = &visualizer{
	builtin: map[string]http.FileSystem{},
}

// Register registers an builtin visualizer implementation.
func Register(name string, dir http.FileSystem) {
	visualizers.register(name, dir)
}

// Setup sets up the given multiplexer to serve visualization implementations.
func Setup(mux *xhttp.ServeMux) error {
	log.Info("activating visualization interface...")

	mux.Handle("/", http.RedirectHandler("/ui/index.html", http.StatusFound))
	mux.Handle("/ui", http.RedirectHandler("/ui/index.html", http.StatusFound))
	mux.Handle("/ui/builtin/", http.FileServer(visualizers))
	mux.Handle("/ui/external/", http.FileServer(visualizers))
	mux.HandleFunc("/ui/index.html", visualizers.generateIndexHTML)

	return nil
}

// Open is the http.Dir implementation for our visualizers.
func (v *visualizer) Open(path string) (http.File, error) {
	log.Debug("HTTP request %q", path)

	relative, err := filepath.Rel(visualizerPrefix+"/", path)
	if err != nil {
		return nil, visualizerError("failed to resolve path %q: %v", err)
	}

	log.Debug("%s => %s", path, relative)

	split := strings.Split(relative, "/")
	if len(split) < 2 {
		return nil, visualizerError("failed to resolve relative path %q", relative)
	}

	kind, name := split[0], split[1]
	fs, err := v.getVisualizerFileSystem(kind, name)
	if err != nil {
		return nil, err
	}

	return fs.Open(filepath.Join(split[2:]...))
}

// getVisualizerFileSystem returns the http.FileSystem for the given visualizer.
func (v *visualizer) getVisualizerFileSystem(kind, name string) (http.FileSystem, error) {
	switch kind {
	case "builtin":
		if dir, ok := v.builtin[name]; ok {
			return dir, nil
		}
		return nil, visualizerError("unknown builtin visualization UI %q", name)
	case "external":
		external := v.discoverExternalUIs()
		if path, ok := external[name]; ok {
			return http.FileSystem(http.Dir(path)), nil
		}
		return nil, visualizerError("unkown external visualization UI %q", name)
	}
	return nil, visualizerError("unknown visualization UI type %q", kind)
}

// Index page HTML header and footer.
const (
	uiPageHTMLHeader = `
<html>
  <head>
    <title>CRI Resource Manager - Workload Placement Visualization</title>
  </head>
  <body>
    <ul>
`
	uiPageHTMLFooter = `
    </ul>
  </body>
</html>
`
)

// generateIndexHTML generates a HTML page to access all known visualization UIs.
func (v *visualizer) generateIndexHTML(w http.ResponseWriter, req *http.Request) {
	builtinUIs := []string{}
	for name := range v.builtin {
		builtinUIs = append(builtinUIs, name)
	}
	sort.Strings(builtinUIs)

	externalUIs := []string{}
	for name := range v.discoverExternalUIs() {
		externalUIs = append(externalUIs, name)
	}
	sort.Strings(externalUIs)

	fmt.Fprintf(w, "%s", uiPageHTMLHeader)
	if len(builtinUIs)+len(externalUIs) == 0 {
		fmt.Fprintf(w, "No builtin or external visualization UIs found.")
	} else {
		for _, name := range builtinUIs {
			fmt.Fprintf(w, "<li><a href=\"/ui/builtin/%s\">%s</a>\n", name, name)
		}
		for _, name := range externalUIs {
			fmt.Fprintf(w, "<li><a href=\"/ui/external/%s\">external %s</a>\n", name, name)
		}
	}
	fmt.Fprintf(w, "%s\r\n", uiPageHTMLFooter)
}

// register registers a builtin visualizer implementation.
func (v *visualizer) register(name string, dir http.FileSystem) {
	if _, ok := v.builtin[name]; ok {
		log.Error("builtin visualizer '%s' already registered", name)
		return
	}
	v.builtin[name] = dir
	log.Info("registered %s builtin visualizer...", name)
}

// discoverExternalUIs returns a map of external visualizer implementations.
func (v *visualizer) discoverExternalUIs() map[string]string {
	external := make(map[string]string)
	for _, root := range strings.Split(externalDirs, ",") {
		filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || info.Name() != "index.html" {
				return nil
			}

			dir, err := filepath.Abs(filepath.Dir(path))
			if err != nil {
				log.Error("failed to determine absolute directory for '%s': %v", path, err)
				return nil
			}

			name := v.uniqueExternalUIName(dir, external)
			external[name] = dir

			log.Debug("found external visualizer '%s' (%s)", name, dir)

			return nil
		})
	}
	return external
}

// uniqueExternalUIName generates a unique name for the external visualizer.
func (v *visualizer) uniqueExternalUIName(dir string, others map[string]string) string {
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
		if _, ok := others[name]; !ok {
			return name
		}
		cnt++
	}
}

// visualizerError returns a formatted package-specific error.
func visualizerError(format string, args ...interface{}) error {
	return fmt.Errorf("visualizer: "+format, args...)
}
