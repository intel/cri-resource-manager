package dyp

import (
	"bufio"
	"context"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/intel/cri-resource-manager/pkg/sysfs"
)

type cpuTimesStat struct {
	cpu       string  `json:"cpu"`
	user      float64 `json:"user"`
	system    float64 `json:"system"`
	idle      float64 `json:"idle"`
	nice      float64 `json:"nice"`
	ioWait    float64 `json:"iowait"`
	irq       float64 `json:"irq"`
	softirq   float64 `json:"softirq"`
	steal     float64 `json:"steal"`
	guest     float64 `json:"guest"`
	guestNice float64 `json:"guestNice"`
}

// getCpuUtilization returns the utilization of each cpu in an interval
func getCpuUtilization(interval time.Duration) ([]float64, error) {
	ctx := context.Background()
	cpuTimesStat1, err := getCpuTimesStat(ctx)
	if err != nil {
		return nil, err
	}
	if err := wait(ctx, interval); err != nil {
		return nil, err
	}
	cpuTimesStat2, err := getCpuTimesStat(ctx)
	if err != nil {
		return nil, err
	}
	return calculateAllCpusUtilization(cpuTimesStat1, cpuTimesStat2)
}

func getCpuTimesStat(ctx context.Context) ([]cpuTimesStat, error) {
	filename := filepath.Join("/", sysfs.SysRoot(), "proc", "stat")
	lines := []string{}
	cpuLines, err := readCpuLines(filename)
	if err != nil || len(cpuLines) == 0 {
		return []cpuTimesStat{}, err
	}

	stat := make([]cpuTimesStat, 0, len(lines))
	for _, l := range cpuLines {
		oneStat, err := parseStatLine(l)
		if err != nil {
			continue
		}
		stat = append(stat, *oneStat)

	}
	return stat, nil
}

func wait(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(interval)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func calculateAllCpusUtilization(cts1, cts2 []cpuTimesStat) ([]float64, error) {
	if len(cts1) != len(cts2) {
		return nil, dynamicPoolsError("received two CPU counts: %d != %d", len(cts1), len(cts2))
	}
	allCpusUtilization := make([]float64, len(cts1))
	for i := 0; i < len(cts1); i++ {
		allCpusUtilization[i] = calculateOneCpuUtilization(cts1[i], cts2[i])
	}
	return allCpusUtilization, nil
}

//readCpuLines skips the first line indicating the total CPU utilization.
func readCpuLines(filename string) ([]string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var statLines []string
	reader := bufio.NewReader(f)
	for {
		line, _, err := reader.ReadLine()
		if err == io.EOF {
			break
		}
		statLines = append(statLines, string(line))
	}

	var cpuLines []string
	if len(statLines) < 2 {
		return nil, nil
	}
	for _, line := range statLines[1:] {
		if !strings.HasPrefix(line, "cpu") {
			break
		}
		cpuLines = append(cpuLines, line)
	}
	return cpuLines, nil
}

// parseStatLine is to parse cpuLine into cpuTimesStat.
func parseStatLine(cpuLine string) (*cpuTimesStat, error) {
	values := strings.Fields(cpuLine)
	if len(values) == 0 || len(values) < 8 {
		return nil, dynamicPoolsError("Stat does not contain cpu info.")
	}
	cpu := values[0]
	user, err := strconv.ParseFloat(values[1], 64)
	if err != nil {
		return nil, err
	}
	nice, err := strconv.ParseFloat(values[2], 64)
	if err != nil {
		return nil, err
	}
	system, err := strconv.ParseFloat(values[3], 64)
	if err != nil {
		return nil, err
	}
	idle, err := strconv.ParseFloat(values[4], 64)
	if err != nil {
		return nil, err
	}
	ioWait, err := strconv.ParseFloat(values[5], 64)
	if err != nil {
		return nil, err
	}
	irq, err := strconv.ParseFloat(values[6], 64)
	if err != nil {
		return nil, err
	}
	softirq, err := strconv.ParseFloat(values[7], 64)
	if err != nil {
		return nil, err
	}
	cts := &cpuTimesStat{
		cpu:     cpu,
		user:    user,
		nice:    nice,
		system:  system,
		idle:    idle,
		ioWait:  ioWait,
		irq:     irq,
		softirq: softirq,
	}
	if len(values) > 8 { // Linux >= 2.6.11
		steal, err := strconv.ParseFloat(values[8], 64)
		if err != nil {
			return nil, err
		}
		cts.steal = steal
	}
	if len(values) > 9 { // Linux >= 2.6.24
		guest, err := strconv.ParseFloat(values[9], 64)
		if err != nil {
			return nil, err
		}
		cts.guest = guest
	}
	if len(values) > 10 { // Linux >= 3.2.0
		guestNice, err := strconv.ParseFloat(values[10], 64)
		if err != nil {
			return nil, err
		}
		cts.guestNice = guestNice
	}
	return cts, nil
}

// calculateOneCpuUtilization returns the utilization of one cpu in an interval
func calculateOneCpuUtilization(cts1, cts2 cpuTimesStat) float64 {
	cts1Total, cts1Busy := getBusyTime(cts1)
	cts2Total, cts2Busy := getBusyTime(cts2)
	if cts2Busy <= cts1Busy {
		return 0
	}
	if cts2Total <= cts1Total {
		return 100
	}
	return math.Min(100, math.Max(0, (cts2Busy-cts1Busy)/(cts2Total-cts1Total)*100))
}

func getBusyTime(cts cpuTimesStat) (float64, float64) {
	total := cts.user + cts.system + cts.idle + cts.nice + cts.ioWait + cts.irq +
		cts.softirq + cts.steal + cts.guest + cts.guestNice
	if runtime.GOOS == "linux" {
		total -= cts.guest     // Linux 2.6.24+
		total -= cts.guestNice // Linux 3.2.0+
	}
	busy := total - cts.idle - cts.ioWait
	return total, busy
}
