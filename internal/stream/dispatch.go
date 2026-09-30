package stream

import (
	"context"
	"sync"
)

// defaultRequestDispatchSize is how many requests a stream reader can enqueue
// before it waits. It matches the send queue's default capacity. The reader
// blocks only when this queue is full.
const defaultRequestDispatchSize = 4096

// requestDispatch is a per-stream FIFO of request handlers. The reader
// enqueues and keeps reading replies. One goroutine starts the next request
// only after the previous handler calls release or returns.
type requestDispatch struct {
	queue chan func(release func())
}

func newRequestDispatch(size int) *requestDispatch {
	if size <= 0 {
		size = defaultRequestDispatchSize
	}
	return &requestDispatch{queue: make(chan func(release func()), size)}
}

// run starts each queued request in its own goroutine and waits for its
// release before starting the next. It returns when ctx is cancelled.
func (d *requestDispatch) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case run, ok := <-d.queue:
			if !ok {
				return
			}
			released := make(chan struct{})
			var once sync.Once
			release := func() { once.Do(func() { close(released) }) }
			// Run the handler separately so a release that arrives before the
			// handler returns admits the next request.
			go run(release)
			select {
			case <-released:
			case <-ctx.Done():
				return
			}
		}
	}
}

// enqueue adds run to the queue. It waits only when the queue is full, and
// it returns without enqueueing when ctx is cancelled.
func (d *requestDispatch) enqueue(ctx context.Context, run func(release func())) {
	select {
	case <-ctx.Done():
		return
	case d.queue <- run:
	}
}
