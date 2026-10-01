package stream

import (
	"context"
	"sync"
)

// defaultRequestDispatchSize is the default number of requests a dispatcher
// queues behind the running handler.
const defaultRequestDispatchSize = 4096

// dispatcher runs request handlers one at a time, in push order. Each handler
// runs in its own goroutine, and the next handler starts when the running one
// calls its release function or returns, whichever comes first.
type dispatcher struct {
	done  <-chan struct{} // stops the dispatcher; nil for one that never stops
	slots chan struct{}   // one token per queued handler, bounding the queue

	mu      sync.Mutex
	queue   []func(release func())
	running bool
}

// newDispatcher returns a dispatcher that queues up to size handlers, or
// [defaultRequestDispatchSize] if size is 0. Closing done stops it: queued
// handlers are discarded and pushes fail.
func newDispatcher(done <-chan struct{}, size uint) *dispatcher {
	if size == 0 {
		size = defaultRequestDispatchSize
	}
	return &dispatcher{done: done, slots: make(chan struct{}, size)}
}

// push queues run, waiting while the queue is full. It reports false, without
// queueing run, if ctx ends or the dispatcher stops first.
func (d *dispatcher) push(ctx context.Context, run func(release func())) bool {
	select {
	case d.slots <- struct{}{}:
	case <-ctx.Done():
		return false
	case <-d.done:
		return false
	}
	return d.admit(run)
}

// tryPush queues run if the queue has space and reports whether it did.
func (d *dispatcher) tryPush(run func(release func())) bool {
	select {
	case d.slots <- struct{}{}:
	default:
		return false
	}
	return d.admit(run)
}

// admit starts run, or queues it behind the running handler, after its slot
// has been taken.
func (d *dispatcher) admit(run func(release func())) bool {
	d.mu.Lock()
	if d.stopped() {
		d.mu.Unlock()
		<-d.slots
		return false
	}
	if d.running {
		d.queue = append(d.queue, run)
		d.mu.Unlock()
		return true
	}
	d.running = true
	d.mu.Unlock()
	<-d.slots
	d.start(run)
	return true
}

// start runs run in a new goroutine and starts the next handler on release.
func (d *dispatcher) start(run func(release func())) {
	var once sync.Once
	release := func() { once.Do(d.next) }
	go func() {
		defer release()
		run(release)
	}()
}

// next starts the oldest queued handler, if any.
func (d *dispatcher) next() {
	d.mu.Lock()
	if d.stopped() {
		for range d.queue {
			<-d.slots
		}
		d.queue = nil
	}
	if len(d.queue) == 0 {
		d.running = false
		d.mu.Unlock()
		return
	}
	run := d.queue[0]
	d.queue[0] = nil
	d.queue = d.queue[1:]
	d.mu.Unlock()
	<-d.slots
	d.start(run)
}

// stopped reports whether done is closed.
func (d *dispatcher) stopped() bool {
	select {
	case <-d.done:
		return true
	default:
		return false
	}
}
