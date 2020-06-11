/*
Copyright 2020 Intel Corporation

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

package metricsring

import (
	"container/ring"
	"github.com/VividCortex/ewma"
	"time"
)

type SampleBuffer interface {
	Push(d float64)
	EWMA() float64
	GetTime() time.Duration
	GetSize() int
	GetLastNSamples(count int) []float64
}

// MetricsRing implements SampleBufer interface
type MetricsRing struct {
	r  *ring.Ring
	s  int // the count of elements in the ring
	ma ewma.MovingAverage
}

type sample struct {
	s         float64
	timestamp time.Time
}

func NewMetricsRing(ringlen int) SampleBuffer {
	// Note: ewma has warm-up period of 10 samples. with ringlen < 10
	// EWMA() returns 0.0 always.

	return &MetricsRing{
		r:  ring.New(ringlen),
		ma: ewma.NewMovingAverage(float64(ringlen)),
	}
}

func (mr *MetricsRing) GetTime() time.Duration {
	t1 := mr.r.Prev().Value.(sample).timestamp

	return t1.Sub(mr.r.Move(-1 * mr.s).Value.(sample).timestamp)
}

func (mr *MetricsRing) EWMA() float64 {
	return mr.ma.Value()
}

func (mr *MetricsRing) Push(d float64) {

	mr.r.Value = sample{
		s:         d,
		timestamp: time.Now(),
	}

	// Add to MovingAverage
	mr.ma.Add(d)

	mr.r = mr.r.Next()

	if mr.s+1 <= mr.r.Len() {
		mr.s++
	}
}

func (mr *MetricsRing) GetSize() int {
	return mr.r.Len()
}

func (mr *MetricsRing) GetLastNSamples(count int) []float64 {

	sliceLen := count
	if sliceLen > mr.r.Len() {
		sliceLen = mr.r.Len()
	}
	if sliceLen > mr.s {
		// ring does not have enough elements yet
		sliceLen = mr.s
	}

	s := make([]float64, sliceLen)

	// Move backwards in the ring
	mr.r = mr.r.Move(-1 * sliceLen)

	for i := 0; i < sliceLen; i++ {
		s[i] = mr.r.Value.(sample).s
		mr.r = mr.r.Next()
	}

	return s
}
