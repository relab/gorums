package stream

import (
	"sync"
	"time"
)

// pendingCalls holds the two-way calls sent on one stream that await responses.
type pendingCalls struct {
	mu     sync.Mutex
	calls  map[uint64]Request
	closed bool
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
		p.calls = make(map[uint64]Request)
	}
	req.SendTime = time.Now()
	p.calls[msgID] = req
	return true
}

// take returns the call for msgID and reports whether there was one. A
// non-streaming call is removed; a streaming call stays for later responses.
func (p *pendingCalls) take(msgID uint64) (Request, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	req, ok := p.calls[msgID]
	if ok && !req.Streaming {
		delete(p.calls, msgID)
	}
	return req, ok
}

// drain removes and returns all calls and makes later adds fail.
func (p *pendingCalls) drain() []Request {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	reqs := make([]Request, 0, len(p.calls))
	for _, req := range p.calls {
		reqs = append(reqs, req)
	}
	p.calls = nil
	return reqs
}

// len returns the number of calls.
func (p *pendingCalls) len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.calls)
}
