package metrics

import (
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	builtInCollectors    = make(map[string]InitCollector)
	registeredCollectors = []prometheus.Collector{}
)

type InitCollector func() (prometheus.Collector, error)

func RegisterCollector(name string, init InitCollector) error {
	if _, found := builtInCollectors[name]; found {
		return fmt.Errorf("Collector %s already registered", name)
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
