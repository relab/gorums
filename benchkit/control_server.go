package benchkit

import (
	"sync/atomic"
	"time"

	"github.com/relab/gorums"
)

// Control is the [Stats]-backed implementation of the generated
// ControlServer interface: the protocol-neutral measurement control plane. A
// protocol binary registers a Control alongside its own workload server on
// the same listener (see [RegisterControlServer]), and the workload handlers
// record into the same Stats instance via [Control.Stats] and
// [Control.RecordOp] so the Stop reply observes their work.
type Control struct {
	stats  *Stats
	ops    atomic.Uint64
	selfID uint32 // this node's gorums node ID

	// Done tracking (see [Control.ArmDone]): doneSeen[id] marks that sender
	// id has signaled, doneLeft counts remaining distinct signals, and
	// doneCh closes once doneLeft reaches zero. Nil/zero until ArmDone is
	// called; Done is then a no-op, which is the default for modes that
	// never need the signal.
	doneSeen []atomic.Bool
	doneLeft atomic.Int32
	doneCh   chan struct{}
}

// NewControl creates a Control server backed by a fresh Stats instance.
func NewControl() *Control {
	return &Control{stats: new(Stats)}
}

// SetID records this node's gorums node ID. Workload senders tag messages with
// this ID so the receiving server can attribute samples per sender. Call it
// from the server registration closure, once the node ID is known.
func (c *Control) SetID(id uint32) { c.selfID = id }

// Stats returns the Stats instance backing this control server. The protocol's
// workload handlers record server-measured latencies here so the Stop reply and
// the handlers observe the same samples.
func (c *Control) Stats() *Stats { return c.stats }

// SelfID returns this node's gorums node ID.
func (c *Control) SelfID() uint32 { return c.selfID }

// ArmDone configures advisory Done tracking for `total` distinct peer
// signals (sender IDs 1..total) and returns the channel that closes once
// every one of them has signaled. Call sites that never need the signal
// (e.g. local or coordinator mode) simply never call ArmDone: Done is a
// no-op and DoneCh returns nil until armed. Arming for no peers at all
// returns an already-closed channel, since there is nothing to wait for.
func (c *Control) ArmDone(total int) <-chan struct{} {
	c.doneSeen = make([]atomic.Bool, max(total, 0)+1) // index 0 unused; IDs are 1..total
	c.doneLeft.Store(int32(total))
	c.doneCh = make(chan struct{})
	if total <= 0 {
		// Done closes doneCh only when a signal drives doneLeft to zero, and no
		// signal can arrive: every sender ID is out of doneSeen's range.
		close(c.doneCh)
	}
	return c.doneCh
}

// DoneCh returns the channel armed by ArmDone, or nil if Done tracking was
// never armed.
func (c *Control) DoneCh() <-chan struct{} { return c.doneCh }

// DoneCount returns the number of distinct peers that have signaled Done, or 0
// if Done tracking was never armed. A straggler uses it to tell whether a
// failed cross-node call is the expected consequence of peers finishing and
// exiting rather than a fault.
func (c *Control) DoneCount() int {
	var n int
	for id := 1; id < len(c.doneSeen); id++ {
		if c.doneSeen[id].Load() {
			n++
		}
	}
	return n
}

// MissingDone returns the sender IDs that have not yet signaled Done, for
// diagnostics when a caller's wait falls back to its own timeout instead of
// observing DoneCh close. Returns nil if Done tracking was never armed.
func (c *Control) MissingDone() []uint32 {
	var missing []uint32
	for id := 1; id < len(c.doneSeen); id++ {
		if !c.doneSeen[id].Load() {
			missing = append(missing, uint32(id))
		}
	}
	return missing
}

// RecordOp increments the server-side operation counter. Workload handlers call
// it once per handled operation so client-measured benchmarks can derive per-op
// memory stats in Stop.
func (c *Control) RecordOp() { c.ops.Add(1) }

// Reset clears the op counter, reconfigures the Stats for the given aggregate
// store mode, and starts a fresh measurement window. This is the body of the
// Start RPC, also reused by server-measured benchmarks to reset counters before
// the measurement window; mode carries the client's -stats-mode so a
// server-measured benchmark bounds memory in StatsMode_HDR like the client path.
func (c *Control) Reset(mode StatsMode) {
	c.ops.Store(0)
	c.stats.Reset(mode)
	c.stats.Start()
}

// ClockSync returns the server's current wall-clock time in nanoseconds.
// Peers use it for NTP-style clock-offset estimation when correcting the
// one-way latencies measured by server-measured benchmarks.
func (c *Control) ClockSync(_ gorums.ServerContext, _ *ClockSyncRequest) (*ClockSyncResponse, error) {
	return ClockSyncResponse_builder{ServerTime: time.Now().UnixNano()}.Build(), nil
}

// Start resets the server's op counter and Stats baseline, building the
// aggregate store in the mode requested by the client (see [StartRequest]).
func (c *Control) Start(_ gorums.ServerContext, req *StartRequest) (*StartResponse, error) {
	c.Reset(req.GetStatsMode())
	return &StartResponse{}, nil
}

// Stop ends the benchmark and returns a Result. For server-measured benchmarks
// (e.g. Multicast) the Result carries latency samples; for client-measured
// benchmarks (e.g. QuorumCall) it carries only memory stats derived from the
// op counter.
func (c *Control) Stop(_ gorums.ServerContext, _ *StopRequest) (*Result, error) {
	c.stats.End()
	n := c.ops.Load()
	r := c.stats.GetResult()
	if r.GetTotalOps() == 0 && n > 0 {
		mallocs, totalAlloc := c.stats.MemDelta()
		r.SetTotalOps(n)
		r.SetAllocsPerOp(mallocs / n)
		r.SetMemPerOp(totalAlloc / n)
	}
	return r, nil
}

// Done records that the sender has finished its own benchmark work and will
// issue no further calls. It is advisory only: a missing or duplicate signal
// never blocks or errors anything here, it only means the caller waiting on
// DoneCh falls back to its own timeout. See benchmark.AwaitPeersDoneOrGrace.
func (c *Control) Done(_ gorums.ServerContext, req *DoneRequest) {
	id := req.GetSenderId()
	if c.doneCh == nil || id == 0 || int(id) >= len(c.doneSeen) {
		return
	}
	if c.doneSeen[id].Swap(true) {
		return // duplicate signal, already counted
	}
	if c.doneLeft.Add(-1) == 0 {
		close(c.doneCh)
	}
}
