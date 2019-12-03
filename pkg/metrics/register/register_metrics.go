package register

import (
	// Pull in avx collector.
	_ "github.com/intel/cri-resource-manager/pkg/avx"
	// Pull in cgroup-based metric collector.
	_ "github.com/intel/cri-resource-manager/pkg/cgroupstats"
)
