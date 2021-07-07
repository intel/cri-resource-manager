/*
Copyright 2019 Intel Corporation

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"log"
)

// Parse command line
func parseArgs() args {
	args := args{}

	flag.IntVar(&args.port, "port", 443, "Port on which to listen for connections")
	flag.StringVar(&args.certFile, "cert-file", "", "x509 certificate used for authenticating connections")
	flag.StringVar(&args.keyFile, "key-file", "", "Private x509 key matching --cert-file")

	flag.Parse()

	return args
}

func main() {
	args := parseArgs()

	if err := Run(args); err != nil {
		log.Fatalf("%v", err)
	}

}
