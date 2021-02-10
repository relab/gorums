package gorums

import (
	"net"
	"testing"
)

type ServerIface interface {
	Serve(net.Listener) error
	Stop()
}

// TestSetup starts numServers gRPC servers using the given registration
// function, and returns the server addresses along with a stop function
// that should be called to shut down the test.
// This function can be used by other packages for testing purposes.
func TestSetup(t testing.TB, numServers int, srvFn func(i int) ServerIface) ([]string, func()) {
	t.Helper()
	servers := make([]ServerIface, numServers)
	addrs := make([]string, numServers)
	for i := 0; i < numServers; i++ {
		srv := srvFn(i)
		// listen on any available port
		lis, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("Failed to listen on port: %v", err)
		}
		addrs[i] = lis.Addr().String()
		servers[i] = srv
		go func() { _ = srv.Serve(lis) }()
	}
	stopFn := func() {
		for _, srv := range servers {
			srv.Stop()
		}
	}
	return addrs, stopFn
}
