// Package servers provides the low-level, gorums-independent machinery for
// starting test servers and dialing them. It has no dependency on the gorums
// package, so it can be imported both by gorums's own white-box tests and by
// its public test-helper functions without an import cycle.
package servers

import (
	"net"
	"sync"
	"testing"
)

// ServerIface is the interface a server must implement to be started by
// [Start].
type ServerIface interface {
	Serve(net.Listener) error
	Stop()
}

// serverState tracks one running server so its listener and server can be
// stopped exactly once, and so stop can wait for the accept loop to exit.
type serverState struct {
	srv     ServerIface
	lis     net.Listener
	stopped chan struct{}
}

func (s *serverState) start(_ testing.TB) {
	_ = s.srv.Serve(s.lis)
	// Close rather than send: Serve can return before stop is called, and a
	// send would then block this goroutine forever with no receiver coming.
	close(s.stopped)
}

func (s *serverState) stop(t testing.TB) {
	t.Helper()
	if err := s.lis.Close(); err != nil {
		t.Errorf("Failed to close listener: %v", err)
	}
	s.srv.Stop()
	<-s.stopped
}

// setupServers starts numServers servers via srvFn on listeners obtained from
// listenFn, and returns their addresses and a variadic stop function. The
// stop function stops the servers at the given indices, or all servers if
// called with no arguments.
func setupServers(t testing.TB, numServers int, srvFn func(i int) ServerIface, listenFn func(i int) net.Listener) ([]string, func(...int)) {
	t.Helper()

	addrs := make([]string, numServers)
	muActive := &sync.Mutex{}
	active := make(map[int]*serverState)

	for i := range numServers {
		lis := listenFn(i)
		addrs[i] = lis.Addr().String()
		state := &serverState{srv: srvFn(i), lis: lis, stopped: make(chan struct{})}
		muActive.Lock()
		active[i] = state
		muActive.Unlock()

		go state.start(t)
	}

	stopNodesFn := func(indices ...int) {
		if len(indices) == 0 {
			// Stop all active servers.
			indices = make([]int, numServers)
			for i := range indices {
				indices[i] = i
			}
		}
		toStop := make([]*serverState, 0, len(indices))
		muActive.Lock()
		for _, idx := range indices {
			if state, ok := active[idx]; ok {
				delete(active, idx)
				toStop = append(toStop, state)
			}
		}
		muActive.Unlock()

		for _, state := range toStop {
			state.stop(t)
		}
	}
	return addrs, stopNodesFn
}
