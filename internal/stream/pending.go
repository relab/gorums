package stream

import (
	"maps"
	"slices"
	"sync"
	"time"
)

// minSweepSize is the table size at which add first removes expired calls.
const minSweepSize = 64

// pendingCalls holds the two-way calls sent on one stream that await
// responses. Calls whose contexts have ended are removed when [pendingCalls.add]
// finds that the table has doubled in size since its last sweep and holds at
// least [minSweepSize] calls. A call whose context ended after the last sweep
// stays in the table until the next sweep or until the stream ends.
type pendingCalls struct {
	mu      sync.Mutex
	calls   map[uint64]NodeRequest
	closed  bool
	sweepAt int
}

// add records req under msgID and stamps its send time. It reports false,
// without recording req, once the table is drained.
func (p *pendingCalls) add(msgID uint64, req NodeRequest) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return false
	}
	if p.calls == nil {
		p.calls = make(map[uint64]NodeRequest)
	}
	if len(p.calls) >= max(p.sweepAt, minSweepSize) {
		p.sweepLocked()
	}
	req.sendTime = time.Now()
	p.calls[msgID] = req
	return true
}

// take returns the call for msgID and reports whether there was one. A
// non-streaming call is removed; a streaming call stays for later responses.
func (p *pendingCalls) take(msgID uint64) (NodeRequest, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	req, ok := p.calls[msgID]
	if ok && !req.Streaming {
		delete(p.calls, msgID)
	}
	return req, ok
}

// drain removes and returns all calls and makes later adds fail.
func (p *pendingCalls) drain() []NodeRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	reqs := slices.Collect(maps.Values(p.calls))
	clear(p.calls)
	return reqs
}

// len returns the number of calls.
func (p *pendingCalls) len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.calls)
}

// sweepLocked removes the calls whose context has ended and sets the size at
// which the next sweep runs.
func (p *pendingCalls) sweepLocked() {
	maps.DeleteFunc(p.calls, func(_ uint64, req NodeRequest) bool {
		return req.Ctx.Err() != nil
	})
	p.sweepAt = 2 * len(p.calls)
}
