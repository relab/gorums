package broadcast

import (
	"fmt"
	"sync"
	"time"
)

type timingMetric struct {
	Avg time.Duration
	//Median time.Duration
	Min time.Duration
	Max time.Duration
}

type goroutineMetric struct {
	started time.Time
	ended   time.Time
}

type Metrics struct {
	start             time.Time
	TotalNum          uint64
	GoroutinesStarted uint64
	GoroutinesStopped uint64
	Goroutines        map[string]*goroutineMetric
	FinishedReqs      struct {
		Total     uint64
		Succesful uint64
		Failed    uint64
	}
	Processed         uint64
	Dropped           uint64
	Invalid           uint64
	AlreadyProcessed  uint64
	RoundTripLatency  timingMetric
	ReqLatency        timingMetric
	ShardDistribution map[uint32]uint64
	// measures unique number of broadcastIDs processed simultaneounsly
	ConcurrencyDistribution timingMetric
}

type Metric struct {
	mut sync.Mutex
	m   Metrics
}

func NewMetric() *Metric {
	return &Metric{
		m: Metrics{
			start:             time.Now(),
			ShardDistribution: make(map[uint32]uint64, 16),
			Goroutines:        make(map[string]*goroutineMetric, 2000),
			RoundTripLatency: timingMetric{
				Min: 100 * time.Hour,
			},
			ReqLatency: timingMetric{
				Min: 100 * time.Hour,
			},
		},
	}
}

func (m *Metric) Reset() {
	m.mut.Lock()
	defer m.mut.Unlock()
	m.m = Metrics{
		start:             time.Now(),
		ShardDistribution: make(map[uint32]uint64, 16),
		Goroutines:        make(map[string]*goroutineMetric, 2000),
		RoundTripLatency: timingMetric{
			Min: 100 * time.Hour,
		},
		ReqLatency: timingMetric{
			Min: 100 * time.Hour,
		},
	}
}

func (m Metrics) String() string {
	dur := time.Since(m.start)
	res := "Metrics:"
	res += "\n\t- TotalTime: " + fmt.Sprintf("%v", dur)
	if m.FinishedReqs.Total > 0 {
		res += "\n\t- ReqTime: " + fmt.Sprintf("%v", dur/time.Duration(m.FinishedReqs.Total))
	}
	res += "\n\t- TotalNum: " + fmt.Sprintf("%v", m.TotalNum)
	res += "\n\t- Goroutines started: " + fmt.Sprintf("%v", m.GoroutinesStarted)
	res += "\n\t- Goroutines stopped: " + fmt.Sprintf("%v", m.GoroutinesStopped)
	res += "\n\t- FinishedReqs:"
	res += "\n\t\t- Total: " + fmt.Sprintf("%v", m.FinishedReqs.Total)
	res += "\n\t\t- Successful: " + fmt.Sprintf("%v", m.FinishedReqs.Succesful)
	res += "\n\t\t- Failed: " + fmt.Sprintf("%v", m.FinishedReqs.Failed)
	res += "\n\t- Processed: " + fmt.Sprintf("%v", m.Processed)
	res += "\n\t- Dropped: " + fmt.Sprintf("%v", m.Dropped)
	res += "\n\t\t- Invalid: " + fmt.Sprintf("%v", m.Invalid)
	res += "\n\t\t- AlreadyProcessed: " + fmt.Sprintf("%v", m.AlreadyProcessed)
	res += "\n\t- RoundTripLatency:"
	res += "\n\t\t- Avg: " + fmt.Sprintf("%v", m.RoundTripLatency.Avg)
	res += "\n\t\t- Min: " + fmt.Sprintf("%v", m.RoundTripLatency.Min)
	res += "\n\t\t- Max: " + fmt.Sprintf("%v", m.RoundTripLatency.Max)
	res += "\n\t- ReqLatency:"
	res += "\n\t\t- Avg: " + fmt.Sprintf("%v", m.ReqLatency.Avg)
	res += "\n\t\t- Min: " + fmt.Sprintf("%v", m.ReqLatency.Min)
	res += "\n\t\t- Max: " + fmt.Sprintf("%v", m.ReqLatency.Max)
	res += "\n\t- ShardDistribution: " + fmt.Sprintf("%v", m.ShardDistribution)
	return res
}

func (m *Metric) GetStats() Metrics {
	if m == nil {
		return Metrics{}
	}
	m.mut.Lock()
	defer m.mut.Unlock()
	if m.m.Processed > 0 {
		m.m.ReqLatency.Avg /= time.Duration(m.m.Processed)
	}
	if m.m.FinishedReqs.Total > 0 {
		m.m.RoundTripLatency.Avg /= time.Duration(m.m.FinishedReqs.Total)
	}
	return m.m
}

func (m *Metric) AddReqLatency(start time.Time) {
	latency := time.Since(start)
	m.mut.Lock()
	defer m.mut.Unlock()
	if latency > m.m.ReqLatency.Max {
		m.m.ReqLatency.Max = latency
	}
	if latency < m.m.ReqLatency.Min {
		m.m.ReqLatency.Min = latency
	}
	m.m.ReqLatency.Avg += latency
}

func (m *Metric) AddRoundTripLatency(start time.Time) {
	latency := time.Since(start)
	m.mut.Lock()
	defer m.mut.Unlock()
	if latency > m.m.RoundTripLatency.Max {
		m.m.RoundTripLatency.Max = latency
	}
	if latency < m.m.RoundTripLatency.Min {
		m.m.RoundTripLatency.Min = latency
	}
	m.m.RoundTripLatency.Avg += latency
	m.m.FinishedReqs.Total++
}

func (m *Metric) AddFinishedSuccessful() {
	m.mut.Lock()
	defer m.mut.Unlock()
	m.m.FinishedReqs.Succesful = m.m.FinishedReqs.Succesful + 1
}

func (m *Metric) AddFinishedFailed() {
	m.mut.Lock()
	defer m.mut.Unlock()
	m.m.FinishedReqs.Failed++
}

func (m *Metric) AddMsg() {
	m.mut.Lock()
	defer m.mut.Unlock()
	m.m.TotalNum++
}

func (m *Metric) AddGoroutine(broadcastID uint64, name string) {
	m.mut.Lock()
	defer m.mut.Unlock()
	m.m.GoroutinesStarted++
	index := fmt.Sprintf("%s.%v", name, broadcastID)
	m.m.Goroutines[index] = &goroutineMetric{
		started: time.Now(),
	}
}

func (m *Metric) RemoveGoroutine(broadcastID uint64, name string) {
	m.mut.Lock()
	defer m.mut.Unlock()
	m.m.GoroutinesStopped++
	index := fmt.Sprintf("%s.%v", name, broadcastID)
	if g, ok := m.m.Goroutines[index]; ok {
		g.ended = time.Now()
		//} else {
		//panic("what")
	}
}

func (m *Metric) AddDropped(invalid bool) {
	m.mut.Lock()
	defer m.mut.Unlock()
	m.m.Dropped++
	if invalid {
		m.m.Invalid++
	} else {
		m.m.AlreadyProcessed++
	}
}

func (m *Metric) AddProcessed() {
	m.mut.Lock()
	defer m.mut.Unlock()
	m.m.Processed++
}

func (m *Metric) AddShardDistribution(i int) {
	m.mut.Lock()
	defer m.mut.Unlock()
	m.m.ShardDistribution[uint32(i)]++
}
