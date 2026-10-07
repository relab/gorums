package stream

import (
	"sync"
	"time"
)

// noLatency is the estimate reported before the first sample.
const noLatency = -1 * time.Second

// Latency is a node's round-trip latency estimate: an exponentially weighted
// moving average with a smoothing factor of 0.2. A nil Latency has no estimate
// and ignores samples.
type Latency struct {
	mu       sync.Mutex
	estimate time.Duration
}

// newLatency returns a Latency with no estimate.
func newLatency() *Latency {
	return &Latency{estimate: noLatency}
}

// Load returns the current estimate, or -1s if there is none.
func (l *Latency) Load() time.Duration {
	if l == nil {
		return noLatency
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.estimate
}

// Store replaces the current estimate.
func (l *Latency) Store(estimate time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.estimate = estimate
}

// observe adds a round-trip sample to the estimate.
func (l *Latency) observe(rtt time.Duration) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.estimate < 0 {
		l.estimate = rtt
	} else {
		l.estimate = time.Duration(0.8*float64(l.estimate) + 0.2*float64(rtt))
	}
}
