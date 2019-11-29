package metrics

import (
	"fmt"
	logger "github.com/intel/cri-resource-manager/pkg/log"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	builtInCollectors    = make(map[string]InitCollector)
	registeredCollectors = []prometheus.Collector{}
	log                  = logger.NewLogger("metrics")
)

type InitCollector func() (prometheus.Collector, error)

func RegisterCollector(name string, init InitCollector) error {
	log.Info("registering collector %s...", name)

	if _, found := builtInCollectors[name]; found {
		return metricsError("Collector %s already registered", name)
	}

	builtInCollectors[name] = init

	return nil
}

func NewMetricGatherer() (prometheus.Gatherer, error) {
	reg := prometheus.NewPedanticRegistry()

	for _, cb := range builtInCollectors {
		c, err := cb()
		if err != nil {
			return nil, err
		}
		registeredCollectors = append(registeredCollectors, c)
	}

	reg.MustRegister(registeredCollectors[:]...)

	return reg, nil
}

func metricsError(format string, args ...interface{}) error {
	return fmt.Errorf("metrics: "+format, args...)
}
