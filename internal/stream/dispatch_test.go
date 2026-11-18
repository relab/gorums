package stream

import (
	"context"
	"sync"
	"testing"
	"time"
)

// testRecvBufferSize is the dispatch queue size tests use when they do not
// exercise the size itself.
const testRecvBufferSize = 4096

// TestDispatcherOrder verifies that handlers start in push order, each after
// the previous one released or returned.
func TestDispatcherOrder(t *testing.T) {
	d := newDispatcher(nil, testRecvBufferSize)
	var mu sync.Mutex
	var order []int
	var wg sync.WaitGroup
	const n = 50
	wg.Add(n)
	for i := range n {
		ok := d.push(t.Context(), func(release func()) {
			defer wg.Done()
			mu.Lock()
			order = append(order, i)
			mu.Unlock()
			if i%2 == 0 {
				release() // even handlers release early, odd ones on return
			}
		})
		if !ok {
			t.Fatalf("push %d failed", i)
		}
	}
	wg.Wait()
	for i, got := range order {
		if got != i {
			t.Fatalf("order = %v, want 0..%d", order, n-1)
		}
	}
}

// TestDispatcherReleaseAdmitsNext verifies that a release before return starts
// the next handler, and that without a release the next handler waits.
func TestDispatcherReleaseAdmitsNext(t *testing.T) {
	d := newDispatcher(nil, testRecvBufferSize)
	releaseFirst := make(chan func())
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	secondStarted := make(chan struct{})
	d.push(t.Context(), func(release func()) {
		releaseFirst <- release
		<-block
	})
	d.push(t.Context(), func(func()) { close(secondStarted) })

	release := <-releaseFirst
	select {
	case <-secondStarted:
		t.Fatal("second handler started before the first released")
	case <-time.After(20 * time.Millisecond):
	}
	release()
	select {
	case <-secondStarted:
	case <-time.After(defaultTestTimeout):
		t.Fatal("second handler did not start after release")
	}
}

// TestDispatcherGoroutineReuse verifies that a handler which returns without
// releasing hands its goroutine to the next queued handler, so a busy stream
// keeps one goroutine and its grown stack, and that a handler which releases
// early starts the next one on a new goroutine.
func TestDispatcherGoroutineReuse(t *testing.T) {
	tests := []struct {
		name      string
		release   bool // each handler releases before returning
		wantReuse bool
	}{
		{name: "ReturnReusesGoroutine", release: false, wantReuse: true},
		{name: "EarlyReleaseStartsNewGoroutine", release: true, wantReuse: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newDispatcher(nil, testRecvBufferSize)
			const n = 5
			ids := make([]uint64, n)
			queued := make(chan struct{})
			var wg sync.WaitGroup
			wg.Add(n)
			for i := range n {
				ok := d.push(t.Context(), func(release func()) {
					defer wg.Done()
					if i == 0 {
						<-queued // hold the first handler until the rest are queued
					}
					ids[i] = goroutineID()
					if tt.release {
						release()
					}
				})
				if !ok {
					t.Fatalf("push %d failed", i)
				}
			}
			close(queued)
			wg.Wait()
			for i := 1; i < n; i++ {
				if reused := ids[i] == ids[i-1]; reused != tt.wantReuse {
					t.Fatalf("handler %d goroutine %d, handler %d goroutine %d: reused = %v, want %v",
						i-1, ids[i-1], i, ids[i], reused, tt.wantReuse)
				}
			}
		})
	}
}

// TestDispatcherBounded verifies that a full queue makes tryPush fail and push
// wait, and that push ends with its context or the dispatcher's done channel.
func TestDispatcherBounded(t *testing.T) {
	done := make(chan struct{})
	d := newDispatcher(done, 1)
	block := make(chan struct{})
	defer close(block)
	running := make(chan struct{})
	d.push(t.Context(), func(func()) { close(running); <-block }) // running
	<-running
	if !d.tryPush(func(func()) {}) { // queued
		t.Fatal("tryPush failed with space in the queue")
	}
	if d.tryPush(func(func()) {}) {
		t.Fatal("tryPush succeeded on a full queue")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if d.push(ctx, func(func()) {}) {
		t.Fatal("push succeeded on a full queue")
	}

	pushed := make(chan bool)
	go func() { pushed <- d.push(t.Context(), func(func()) {}) }()
	close(done)
	select {
	case ok := <-pushed:
		if ok {
			t.Fatal("push succeeded after the dispatcher stopped")
		}
	case <-time.After(defaultTestTimeout):
		t.Fatal("push did not end when the dispatcher stopped")
	}
}

// TestDispatcherHold verifies that a held dispatcher queues handlers until
// its gate closes and then runs them in push order, and that a dispatcher
// that stops while held discards them.
func TestDispatcherHold(t *testing.T) {
	t.Run("GateOpens", func(t *testing.T) {
		d := newDispatcher(nil, testRecvBufferSize)
		gate := make(chan struct{})
		d.hold(gate)
		ran := make(chan int, 2)
		for i := range 2 {
			if !d.push(t.Context(), func(func()) { ran <- i }) {
				t.Fatalf("push %d failed", i)
			}
		}
		select {
		case i := <-ran:
			t.Fatalf("handler %d ran before the gate opened", i)
		case <-time.After(20 * time.Millisecond):
		}
		close(gate)
		for want := range 2 {
			select {
			case got := <-ran:
				if got != want {
					t.Fatalf("handler %d ran, want %d", got, want)
				}
			case <-time.After(defaultTestTimeout):
				t.Fatalf("handler %d did not run after the gate opened", want)
			}
		}
		if !d.wait(nil) {
			t.Fatal("wait() = false, want true")
		}
	})
	for _, sealed := range []bool{false, true} {
		name := map[bool]string{false: "StoppedDiscards", true: "StoppedSealedRuns"}[sealed]
		t.Run(name, func(t *testing.T) {
			done := make(chan struct{})
			d := newDispatcher(done, testRecvBufferSize)
			gate := make(chan struct{})
			d.hold(gate)
			ran := make(chan struct{}, 1)
			if !d.push(t.Context(), func(func()) { ran <- struct{}{} }) {
				t.Fatal("push failed")
			}
			if sealed {
				d.seal()
			}
			close(done)
			select {
			case <-ran:
				t.Fatal("a held handler ran before the gate opened")
			case <-time.After(20 * time.Millisecond):
			}
			close(gate)
			if !d.wait(nil) {
				t.Fatal("wait() = false, want true")
			}
			select {
			case <-ran:
				if !sealed {
					t.Fatal("a stopped dispatcher ran a held handler")
				}
			default:
				if sealed {
					t.Fatal("a sealed dispatcher discarded a held handler")
				}
			}
		})
	}
}

// TestDispatcherWaitRetired verifies that waitRetired returns once the
// dispatcher will start no more handlers: it has stopped or is sealed, and no
// handler holds the order, because the running one returned or released.
func TestDispatcherWaitRetired(t *testing.T) {
	tests := []struct {
		name string
		end  func(d *dispatcher, done chan struct{}) // makes the dispatcher stop starting handlers
		exit func(release func(), block chan struct{})
	}{
		{name: "StopThenReturn", end: func(_ *dispatcher, done chan struct{}) { close(done) }, exit: func(_ func(), block chan struct{}) { close(block) }},
		{name: "StopThenRelease", end: func(_ *dispatcher, done chan struct{}) { close(done) }, exit: func(release func(), _ chan struct{}) { release() }},
		{name: "SealThenReturn", end: func(d *dispatcher, _ chan struct{}) { d.seal() }, exit: func(_ func(), block chan struct{}) { close(block) }},
		{name: "SealThenRelease", end: func(d *dispatcher, _ chan struct{}) { d.seal() }, exit: func(release func(), _ chan struct{}) { release() }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			done := make(chan struct{})
			d := newDispatcher(done, testRecvBufferSize)
			block := make(chan struct{})
			t.Cleanup(func() {
				select {
				case <-block:
				default:
					close(block)
				}
			})
			releaseCh := make(chan func(), 1)
			d.push(t.Context(), func(release func()) {
				releaseCh <- release
				<-block
			})
			release := <-releaseCh

			retired := make(chan struct{})
			go func() { d.waitRetired(); close(retired) }()
			tt.end(d, done)
			select {
			case <-retired:
				t.Fatal("waitRetired returned while a handler held the order")
			case <-time.After(20 * time.Millisecond):
			}
			tt.exit(release, block)
			select {
			case <-retired:
			case <-time.After(defaultTestTimeout):
				t.Fatal("waitRetired did not return after the handler gave up the order")
			}
		})
	}
	t.Run("SealedRunsQueue", func(t *testing.T) {
		d := newDispatcher(nil, testRecvBufferSize)
		block := make(chan struct{})
		var ran sync.WaitGroup
		ran.Add(2)
		d.push(t.Context(), func(func()) { <-block; ran.Done() })
		d.push(t.Context(), func(func()) { ran.Done() })
		d.seal()
		close(block)
		d.waitRetired()
		ran.Wait() // both handlers started before waitRetired returned
	})
}
