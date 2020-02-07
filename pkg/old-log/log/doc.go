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

package log

var configHelp = `
Logging and debugging messages.

To control logging and debug messages include a corresponding configuration
fragment in your runtime configuration. You can control the lowest severity
of messages to pass through, which log sources are enabled, and which log
sources are producing debug messages.

The available message severity levels are error, warning, and info. By default
all log sources produce messages of all severity and none of the log sources
produce any debug messages. For instance to enable only warnings and errors,
and debugging for the resource-manager, policy, cache, and message sources you
can use the config fragment below:

  logger:
    enable: warning,error
    debug: resource-manager,policy,cache,messages

The reserved keywords 'all' and 'none' can be used to refer to all or none of
the log sources. For instance, the following fragment enables full logging and
debugging:

  logger:
    enable: all
    debug: all

The very same logger settings can be also controlled using the --logger-source
and --logger-debug command line options.
`
