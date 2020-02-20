package metrics

import (
	"fmt"
	logger "github.com/intel/cri-resource-manager/pkg/log"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	builtInCollectors    = make(map[string]InitCollector)
	registeredCollectors = []prometheus.Collector{}
	log                  = logger.NewLogger("collectors")
)

// InitCollector is the type for functions that initialize collectors.
type InitCollector func() (prometheus.Collector, error)

// RegisterCollector registers the named prometheus.Collector for metrics collection.
func RegisterCollector(name string, init InitCollector) error {
	log.Info("registering collector %s...", name)

	if _, found := builtInCollectors[name]; found {
		return metricsError("Collector %s already registered", name)
	}

	builtInCollectors[name] = init

	return nil
}

// NewMetricGatherer creates a new prometheus.Gatherer with all registered collectors.
func NewMetricGatherer() (prometheus.Gatherer, error) {
	reg := prometheus.NewPedanticRegistry()

	for n, cb := range builtInCollectors {
		c, err := cb()
		if err != nil {
			log.Error("Failed to initialize collector '%s': %v. Skipping it.", n, err)
			continue
		}
		registeredCollectors = append(registeredCollectors, c)
	}

	reg.MustRegister(registeredCollectors[:]...)

	return reg, nil
}

func metricsError(format string, args ...interface{}) error {
	return fmt.Errorf("metrics: "+format, args...)
}
