package stream

import (
	"context"
	"sync"
	"time"
)

// minSweepSize is the table size at which add first removes expired calls.
const minSweepSize = 64

// pendingCalls holds the two-way calls sent on one stream that await responses.
// A call whose context has ended is removed once the table has doubled in size
// since the last sweep, and at once while the table watches for expiry.
type pendingCalls struct {
	mu       sync.Mutex
	calls    map[uint64]pendingCall
	closed   bool
	seq      uint64 // identifies each added call
	sweepAt  int
	onExpire func() // set by watchExpiry; nil until then
}

// pendingCall is a call in a [pendingCalls] table.
type pendingCall struct {
	req  Request
	seq  uint64
	stop func() bool // stops the expiry watch; nil if there is none
}

// add records req under msgID and stamps its send time. It reports false,
// without recording req, once the table is drained.
func (p *pendingCalls) add(msgID uint64, req Request) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return false
	}
	if p.calls == nil {
		p.calls = make(map[uint64]pendingCall)
	}
	if len(p.calls) >= max(p.sweepAt, minSweepSize) {
		p.sweepLocked()
	}
	req.SendTime = time.Now()
	p.seq++
	call := pendingCall{req: req, seq: p.seq}
	if p.onExpire != nil {
		call.stop = p.watchLocked(msgID, call)
	}
	p.calls[msgID] = call
	return true
}

// take returns the call for msgID and reports whether there was one. A
// non-streaming call is removed; a streaming call stays for later responses.
func (p *pendingCalls) take(msgID uint64) (Request, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	call, ok := p.calls[msgID]
	if ok && !call.req.Streaming {
		p.deleteLocked(msgID, call)
	}
	return call.req, ok
}

// drain removes and returns all calls and makes later adds fail.
func (p *pendingCalls) drain() []Request {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	reqs := make([]Request, 0, len(p.calls))
	for msgID, call := range p.calls {
		reqs = append(reqs, call.req)
		p.deleteLocked(msgID, call)
	}
	return reqs
}

// len returns the number of calls.
func (p *pendingCalls) len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.calls)
}

// watchExpiry makes the table remove each call, present or later added, as
// soon as its context ends, and call onExpire after each such removal.
// It is idempotent.
func (p *pendingCalls) watchExpiry(onExpire func()) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.onExpire != nil {
		return
	}
	p.onExpire = onExpire
	for msgID, call := range p.calls {
		call.stop = p.watchLocked(msgID, call)
		p.calls[msgID] = call
	}
}

// watchLocked removes call, stored under msgID, when its context ends.
func (p *pendingCalls) watchLocked(msgID uint64, call pendingCall) func() bool {
	return context.AfterFunc(call.req.Ctx, func() {
		p.mu.Lock()
		current, ok := p.calls[msgID]
		removed := ok && current.seq == call.seq
		if removed {
			delete(p.calls, msgID)
		}
		onExpire := p.onExpire
		p.mu.Unlock()
		if removed {
			onExpire()
		}
	})
}

// sweepLocked removes the calls whose context has ended and sets the size at
// which the next sweep runs.
func (p *pendingCalls) sweepLocked() {
	for msgID, call := range p.calls {
		if call.req.Ctx.Err() != nil {
			p.deleteLocked(msgID, call)
		}
	}
	p.sweepAt = 2 * len(p.calls)
}

// deleteLocked removes call, stored under msgID, and stops its expiry watch.
func (p *pendingCalls) deleteLocked(msgID uint64, call pendingCall) {
	delete(p.calls, msgID)
	if call.stop != nil {
		call.stop()
	}
}
