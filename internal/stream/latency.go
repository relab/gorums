package stream

import (
	"sync"
	"time"
)

// noLatency is the estimate reported before the first sample.
const noLatency = -1 * time.Second

// Latency holds a node's round-trip latency estimates: one for all methods and
// one for each method. Each is an exponentially weighted moving average with a
// smoothing factor of 0.2. A nil Latency has no estimates and ignores samples.
type Latency struct {
	mu       sync.Mutex
	estimate time.Duration
	byMethod map[string]time.Duration
}

// newLatency returns a Latency with no estimates.
func newLatency() *Latency {
	return &Latency{estimate: noLatency}
}

// Load returns the estimate for all methods, or -1s if there is none.
func (l *Latency) Load() time.Duration {
	if l == nil {
		return noLatency
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.estimate
}

// LoadMethod returns the estimate for method, or -1s if there is none.
func (l *Latency) LoadMethod(method string) time.Duration {
	if l == nil {
		return noLatency
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if estimate, ok := l.byMethod[method]; ok {
		return estimate
	}
	return noLatency
}

// observe adds a round-trip sample of method to the estimates.
func (l *Latency) observe(method string, rtt time.Duration) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.estimate = ewma(l.estimate, rtt)
	if l.byMethod == nil {
		l.byMethod = make(map[string]time.Duration)
	}
	prev, ok := l.byMethod[method]
	if !ok {
		prev = noLatency
	}
	l.byMethod[method] = ewma(prev, rtt)
}

// ewma returns the exponentially weighted moving average after adding sample
// to prev, or sample if prev is -1s.
func ewma(prev, sample time.Duration) time.Duration {
	if prev < 0 {
		return sample
	}
	return time.Duration(0.8*float64(prev) + 0.2*float64(sample))
}

// Store replaces the estimate for all methods. For testing only.
func (l *Latency) Store(estimate time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.estimate = estimate
}
