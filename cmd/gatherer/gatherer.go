package main

import (
	"bytes"
	"flag"
	"fmt"
	"github.com/intel/cri-resource-manager/pkg/metrics"
	_ "github.com/intel/cri-resource-manager/pkg/metrics/register"
	"github.com/prometheus/common/expfmt"
	"os"
	"time"
)

func main() {

	g, err := metrics.NewMetricGatherer()
	if err != nil {
		fmt.Printf("Unable to create Metrics Collector: %+v\n", err)
		os.Exit(1)
	}

	flag.Parse()

	for {
		time.Sleep(5 * time.Second)
		mfs, err := g.Gather()
		if err != nil {
			fmt.Printf("Error in collecting metrics: %+v\n", err)
			continue
		}
		for _, mf := range mfs {
			out := &bytes.Buffer{}
			if _, err = expfmt.MetricFamilyToText(out, mf); err != nil {
				panic(err)
			}
			fmt.Print(out)
		}
	}
}
