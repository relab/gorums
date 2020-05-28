package benchmark

import (
	fmt "fmt"
	"math"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Result contains information about a benchmark run
type Result struct {
	Name        string
	TotalOps    uint64        // total number of operations
	TotalTime   time.Duration // The total time taken
	Throughput  float64       // throughput measured in Kops/sec
	LatencyAvg  float64       // average latency measured in ms
	LatencyVar  float64       // latency variance
	AllocsPerOp uint64        // average number of memory allocations per operation
	MemPerOp    uint64        // average amount of bytes allocated per operation
}

func (r Result) String() string {
	b := new(strings.Builder)
	fmt.Fprintf(b, "%s\t", r.Name)
	fmt.Fprintf(b, "%.2f Kops/sec\t", r.Throughput)
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
func (s *Stats) GetResult() Result {
	s.mut.Lock()
	defer s.mut.Unlock()

	totalOps := uint64(len(s.latencies))

	r := Result{}

	r.Throughput = float64(totalOps) / s.endTime.Sub(s.startTime).Seconds()

	var latencySum time.Duration
	for _, l := range s.latencies {
		latencySum += l
	}
	r.LatencyAvg = float64(latencySum.Milliseconds()) / float64(totalOps)

	for _, l := range s.latencies {
		r.LatencyVar += math.Pow(float64(l.Milliseconds())-r.LatencyAvg, 2)
	}
	r.LatencyVar /= float64(totalOps - 1)

	r.AllocsPerOp = (s.endMs.Mallocs - s.startMs.Mallocs) / totalOps
	r.MemPerOp = (s.endMs.TotalAlloc - s.startMs.TotalAlloc) / totalOps
	return r
}
