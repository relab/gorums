package gorums

import (
	"net"
	"sync"
	"testing"
)

// ServerIface is the interface that must be implemented by a server in order to support the TestSetup function.
type ServerIface interface {
	Serve(net.Listener) error
	Stop()
}

// TestSetup starts numServers gRPC servers using the given registration
// function, and returns the server addresses along with a stop function
// that should be called to shut down the test. The stop function will block
// until all servers have stopped.
// This function can be used by other packages for testing purposes.
func TestSetup(t testing.TB, numServers int, srvFn func(i int) ServerIface) ([]string, func()) {
	t.Helper()
	servers := make([]ServerIface, numServers)
	listeners := make([]net.Listener, numServers)
	addrs := make([]string, numServers)
	srvStopped := make(chan struct{}, numServers)
	for i := range numServers {
		srv := srvFn(i)
		// listen on any available port
		lis, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("Failed to listen on port: %v", err)
		}
		listeners[i] = lis
		addrs[i] = lis.Addr().String()
		servers[i] = srv
		go func() {
			_ = srv.Serve(lis) // blocks until the server is stopped
			srvStopped <- struct{}{}
		}()
	}
	stopFn := sync.OnceFunc(func() {
		for i := range numServers {
			listeners[i].Close()
			servers[i].Stop()
		}
		// wait for all servers to stop
		for range numServers {
			<-srvStopped
		}
	})
	return addrs, stopFn
}
