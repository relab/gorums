package stream

import (
	"sync"
	"sync/atomic"
)

// sendQueue is a bounded FIFO queue of requests awaiting a stream send.
type sendQueue struct {
	id   ID
	ch   chan NodeRequest
	done <-chan struct{} // releases pushes that wait for queue space

	mu     sync.RWMutex // held for reading by push, for writing by close
	closed bool

	dropped atomic.Int64
}

// newSendQueue returns a queue with the given capacity. A [sendQueue.push] that
// waits for queue space returns when done closes. [sendQueue.close] waits for
// such calls to return, so close done before closing the queue.
func newSendQueue(id ID, capacity uint, done <-chan struct{}) *sendQueue {
	return &sendQueue{id: id, ch: make(chan NodeRequest, capacity), done: done}
}

// tryPush admits req immediately or returns an error: [ErrSendQueueFull] if the
// queue is full, or [ErrNodeClosed] if it is closed.
func (q *sendQueue) tryPush(req NodeRequest) error {
	q.mu.RLock()
	defer q.mu.RUnlock()
	if q.closed {
		return ErrNodeClosed
	}
	select {
	case q.ch <- req:
		return nil
	default:
		return ErrSendQueueFull
	}
}

// push admits req once the queue has space. It returns the request's context
// error if that context ends first, or [ErrNodeClosed] if the queue is closed
// or its done channel closes first.
func (q *sendQueue) push(req NodeRequest) error {
	q.mu.RLock()
	defer q.mu.RUnlock()
	if q.closed {
		return ErrNodeClosed
	}
	select {
	case q.ch <- req:
		return nil
	case <-req.Ctx.Done():
		return req.Ctx.Err()
	case <-q.done:
		return ErrNodeClosed
	}
}

// close closes the queue and reports [ErrNodeClosed] for each request still
// queued. It is idempotent.
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

// fail reports a failure of an accepted request, or counts a discarded
// handler reply.
func (q *sendQueue) fail(req NodeRequest, err error) {
	if req.ResponseChan == nil && !req.Oneway {
		q.dropped.Add(1)
		return
	}
	req.sendErrorResponse(q.id, err)
}
