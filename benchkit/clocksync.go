package benchkit

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/relab/gorums"
)

// clockSyncRounds is the number of NTP-style ClockSync exchanges performed per
// EstimateOffsets call. Per peer, the estimate from the round with the smallest
// round-trip delay is kept, following NTP's min-filter heuristic to suppress
// queuing noise on the probe path. More rounds raise the chance of catching a
// low-jitter sample and so tighten the offset estimate, at a setup cost of
// round-trips x rounds per call (paid twice, before and after the window); on a
// LAN that is negligible, so this is set well above NTP's typical burst of 4-8.
// It cannot correct systematic path asymmetry, which no number of rounds removes.
const clockSyncRounds = 50

// clockOffset computes the NTP clock offset and round-trip delay for a single
// exchange: t1 is the local time just before the request, serverTime is the
// server's time stamped in the reply, and t4 is the local time the reply
// arrived. Assuming a symmetric path, the offset (peer clock minus this node's
// clock) is serverTime - (t1+t4)/2 and the delay is t4 - t1.
func clockOffset(t1, serverTime, t4 int64) (offset, delay int64) {
	return serverTime - (t1+t4)/2, t4 - t1
}

// EstimateOffsets runs clockSyncRounds NTP-style ClockSync exchanges from cfg's
// client to every peer and returns, per peer node ID, the estimated clock offset
// in nanoseconds (peer clock minus this node's clock).
//
// For each reply, with t1 the local time just before the call, t4 the local
// time the reply arrived, and st the server's time stamped in the reply, the
// offset is theta = st - (t1+t4)/2 and the round-trip delay is delay = t4 - t1
// (assuming a symmetric path). The offset from the round with the smallest delay
// is kept for each peer. The returned offset of a node relative to itself
// (loopback) is approximately zero.
func EstimateOffsets(ctx context.Context, cfg gorums.Config) (map[uint32]int64, error) {
	cfgCtx := cfg.Context(ctx)
	n := cfg.Size()
	bestDelay := make(map[uint32]int64, n)
	offsets := make(map[uint32]int64, n)
	for range clockSyncRounds {
		t1 := time.Now().UnixNano()
		for r := range ClockSync(cfgCtx, &ClockSyncRequest{}).Results() {
			t4 := time.Now().UnixNano()
			if r.Err != nil {
				continue
			}
			theta, delay := clockOffset(t1, r.Value.GetServerTime(), t4)
			if best, ok := bestDelay[r.NodeID]; !ok || delay < best {
				bestDelay[r.NodeID] = delay
				offsets[r.NodeID] = theta
			}
		}
	}
	if len(offsets) < n {
		return nil, fmt.Errorf("clock sync incomplete: got offsets for %d of %d peers", len(offsets), n)
	}
	return offsets, nil
}

// CorrectLatencies subtracts the given clock offset (peer clock minus this
// node's clock, in nanoseconds) from every latency sample in r, removing the
// cross-machine clock skew baked into a server-measured one-way latency. Used
// coordinator-side where a server's samples all come from a single sender.
//
// In StatsMode_HDR, where r carries a histogram instead of raw samples, the
// subtraction is applied to the histogram bucket values and the result is
// re-quantized onto the canonical HDR layout: the offset is a per-server
// additive constant, so it shifts the distribution without changing its shape.
func CorrectLatencies(r *Result, offset int64) {
	if offset == 0 {
		return
	}
	if lat := r.GetLatencies(); len(lat) > 0 {
		for i := range lat {
			lat[i] -= offset
		}
		r.SetLatencies(lat)
		return
	}
	if h := r.GetHistogram(); h != nil {
		r.SetHistogram(offsetHistogram(h, -offset))
	}
}

// LogOffsets prints the estimated per-peer clock offsets and the drift between
// the before and after samples. The values are diagnostics: a large offset
// indicates significant clock skew between machines, and a large drift suggests
// the clocks moved relative to each other during the run. The self/loopback
// peer should report an offset near zero. It is emitted unconditionally (via
// benchkit.Printf, not the -verbose Logf) because the offsets document how a
// server-measured latency was corrected: recording them in every run's collected
// log lets a corrected result — including one whose smallest samples land below
// zero from residual estimation error — be audited after the fact.
func LogOffsets(label string, before, after map[uint32]int64) {
	for _, id := range slices.Sorted(maps.Keys(before)) {
		b, a := before[id], after[id]
		Printf("[offsets %s] peer %d: before=%v after=%v drift=%v\n",
			label, id, time.Duration(b), time.Duration(a), time.Duration(a-b))
	}
}

// AverageOffsets returns the per-key mean of two offset maps. A key present in
// only one map is carried through unchanged, so a transient gap in one sample
// does not drop a peer's correction entirely.
func AverageOffsets(a, b map[uint32]int64) map[uint32]int64 {
	out := make(map[uint32]int64, len(a))
	for id, va := range a {
		if vb, ok := b[id]; ok {
			out[id] = (va + vb) / 2
		} else {
			out[id] = va
		}
	}
	for id, vb := range b {
		if _, ok := a[id]; !ok {
			out[id] = vb
		}
	}
	return out
}
