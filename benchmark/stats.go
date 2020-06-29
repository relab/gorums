package benchmark

import (
	fmt "fmt"
	math "math"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/eclesh/welford"
)

// Format returns a tab formatted string representation of the result
func (r *Result) Format() string {
	b := new(strings.Builder)
	fmt.Fprintf(b, "%s\t", r.Name)
	fmt.Fprintf(b, "%.2f ops/sec\t", r.Throughput)
	fmt.Fprintf(b, "%.2f ms\t", r.LatencyAvg/float64(time.Millisecond))
	fmt.Fprintf(b, "%.2f ms\t", math.Sqrt(r.LatencyVar)/float64(time.Millisecond))
	fmt.Fprintf(b, "%d B/op\t", r.MemPerOp)
	fmt.Fprintf(b, "%d allocs/op\t", r.AllocsPerOp)
	return b.String()
}

// Stats records and processes the raw data of a benchmark
type Stats struct {
	mut       sync.Mutex
	stats     welford.Stats
	startTime time.Time
	endTime   time.Time
	startMs   runtime.MemStats
	endMs     runtime.MemStats
}

// Start records the start time and memory stats
func (s *Stats) Start() {
	s.mut.Lock()
	defer s.mut.Unlock()

	runtime.ReadMemStats(&s.startMs)
	s.startTime = time.Now()
}

// End records the end time and memory stats
func (s *Stats) End() {
	s.mut.Lock()
	defer s.mut.Unlock()

	s.endTime = time.Now()
	runtime.ReadMemStats(&s.endMs)
}

// AddLatency adds a latency measurement
func (s *Stats) AddLatency(l time.Duration) {
	s.mut.Lock()
	defer s.mut.Unlock()

	s.stats.Add(float64(l))
}

// GetResult computes and returns the result of the benchmark
func (s *Stats) GetResult() *Result {
	s.mut.Lock()
	defer s.mut.Unlock()

	r := &Result{}
	r.TotalOps = s.stats.Count()
	r.TotalTime = int64(s.endTime.Sub(s.startTime))
	r.Throughput = float64(r.TotalOps) / float64(time.Duration(r.TotalTime).Seconds())
	r.LatencyAvg = s.stats.Mean()
	r.LatencyVar = s.stats.Variance()
	r.AllocsPerOp = (s.endMs.Mallocs - s.startMs.Mallocs) / r.TotalOps
	r.MemPerOp = (s.endMs.TotalAlloc - s.startMs.TotalAlloc) / r.TotalOps
	return r
}

// Clear zeroes out the stats
func (s *Stats) Clear() {
	s.mut.Lock()
	s.stats.Reset()
	s.startTime = time.Time{}
	s.endTime = time.Time{}
	s.startMs = runtime.MemStats{}
	s.endMs = runtime.MemStats{}
	s.mut.Unlock()
}
