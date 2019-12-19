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

package main

import (
	"flag"
	"fmt"
	"github.com/minio/sha256-simd"
	"os"
	"os/exec"
	"runtime"
	"time"
)

const (
	bufSize    = 4 * 1024
	iterations = 32 * 1024
)

type config struct {
	count int
	fork  bool
	id    string
}

var cfg = &config{}

type avx512 struct {
	id   string
	fork bool
	buf  [bufSize]byte
}

func newAvx512(id string, fork bool) *avx512 {
	return &avx512{id: id, fork: fork}
}

func (a *avx512) log(format string, args ...interface{}) {
	fmt.Printf("["+a.id+"] "+format+"\n", args...)
}

func (a *avx512) start() error {
	if a.fork {
		cmd := exec.Command(os.Args[0], "--id", a.id)
		err := cmd.Start()
		return err
	}

	go func() { runtime.LockOSThread(); a.load() }()
	return nil
}

func (a *avx512) load() {
	a.log("preparing test input...")
	for i := 0; i < len(a.buf); i++ {
		a.buf[i] = byte(i & 0xff)
		if (i % (len(a.buf) / 100)) == 0 {
			a.log("%.2f %%", 100*float64(i)/float64(len(a.buf)))
		}
	}
	a.log("done")

	server := sha256.NewAvx512Server()
	h512 := sha256.NewAvx512(server)
	rounds := 0
	for {
		for i := 0; i < iterations; i++ {
			h512.Write(a.buf[:])
		}
		h512.Sum([]byte{})
		h512.Reset()
		rounds++
		a.log("done with %d test rounds", rounds)
	}
}

func main() {
	flag.IntVar(&cfg.count, "count", 1, "how many AVX512 load generator to run in parallel")
	flag.BoolVar(&cfg.fork, "fork", false, "fork subprocesses instead of using goroutines")
	flag.StringVar(&cfg.id, "id", "", "AVX512 load generator id")
	flag.Parse()

	for i := 0; i < cfg.count; i++ {
		id := fmt.Sprintf("AVX512#%d", i)
		avx := newAvx512(id, cfg.fork)
		err := avx.start()
		if err != nil {
			fmt.Printf("error: failed to start %s: %v\n", id, err)
			os.Exit(1)
		}
	}

	if !cfg.fork {
		n := runtime.NumGoroutine()
		runtime.GOMAXPROCS(n)
		fmt.Printf("GOMAXPROCS set to %d\n", n)
	}

	for {
		time.Sleep(3600 * time.Second)
	}
}
