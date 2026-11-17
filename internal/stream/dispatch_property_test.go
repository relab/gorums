package stream

import (
	"slices"
	"sync"
	"testing"
	"testing/synctest"

	"pgregory.net/rapid"
)

func TestDispatcherProperties(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		early := rapid.SliceOfN(rapid.Bool(), 2, 12).Draw(t, "releaseBeforeReturn")
		stopAt := rapid.IntRange(0, len(early)).Draw(t, "stopAt")
		sealed := rapid.Bool().Draw(t, "sealed")
		rapid.SyncTest(t, func(t *rapid.T) {
			done := make(chan struct{})
			stop := sync.OnceFunc(func() { close(done) })
			defer stop()
			d := newDispatcher(done, uint(len(early)-1))
			gates := make([]chan struct{}, len(early))
			returns := make([]chan struct{}, len(early))
			releaseGate := make([]func(), len(early))
			returnHandler := make([]func(), len(early))
			for i := range early {
				gates[i], returns[i] = make(chan struct{}), make(chan struct{})
				releaseGate[i] = sync.OnceFunc(func() { close(gates[i]) })
				returnHandler[i] = sync.OnceFunc(func() { close(returns[i]) })
			}
			defer func() {
				for i := range early {
					releaseGate[i]()
					returnHandler[i]()
				}
			}()
			var started []int
			run := func(i int) func(func()) {
				return func(release func()) {
					started = append(started, i)
					<-gates[i]
					if early[i] {
						release()
						release()
						<-returns[i]
					}
				}
			}
			if !d.tryPush(run(0)) {
				t.Fatal("first push rejected")
			}
			synctest.Wait()
			for i := 1; i < len(early); i++ {
				if !d.tryPush(run(i)) {
					t.Fatalf("queued push %d rejected", i)
				}
			}
			if d.tryPush(func(func()) {}) {
				t.Fatal("full dispatcher accepted a handler")
			}
			waited := make(chan struct{})
			go func() { d.wait(nil); close(waited) }()
			synctest.Wait()
			for i := range early {
				want := make([]int, i+1)
				for j := range want {
					want[j] = j
				}
				if !slices.Equal(started, want) {
					t.Fatalf("handler start order = %v, want %v", started, want)
				}
				select {
				case <-waited:
					t.Fatal("wait returned before handlers finished")
				default:
				}
				if i == stopAt {
					if sealed {
						d.seal()
					}
					stop()
				}
				releaseGate[i]()
				synctest.Wait()
				if i == stopAt && !sealed {
					if !slices.Equal(started, want) {
						t.Fatalf("stopped dispatcher started queued handlers: %v", started)
					}
					break
				}
			}
			for _, finish := range returnHandler {
				finish()
			}
			synctest.Wait()
			select {
			case <-waited:
			default:
				t.Fatal("wait did not finish after drain")
			}
			stop()
			if d.tryPush(func(func()) {}) {
				t.Fatal("stopped dispatcher accepted a handler")
			}
			if !d.wait(nil) {
				t.Fatal("idle dispatcher did not report completion")
			}
		})
	})
}
