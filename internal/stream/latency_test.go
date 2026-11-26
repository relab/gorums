package stream

import (
	"testing"
	"time"
)

func TestLatencyObserve(t *testing.T) {
	const ms = time.Millisecond
	type sample struct {
		method string
		rtt    time.Duration
	}
	tests := []struct {
		name    string
		samples []sample
		wantRTT time.Duration
		want    map[string]time.Duration
	}{
		{
			name:    "ColdStart",
			wantRTT: noLatency,
			want:    map[string]time.Duration{"m": noLatency},
		},
		{
			name:    "FirstSample",
			samples: []sample{{"m", 10 * ms}},
			wantRTT: 10 * ms,
			want:    map[string]time.Duration{"m": 10 * ms, "other": noLatency},
		},
		{
			name:    "EWMA",
			samples: []sample{{"m", 10 * ms}, {"m", 20 * ms}},
			wantRTT: 12 * ms,
			want:    map[string]time.Duration{"m": 12 * ms},
		},
		{
			name:    "MethodsSeparate",
			samples: []sample{{"fast", ms}, {"slow", 10 * ms}},
			wantRTT: 2800 * time.Microsecond,
			want:    map[string]time.Duration{"fast": ms, "slow": 10 * ms},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := newLatency()
			for _, s := range tt.samples {
				l.observe(s.method, s.rtt)
			}
			if got := l.Load(); got != tt.wantRTT {
				t.Errorf("Load() = %v, want %v", got, tt.wantRTT)
			}
			for method, want := range tt.want {
				if got := l.LoadMethod(method); got != want {
					t.Errorf("LoadMethod(%q) = %v, want %v", method, got, want)
				}
			}
		})
	}
}

func TestLatencyNil(t *testing.T) {
	var l *Latency
	l.observe("m", time.Millisecond)
	if got := l.Load(); got != noLatency {
		t.Errorf("Load() = %v, want %v", got, noLatency)
	}
	if got := l.LoadMethod("m"); got != noLatency {
		t.Errorf("LoadMethod() = %v, want %v", got, noLatency)
	}
}
