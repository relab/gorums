package broadcast

import (
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
	start        time.Time
	TotalNum     uint64
	Goroutines   map[string]*goroutineMetric
	FinishedReqs struct {
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

func NewMetric() Metrics {
	return Metrics{
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

func (m Metrics) GetStats() Metrics {
	if m.Processed > 0 {
		m.ReqLatency.Avg /= time.Duration(m.Processed)
	}
	if m.FinishedReqs.Total > 0 {
		m.RoundTripLatency.Avg /= time.Duration(m.FinishedReqs.Total)
	}
	return m
}
