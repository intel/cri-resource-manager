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

package dump

//
// This package implements the dumping of (gRPC) methods calls where
// each method is called with a single request struct and returns a
// single reply struct or an error. Configuring what to dump happens
// by specifying a comma-separated dump request on the command line.
//
// A dump request is a comma-separated list of dump specs:
//     <spec>[,<spec>,...,<spec>], where each spec is of the form
//     <[target:]request>
// A request is either a requests name (gRPC method name without
// the leading path), or a regexp for matching requests.
// The dump targets are: 'off', 'name', 'full', 'count' by default.
//

const configHelp = `
CRI protocol gRPC request/response dumping..

This configuration controls message dumping details of CRI gRPC
method calls. Both requests and the resulting replies or errors
can be dumped. Messages can be both logged and dumped to a given
file.

Configuring what to dumps happens using a dump configuration string
of the following format:

  level1:pattern1[,level2:pattern2,...][,debug]

Each level specifies a level of detail for method calls with names
matching the corresponding pattern. A pattern can be a method call
name to match just a single method, or it can be regexp to match
several methods. For regexps all the patterns are evaluated in order
of appearance with the last one staying in effect. Exact method name
patterns terminate the evaluation without any regexp processing.

The possible levels of duping detail are:

  off: suppress dumping of matching requests and replies
  short: short dump of requests and potential error replies
  full: full dump of both request and reply content as YAML

Additionally including 'debug' in the configuration string will
cause messages to be logged as debug messages with the 'message'
log source. Note that debugging for this source needs to be
explicitly enabled, otherwise messages are suppressed.

If a dump file is specified messages will be dumped additionally
to the dump file as well.

Here is a sample configuration fragment to suppress all .*List.*
calls, produce short dumps of all .*Stop.* calls, and full dumps
of everything else, dumps also going to the file '/tmp/cri-dump.log'

  dump:
    config: full:.*,short:.*Stop.*,off:.*List.*
    file: /tmp/cri-dump.log
`
