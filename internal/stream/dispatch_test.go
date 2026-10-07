package stream

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestDispatcherOrder verifies that handlers start in push order, each after
// the previous one released or returned.
func TestDispatcherOrder(t *testing.T) {
	d := newDispatcher(nil, 0)
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
	d := newDispatcher(nil, 0)
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
			d := newDispatcher(nil, 0)
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
