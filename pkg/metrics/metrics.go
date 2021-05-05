package metrics

import (
	"fmt"
	logger "github.com/intel/cri-resource-manager/pkg/log"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	builtInCollectors     = make(map[string]InitCollector)
	registeredCollectors  = []prometheus.Collector{}
	initializedCollectors = make(map[string]struct{})
	log                   = logger.NewLogger("collectors")
)

// InitCollector is the type for functions that initialize collectors.
type InitCollector func() (prometheus.Collector, error)

// RegisterCollector registers the named prometheus.Collector for metrics collection.
func RegisterCollector(name string, init InitCollector) error {
	log.Infof("registering collector %s...", name)

	if _, found := builtInCollectors[name]; found {
		return metricsError("Collector %s already registered", name)
	}

	builtInCollectors[name] = init

	return nil
}

// NewMetricGatherer creates a new prometheus.Gatherer with all registered collectors.
func NewMetricGatherer() (prometheus.Gatherer, error) {
	reg := prometheus.NewPedanticRegistry()

	for name, cb := range builtInCollectors {
		if _, ok := initializedCollectors[name]; ok {
			continue
		}

		c, err := cb()
		if err != nil {
			log.Errorf("Failed to initialize collector '%s': %v. Skipping it.", name, err)
			continue
		}
		registeredCollectors = append(registeredCollectors, c)
		initializedCollectors[name] = struct{}{}
	}

	reg.MustRegister(registeredCollectors[:]...)

	return reg, nil
}

func metricsError(format string, args ...interface{}) error {
	return fmt.Errorf("metrics: "+format, args...)
}
