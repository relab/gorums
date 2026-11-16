package stream

import (
	"sync"
	"sync/atomic"
)

// sendQueue is a bounded FIFO of requests awaiting a stream send.
// Closing fails queued requests with [ErrNodeClosed] and rejects later admissions.
type sendQueue struct {
	id   ID
	ch   chan NodeRequest
	done <-chan struct{} // ends waits for queue space

	mu     sync.RWMutex // held for reading by push, for writing by close
	closed bool

	dropped atomic.Int64
}

// newSendQueue returns a queue with the given capacity. Closing done ends
// waits for queue space; [sendQueue.close] waits for those to end.
func newSendQueue(id ID, capacity uint, done <-chan struct{}) *sendQueue {
	return &sendQueue{id: id, ch: make(chan NodeRequest, capacity), done: done}
}

// tryPush admits req immediately or returns an error. Rejection is reported
// only to the caller; the request has not entered the asynchronous send path.
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

// push waits for queue space until the request or queue lifetime ends.
// It is used for replies on ordinary client streams.
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

// fail reports a failure of an accepted request, or counts a discarded
// handler reply.
func (q *sendQueue) fail(req NodeRequest, err error) {
	if req.ResponseChan == nil && !req.Oneway {
		q.dropped.Add(1)
		return
	}
	req.sendErrorResponse(q.id, err)
}
