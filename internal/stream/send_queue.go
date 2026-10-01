package stream

import (
	"sync"
	"sync/atomic"
)

// sendQueue is a bounded FIFO of requests awaiting a stream send.
// Once closed, it fails every queued and later request with [ErrNodeClosed].
type sendQueue struct {
	id   uint32
	ch   chan Request
	done <-chan struct{} // ends waits for queue space

	mu     sync.RWMutex // held for reading by push, for writing by close
	closed bool

	dropped atomic.Int64
}

// newSendQueue returns a queue with the given capacity. The done channel ends
// waits for queue space; close it before calling [sendQueue.close].
func newSendQueue(id uint32, capacity uint, done <-chan struct{}) *sendQueue {
	return &sendQueue{id: id, ch: make(chan Request, capacity), done: done}
}

// push adds req to the queue. If the queue is full and wait is false, it
// fails req with [ErrSendQueueFull]. If wait is true, it waits for space until
// req's context or the queue's done channel ends, and fails req with the
// corresponding error. A closed queue fails req with [ErrNodeClosed].
func (q *sendQueue) push(req Request, wait bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	if q.closed {
		q.fail(req, ErrNodeClosed)
		return
	}
	if !wait {
		select {
		case q.ch <- req:
		default:
			q.fail(req, ErrSendQueueFull)
		}
		return
	}
	select {
	case q.ch <- req:
	case <-req.Ctx.Done():
		req.ReplyError(q.id, req.Ctx.Err())
	case <-q.done:
		q.fail(req, ErrNodeClosed)
	}
}

// close closes the queue and fails the requests left in it with [ErrNodeClosed].
// It is idempotent.
func (q *sendQueue) close() {
	q.mu.Lock()
	q.closed = true
	q.mu.Unlock()
	for {
		select {
		case req := <-q.ch:
			q.fail(req, ErrNodeClosed)
		default:
			return
		}
	}
}

// fail replies err to req, or counts req as dropped if it has no response channel.
func (q *sendQueue) fail(req Request, err error) {
	if req.ResponseChan == nil {
		q.dropped.Add(1)
		return
	}
	req.ReplyError(q.id, err)
}
