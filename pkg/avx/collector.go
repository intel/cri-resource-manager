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

package avx

//go:generate go run elfdump.go

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	bpf "github.com/cilium/ebpf"
	"github.com/intel/cri-resource-manager/pkg/cgroups"
	logger "github.com/intel/cri-resource-manager/pkg/log"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sys/unix"
)

const (
	// LastCPUName is the Prometheuse Gauge name for last CPU with AVX512 instructions.
	LastCPUName = "last_cpu_avx_task_switches"
	// AVXSwitchCountName is the Prometheuse Gauge name for AVX switch count per cgroup.
	AVXSwitchCountName = "avx_switch_count_per_cgroup"
	// AllSwitchCountName is the Prometheuse Gauge name for all switch count per cgroup.
	AllSwitchCountName = "all_switch_count_per_cgroup"
	// LastUpdateNs is the Prometheuse Gauge name for per cgroup AVX512 activity timestamp.
	LastUpdateNs = "last_update_ns"
	// Path to kernel tracepoints
	kernelTracepointPath = "/sys/kernel/debug/tracing/events"
	// rlimit value (512k) needed to lock map data in memory
	mapMemLockLimit = 524288
)

// Prometheus Metric descriptor indices and descriptor table
const (
	lastCPUDesc = iota
	avxSwitchCountDesc
	allSwitchCountDesc
	lastUpdateNsDesc
	numDescriptors
)

var descriptors = [numDescriptors]*prometheus.Desc{
	lastCPUDesc: prometheus.NewDesc(
		LastCPUName,
		"Number of task switches on the CPU where AVX512 instructions were used.",
		[]string{
			"cpu_id",
		}, nil,
	),
	avxSwitchCountDesc: prometheus.NewDesc(
		AVXSwitchCountName,
		"Number of task switches where AVX512 instructions were used in a particular cgroup.",
		[]string{
			"cgroup",
			"cgroup_id",
		}, nil,
	),
	allSwitchCountDesc: prometheus.NewDesc(
		AllSwitchCountName,
		"Total number of task switches in a particular cgroup.",
		[]string{
			"cgroup",
		}, nil,
	),
	lastUpdateNsDesc: prometheus.NewDesc(
		"last_update_ns",
		"Time since last AVX512 activity in a particular cgroup.",
		[]string{
			"cgroup",
		}, nil,
	),
}

var (
	bpfBinaryName  = "avx512.o"
	bpfInstallpath = "/usr/libexec/bpf"

	// our logger instance
	log = logger.NewLogger("avx")
)

type collector struct {
	root string
	ebpf *bpf.Collection
	fds  []int
}

func enablePerfTracepoint(prog *bpf.Program, tracepoint string) (int, error) {

	id, err := ioutil.ReadFile(filepath.Join(kernelTracepointPath, tracepoint, "id"))
	if err != nil {
		return -1, errors.Wrap(err, "unable to read tracepoint ID")
	}

	tid, err := strconv.Atoi(strings.TrimSpace(string(id)))
	if err != nil {
		return -1, errors.New("unable to convert tracepoint ID")
	}

	attr := unix.PerfEventAttr{
		Type:        unix.PERF_TYPE_TRACEPOINT,
		Config:      uint64(tid), // tracepoint id
		Sample_type: unix.PERF_SAMPLE_RAW,
		Sample:      1,
		Wakeup:      1,
	}

	pfd, err := unix.PerfEventOpen(&attr, -1, 0, -1, unix.PERF_FLAG_FD_CLOEXEC)
	if err != nil {
		return -1, errors.Wrap(err, "unable to open perf events")
	}

	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(pfd), unix.PERF_EVENT_IOC_ENABLE, 0); errno != 0 {
		return -1, errors.Errorf("unable to set up perf events: %s", errno)
	}

	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(pfd), unix.PERF_EVENT_IOC_SET_BPF, uintptr(prog.FD())); errno != 0 {
		return -1, errors.Errorf("unable to attach bpf program to perf events: %s", errno)
	}

	return pfd, nil
}

func getKernelVersion() uint32 {

	var uts unix.Utsname

	err := unix.Uname(&uts)
	if err != nil {
		return 0
	}

	str := string(bytes.SplitN(uts.Release[:], []byte{0}, 2)[0])

	ver := strings.SplitN(str, ".", 3)

	major, err := strconv.ParseUint(ver[0], 10, 8)
	if err != nil {
		return 0
	}
	minor, err := strconv.ParseUint(ver[1], 10, 8)
	if err != nil {
		return uint32(major << 16)
	}

	// ignore patch version
	return uint32(major<<16 + minor<<8)
}

func kernelVersionStr(v uint32) string {
	return fmt.Sprintf("%d.%d.0", v>>16, (v>>8)&0xff)
}

// NewCollector creates new Prometheus collector for AVX metrics
func NewCollector() (prometheus.Collector, error) {

	// Set rlimit to be able to lock map values in memory
	memlockLimit := &unix.Rlimit{
		Cur: mapMemLockLimit,
		Max: mapMemLockLimit,
	}
	err := unix.Setrlimit(unix.RLIMIT_MEMLOCK, memlockLimit)
	if err != nil {
		return nil, errors.Wrap(err, "unable to set rlimit")
	}

	spec, err := bpf.LoadCollectionSpec(filepath.Join(bpfInstallpath, bpfBinaryName))
	if err != nil {
		log.Info("Unable to load user eBPF (%v). Using default CollectionSpec from ELF program bytes", err)
		spec, err = bpf.LoadCollectionSpecFromReader(bytes.NewReader(program[:]))
		if err != nil {
			return nil, errors.Wrap(err, "unable to load default CollectionSpec from ELF program bytes")
		}
	}

	hostVer := getKernelVersion()
	progVer := spec.Programs["tracepoint__x86_fpu_regs_deactivated"].KernelVersion

	if hostVer < progVer {
		return nil, errors.Wrapf(err, "The host kernel version (v%s) is too old to run the AVX512 collector program. Minimum version is v%s.", kernelVersionStr(hostVer), kernelVersionStr(progVer))
	}

	collection, err := bpf.NewCollection(spec)
	if err != nil {
		return nil, errors.Wrap(err, "unable to create new Collection")
	}

	ffd, err := enablePerfTracepoint(collection.Programs["tracepoint__x86_fpu_regs_deactivated"], "x86_fpu/x86_fpu_regs_deactivated")
	if err != nil {
		return nil, errors.Wrap(err, "unable to enable fpu tracepoint")
	}

	sfd, err := enablePerfTracepoint(collection.Programs["tracepoint__sched_switch"], "sched/sched_switch")
	if err != nil {
		return nil, errors.Wrap(err, "unable to enable sched tracepoint")
	}

	return &collector{
		root: cgroups.V2path,
		ebpf: collection,
		fds:  []int{ffd, sfd},
	}, nil
}

// Describe implements prometheus.Collector interface
func (c *collector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range descriptors {
		ch <- d
	}
}

// from iovisor/gobpf: bpf.NowNanoseconds()
// nowNanoseconds returns a time that can be compared to bpf_ktime_get_ns()
func nowNanoseconds() uint64 {
	var ts syscall.Timespec
	syscall.Syscall(syscall.SYS_CLOCK_GETTIME, 1 /* CLOCK_MONOTONIC */, uintptr(unsafe.Pointer(&ts)), 0)
	sec, nsec := ts.Unix()
	return 1000*1000*1000*uint64(sec) + uint64(nsec)
}

// Collect implements prometheus.Collector interface
func (c collector) Collect(ch chan<- prometheus.Metric) {
	var (
		wg  sync.WaitGroup
		key uint64
		val uint32
	)

	cgroupids := make(map[uint64]uint32)

	wg.Add(1)
	go func() {
		defer wg.Done()
		c.collectLastCPUStats(ch)
	}()

	cg := cgroups.NewCgroupID(c.root)

	m := c.ebpf.Maps["avx_context_switch_count_hash"]
	iter := m.Iterate()

	for iter.Next(&key, &val) {
		cgroupids[key] = val
		log.Debug("cgroupid %d => counter %d", key, val)

		// reset the counter by deleting the key
		err := m.Delete(key)
		if err != nil {
			log.Error("%+v", err)
		}
	}
	if iter.Err() != nil {
		log.Error("unable to iterate all elements of avx_context_switch_count: %+v", iter.Err())
	}

	for cgroupid, counter := range cgroupids {
		wg.Add(1)
		go func(cgroupid_ uint64, counter_ uint32) {
			var allCount uint32
			var lastUpdate uint64

			defer wg.Done()

			path, err := cg.Find(cgroupid_)
			if err != nil {
				log.Error("failed to find cgroup by id: %v", err)
				return
			}

			ch <- prometheus.MustNewConstMetric(
				descriptors[avxSwitchCountDesc],
				prometheus.GaugeValue,
				float64(counter_),
				path,
				fmt.Sprintf("%d", cgroupid_))

			if err := c.ebpf.Maps["all_context_switch_count_hash"].Lookup(uint64(cgroupid_), &allCount); err != nil {
				log.Error("unable to find 'all' context switch count: %+v", err)
				return
			}
			log.Debug("all: %d", allCount)

			if err := c.ebpf.Maps["last_update_ns_hash"].Lookup(uint64(cgroupid_), &lastUpdate); err != nil {
				log.Error("unable to find last update timestamp: %+v", err)
				return
			}
			log.Debug("last: %d", lastUpdate)

			ch <- prometheus.MustNewConstMetric(
				descriptors[allSwitchCountDesc],
				prometheus.GaugeValue,
				float64(allCount),
				path)

			ch <- prometheus.MustNewConstMetric(
				descriptors[lastUpdateNsDesc],
				prometheus.GaugeValue,
				float64(nowNanoseconds()-lastUpdate),
				path)

		}(cgroupid, counter)
	}

	// We need to wait so that the response channel doesn't get closed.
	wg.Wait()

	m = c.ebpf.Maps["all_context_switch_count_hash"]
	iter = m.Iterate()

	for iter.Next(&key, &val) {
		// reset the counter by deleting the key
		err := m.Delete(key)
		if err != nil {
			log.Error("%+v", err)
		}
	}

	if iter.Err() != nil {
		log.Error("unable to reset all elements of all_context_switch_count: %+v", iter.Err())
	}
}

func (c collector) collectLastCPUStats(ch chan<- prometheus.Metric) {

	lastCPUs := make(map[uint32]uint32)
	var cpu uint32
	var counter uint32

	m := c.ebpf.Maps["cpu_hash"]
	iter := m.Iterate()
	for iter.Next(&cpu, &counter) {
		lastCPUs[cpu] = counter

		log.Debug("CPU%d = %d", cpu, counter)

		// reset the counter by deleting key
		err := m.Delete(cpu)
		if err != nil {
			log.Error("%+v", err)
		}
	}

	if iter.Err() != nil {
		log.Error("unable to iterate all elements of cpu_hash: %+v", iter.Err())
		return
	}

	for lastCPU, count := range lastCPUs {
		ch <- prometheus.MustNewConstMetric(
			descriptors[lastCPUDesc],
			prometheus.GaugeValue,
			float64(count),
			fmt.Sprintf("CPU%d", lastCPU))
	}

func init() {
	flag.StringVar(&bpfInstallpath, "bpf-install-path", bpfInstallpath,
		"Path to eBPF install directory")
}
