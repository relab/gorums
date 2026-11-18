package stream

import (
	"context"
	"sync"
)

// dispatcher runs request handlers one at a time, in push order. The next
// handler starts when the running one calls its release function or returns,
// whichever comes first. A handler that returns without releasing hands its
// goroutine to the next queued handler; a release before return starts the
// next handler on a new goroutine.
type dispatcher struct {
	done  <-chan struct{} // stops the dispatcher; nil for one that never stops
	slots chan struct{}   // one token per queued handler, bounding the queue

	mu      sync.Mutex
	queue   []func(release func())
	running bool
	sealed  bool          // no more handlers are pushed; queued handlers run even after done closes
	active  int           // queued and running handlers
	idle    chan struct{} // closed when active drops to zero; nil until wait needs it
	changed chan struct{} // closed when running becomes false or on seal; nil until waitRetired needs it
}

// newDispatcher returns a dispatcher that queues up to size handlers; size
// must be positive. Closing done stops it: queued handlers are discarded,
// unless the dispatcher is sealed, and pushes fail.
func newDispatcher(done <-chan struct{}, size uint) *dispatcher {
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
	d.active++
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

// start runs run in a new goroutine. While each handler returns without
// releasing, the same goroutine runs the next queued handler, so a busy stream
// keeps one goroutine and the stack it has grown.
func (d *dispatcher) start(run func(release func())) {
	go func() {
		for run != nil {
			var once sync.Once
			run(func() { once.Do(d.next) })
			d.finished()
			run = nil
			once.Do(func() { run = d.take() })
		}
	}()
}

// next starts the oldest queued handler, if any, in a new goroutine.
func (d *dispatcher) next() {
	if run := d.take(); run != nil {
		d.start(run)
	}
}

// take removes and returns the oldest queued handler, or nil if the queue is
// empty or the dispatcher has stopped, in which case it discards the queue and
// marks the dispatcher idle. A sealed dispatcher keeps its queue.
func (d *dispatcher) take() func(release func()) {
	d.mu.Lock()
	if d.stopped() && !d.sealed {
		for range d.queue {
			<-d.slots
		}
		d.finishedLocked(len(d.queue))
		d.queue = nil
	}
	if len(d.queue) == 0 {
		d.running = false
		d.notifyLocked()
		d.mu.Unlock()
		return nil
	}
	run := d.queue[0]
	d.queue[0] = nil
	d.queue = d.queue[1:]
	d.mu.Unlock()
	<-d.slots
	return run
}

// seal records that no more handlers will be pushed. The queued handlers then
// run even if the dispatcher stops.
func (d *dispatcher) seal() {
	d.mu.Lock()
	d.sealed = true
	d.notifyLocked()
	d.mu.Unlock()
}

// hold makes the dispatcher queue handlers until gate closes, and then start
// them in push order. If the dispatcher has stopped by then, it discards them,
// unless it is sealed. Call hold before the first push.
func (d *dispatcher) hold(gate <-chan struct{}) {
	d.mu.Lock()
	d.running = true
	d.mu.Unlock()
	go func() {
		<-gate
		d.next()
	}()
}

// waitRetired waits until the dispatcher will start no more handlers: it has
// stopped or is sealed, and no handler holds the order. A sealed dispatcher
// has then started its queued handlers, and the latest one has returned or
// released.
func (d *dispatcher) waitRetired() {
	done := d.done
	for {
		d.mu.Lock()
		if !d.running && (d.sealed || d.stopped()) {
			d.mu.Unlock()
			return
		}
		if d.changed == nil {
			d.changed = make(chan struct{})
		}
		changed := d.changed
		d.mu.Unlock()
		select {
		case <-changed:
		case <-done:
			done = nil // stopped; wait for the running handler
		}
	}
}

// notifyLocked wakes the goroutines in waitRetired. The caller must hold mu.
func (d *dispatcher) notifyLocked() {
	if d.changed != nil {
		close(d.changed)
		d.changed = nil
	}
}

// wait waits until no handler is queued or running, or until stop is closed,
// and reports whether the dispatcher became idle. A nil stop never closes.
// Handlers that a stopped dispatcher discards count as done.
func (d *dispatcher) wait(stop <-chan struct{}) bool {
	d.mu.Lock()
	if d.active == 0 {
		d.mu.Unlock()
		return true
	}
	if d.idle == nil {
		d.idle = make(chan struct{})
	}
	idle := d.idle
	d.mu.Unlock()
	select {
	case <-idle:
		return true
	case <-stop:
		return false
	}
}

// finished records that a handler returned.
func (d *dispatcher) finished() {
	d.mu.Lock()
	d.finishedLocked(1)
	d.mu.Unlock()
}

// finishedLocked records that n handlers returned or were discarded, and
// wakes the waiters once none is queued or running. The caller must hold mu.
func (d *dispatcher) finishedLocked(n int) {
	d.active -= n
	if d.active == 0 && d.idle != nil {
		close(d.idle)
		d.idle = nil
	}
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
