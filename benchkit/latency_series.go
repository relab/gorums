package benchkit

import (
	"maps"
	"math"
	"slices"
	"time"
)

// latencyMoments holds the count, mean, and sum of squared deviations of a
// sample set. A clock correction shifts mean but leaves m2 unchanged.
type latencyMoments struct {
	count uint64
	mean  float64
	m2    float64
}

func (m *latencyMoments) add(ns int64) {
	m.count++
	delta := float64(ns) - m.mean
	m.mean += delta / float64(m.count)
	m.m2 += delta * (float64(ns) - m.mean)
}

func (m *latencyMoments) merge(other latencyMoments) {
	if other.count == 0 {
		return
	}
	if m.count == 0 {
		*m = other
		return
	}
	count := m.count + other.count
	delta := other.mean - m.mean
	m.m2 += other.m2 + delta*delta*float64(m.count)*float64(other.count)/float64(count)
	m.mean += delta * float64(other.count) / float64(count)
	m.count = count
}

func (m latencyMoments) stddev() float64 {
	if m.count > 1 {
		return math.Sqrt(m.m2 / float64(m.count-1))
	}
	return 0
}

func (m latencyMoments) event(offset int64) *Event {
	return Event_builder{
		Offset: offset,
		Latency: LatencyInterval_builder{
			Count: m.count, Mean: m.mean, Stddev: m.stddev(),
		}.Build(),
	}.Build()
}

// latencySeries keeps per-sender moments in receive-time buckets aligned to
// the client's START. Bucket keys are their exclusive upper bounds, in
// nanoseconds since START. Only occupied buckets allocate storage; memory is
// proportional to intervals and senders, independent of the sample count.
// The owning Stats mutex protects all access.
type latencySeries struct {
	start    time.Time
	interval time.Duration
	buckets  map[int64]map[uint32]latencyMoments
}

func (s *latencySeries) add(now time.Time, sender uint32, ns int64) {
	if s == nil {
		return
	}
	offset := now.Sub(s.start)
	if offset < 0 {
		return
	}
	end := int64(offset.Truncate(s.interval) + s.interval)
	bySender := s.buckets[end]
	if bySender == nil {
		bySender = make(map[uint32]latencyMoments)
		s.buckets[end] = bySender
	}
	m := bySender[sender]
	m.add(ns)
	bySender[sender] = m
}

func (s *latencySeries) corrected(offsets map[uint32]int64) []*Event {
	if s == nil {
		return nil
	}
	events := make([]*Event, 0, len(s.buckets))
	for _, end := range slices.Sorted(maps.Keys(s.buckets)) {
		var merged latencyMoments
		bySender := s.buckets[end]
		for _, id := range slices.Sorted(maps.Keys(bySender)) {
			m := bySender[id]
			m.mean += float64(offsets[id])
			merged.merge(m)
		}
		events = append(events, merged.event(end))
	}
	return events
}

// mergeLatencyEvents combines server intervals with the same bucket end.
// Their means are weighted by sample count, and their sample variances include
// both within-server and between-server variation.
func mergeLatencyEvents(events []*Event) []*Event {
	buckets := make(map[int64]latencyMoments)
	for _, e := range events {
		lat := e.GetLatency()
		if lat == nil || lat.GetCount() == 0 {
			continue
		}
		m := buckets[e.GetOffset()]
		m.merge(latencyMoments{
			count: lat.GetCount(), mean: lat.GetMean(),
			m2: lat.GetStddev() * lat.GetStddev() * float64(lat.GetCount()-1),
		})
		buckets[e.GetOffset()] = m
	}
	merged := make([]*Event, 0, len(buckets))
	for _, end := range slices.Sorted(maps.Keys(buckets)) {
		merged = append(merged, buckets[end].event(end))
	}
	return merged
}
