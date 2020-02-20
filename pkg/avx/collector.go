package avx

import (
	"debug/elf"
	"encoding/binary"
	"flag"
	"fmt"
	"path"
	"sync"
	"syscall"
	"unsafe"

	"github.com/intel/cri-resource-manager/pkg/cgroups"
	logger "github.com/intel/cri-resource-manager/pkg/log"
	bpf "github.com/iovisor/gobpf/elf"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
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

func kernelVersionCode(major, minor, patch uint8) uint32 {
	return uint32(major)<<16 + uint32(minor)<<8 + uint32(patch)
}

// getElfKernelVersion returns major, minor and patch parts of kernel version
// compiled into eBPF ELF file.
func getElfKernelVersion(path string) (uint8, uint8, uint8, error) {
	elfFile, err := elf.Open(path)
	if err != nil {
		return 0, 0, 0, errors.Wrapf(err, "unable to open ELF file %s", path)
	}
	defer elfFile.Close()

	sec := elfFile.Section("version")
	if sec == nil {
		return 0, 0, 0, errors.New("unable to find 'version' section")
	}
	data, err := sec.Data()
	if err != nil {
		return 0, 0, 0, errors.Wrap(err, "unable to get version data")
	}

	// Least Significant Byte first
	return data[2], data[1], data[0], nil
}

func checkElfKernelVersion(path string) error {
	elfMajor, elfMinor, _, err := getElfKernelVersion(path)
	if err != nil {
		return err
	}

	currentCode, err := bpf.CurrentKernelVersion()
	if err != nil {
		return errors.Wrap(err, "unable to get current kernel version")
	}

	if currentCode < kernelVersionCode(elfMajor, elfMinor, 0) {
		return errors.New("host kernel is too old, consider rebuilding eBPF")
	}

	return nil
}

type collector struct {
	root                     string
	elfFilepath              string
	bpfModule                *bpf.Module
	avxContextSwitchCounters *bpf.Map
	allContextSwitchCounters *bpf.Map
	lastUpdateNs             *bpf.Map
	lastCPUCounters          *bpf.Map
}

// NewCollector creates new Prometheus collector for AVX metrics
func NewCollector() (prometheus.Collector, error) {

	elfFilepath := path.Join(bpfInstallpath, bpfBinaryName)

	if err := checkElfKernelVersion(elfFilepath); err != nil {
		return nil, err
	}

	bpfModule := bpf.NewModule(elfFilepath)

	sectionParams := make(map[string]bpf.SectionParams)
	if err := bpfModule.Load(sectionParams); err != nil {
		return nil, errors.Wrap(err, "unable to load eBPF ELF file")
	}

	allSwitchCounters := bpfModule.Map("all_context_switch_count")
	if allSwitchCounters == nil {
		return nil, errors.New("map all_context_switch_count not found")
	}

	avxSwitchCounters := bpfModule.Map("avx_context_switch_count")
	if avxSwitchCounters == nil {
		return nil, errors.New("map avx_context_switch_count not found")
	}

	lastUpdateNs := bpfModule.Map("last_update_ns")
	if lastUpdateNs == nil {
		return nil, errors.New("map last_update_ns not found")
	}

	lastCPUCounters := bpfModule.Map("cpu")
	if lastCPUCounters == nil {
		return nil, errors.New("map cpu not found")
	}

	if err := bpfModule.EnableTracepoint("tracepoint/sched/sched_switch"); err != nil {
		return nil, errors.Wrap(err, "couldn't enable tracepoint/sched/sched_switch")
	}

	if err := bpfModule.EnableTracepoint("tracepoint/x86_fpu/x86_fpu_regs_deactivated"); err != nil {
		return nil, errors.Wrap(err, "couldn't enable tracepoint/x86_fpu/x86_fpu_regs_deactivated")
	}

	return &collector{
		root:                     cgroups.V2path,
		bpfModule:                bpfModule,
		avxContextSwitchCounters: avxSwitchCounters,
		allContextSwitchCounters: allSwitchCounters,
		lastUpdateNs:             lastUpdateNs,
		lastCPUCounters:          lastCPUCounters,
	}, nil
}

// Describe implements prometheus.Collector interface
func (c *collector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range descriptors {
		ch <- d
	}
}

// TODO use bpf.NowNanoseconds() after https://github.com/iovisor/gobpf/pull/222
// nowNanoseconds returns a time that can be compared to bpf_ktime_get_ns()
func nowNanoseconds() uint64 {
	var ts syscall.Timespec
	syscall.Syscall(syscall.SYS_CLOCK_GETTIME, 1 /* CLOCK_MONOTONIC */, uintptr(unsafe.Pointer(&ts)), 0)
	sec, nsec := ts.Unix()
	return 1000*1000*1000*uint64(sec) + uint64(nsec)
}

// Collect implements prometheus.Collector interface
func (c collector) Collect(ch chan<- prometheus.Metric) {
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		c.collectLastCPUStats(ch)
	}()

	cg := cgroups.NewCgroupID(c.root)
	cgroupids, counters, err := c.moveAllElements(c.avxContextSwitchCounters, unsafe.Sizeof(uint64(0)), unsafe.Sizeof(uint32(0)))
	if err != nil {
		log.Error("unable to move elements of avx_context_switch_count: %+v", err)
	}

	for idx, cgroupid := range cgroupids {
		wg.Add(1)
		go func(idx_ int, cgroupid_ []byte) {
			var allCount uint32
			var lastUpdate uint64

			defer wg.Done()

			path, err := cg.Find(binary.LittleEndian.Uint64(cgroupid_[0:]))
			if err != nil {
				log.Error("failed to find cgroup by id: %v", err)
				return
			}

			ch <- prometheus.MustNewConstMetric(
				descriptors[avxSwitchCountDesc],
				prometheus.GaugeValue,
				float64(binary.LittleEndian.Uint32(counters[idx_][0:])),
				path,
				fmt.Sprintf("%d", binary.LittleEndian.Uint64(cgroupid_[0:])))

			if err := c.bpfModule.LookupElement(c.allContextSwitchCounters, unsafe.Pointer(&cgroupid_[0]), unsafe.Pointer(&allCount)); err != nil {
				log.Error("unable to find 'all' switch count: %+v", err)
				return
			}

			if err := c.bpfModule.LookupElement(c.lastUpdateNs, unsafe.Pointer(&cgroupid_[0]), unsafe.Pointer(&lastUpdate)); err != nil {
				log.Error("unable to find last update timestamp: %+v", err)
				return
			}

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

		}(idx, cgroupid)
	}

	// We need to wait so that the response channel doesn't get closed.
	wg.Wait()

	_, _, err = c.moveAllElements(c.allContextSwitchCounters, unsafe.Sizeof(uint64(0)), unsafe.Sizeof(uint32(0)))
	if err != nil {
		log.Error("unable to delete elements of all_context_switch_count: %+v", err)
	}
}

func (c *collector) moveAllElements(table *bpf.Map, keySize, valueSize uintptr) ([][]byte, [][]byte, error) {
	var keys [][]byte
	var values [][]byte

	zero := make([]byte, keySize)
	nextKey := make([]byte, keySize)
	value := make([]byte, valueSize)

	for {
		ok, err := c.bpfModule.LookupNextElement(table, unsafe.Pointer(&zero[0]), unsafe.Pointer(&nextKey[0]), unsafe.Pointer(&value[0]))
		if err != nil {
			return nil, nil, errors.Wrap(err, "unable to look up")
		}

		if !ok {
			break
		}

		keyClone := append(nextKey[:0:0], nextKey...)
		keys = append(keys, keyClone)

		valueClone := append(value[:0:0], value...)
		values = append(values, valueClone)

		if err := c.bpfModule.DeleteElement(table, unsafe.Pointer(&nextKey[0])); err != nil {
			return nil, nil, errors.Wrap(err, "unable to delete")
		}
	}

	return keys, values, nil
}

func (c collector) collectLastCPUStats(ch chan<- prometheus.Metric) {
	// NB: (* struct fpu)->last_cpu is of type `unsigned int` which translates to Go's uint32, not uint (4 bytes size in IA)
	lastCPUs, counters, err := c.moveAllElements(c.lastCPUCounters, unsafe.Sizeof(uint32(0)), unsafe.Sizeof(uint32(0)))
	if err != nil {
		log.Error("unable to move elements of cpu map: %+v", err)
		return
	}

	for idx, lastCPU := range lastCPUs {
		ch <- prometheus.MustNewConstMetric(
			descriptors[lastCPUDesc],
			prometheus.GaugeValue,
			float64(binary.LittleEndian.Uint32(counters[idx])),
			fmt.Sprintf("CPU%d", binary.LittleEndian.Uint32(lastCPU)))
	}
}

func init() {
	flag.StringVar(&bpfInstallpath, "bpf-install-path", bpfInstallpath,
		"Path to eBPF install directory")
}
