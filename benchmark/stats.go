package benchmark

import (
	fmt "fmt"
	"math"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Format returns a tab formatted string representation of the result
func (r *Result) Format() string {
	b := new(strings.Builder)
	fmt.Fprintf(b, "%s\t", r.Name)
	fmt.Fprintf(b, "%.2f ops/sec\t", r.Throughput)
	fmt.Fprintf(b, "%.2f ms\t", r.LatencyAvg)
	fmt.Fprintf(b, "%.2f ms\t", math.Sqrt(r.LatencyVar))
	fmt.Fprintf(b, "%d B/op\t", r.MemPerOp)
	fmt.Fprintf(b, "%d allocs/op\t", r.AllocsPerOp)
	return b.String()
}

// Stats records and processes the raw data of a benchmark
type Stats struct {
	mut       sync.Mutex
	latencies []time.Duration
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

	s.latencies = append(s.latencies, l)
}

// GetResult computes and returns the result of the benchmark
func (s *Stats) GetResult() *Result {
	s.mut.Lock()
	defer s.mut.Unlock()

	r := &Result{}
	r.TotalOps = uint64(len(s.latencies))
	r.TotalTime = int64(s.endTime.Sub(s.startTime))
	r.Throughput = float64(r.TotalOps) / float64(time.Duration(r.TotalTime).Seconds())

	var latencySum time.Duration
	for _, l := range s.latencies {
		latencySum += l
	}
	r.LatencyAvg = float64(latencySum.Milliseconds()) / float64(r.TotalOps)

	for _, l := range s.latencies {
		r.LatencyVar += math.Pow(float64(l.Milliseconds())-r.LatencyAvg, 2)
	}
	r.LatencyVar /= float64(r.TotalOps - 1)

	r.AllocsPerOp = (s.endMs.Mallocs - s.startMs.Mallocs) / r.TotalOps
	r.MemPerOp = (s.endMs.TotalAlloc - s.startMs.TotalAlloc) / r.TotalOps
	return r
}

// Clear zeroes out the stats
func (s *Stats) Clear() {
	s.mut.Lock()
	s.latencies = nil
	s.startTime = time.Time{}
	s.endTime = time.Time{}
	s.startMs = runtime.MemStats{}
	s.endMs = runtime.MemStats{}
	s.mut.Unlock()
}
