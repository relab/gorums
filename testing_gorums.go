package gorums

import (
	"net"
	"testing"
)

type server interface {
	Serve(net.Listener) error
	Stop()
}

// Package testing provide a public API for setting up Gorums.
// This package can be used by other packages, such as Raft and HotStuff.

// TestSetup starts numServers gRPC servers using the given registration
// function, and returns the server addresses along with a stop function
// that should be called to shut down the test.
func TestSetup(t testing.TB, numServers int, srvFunc func() interface{}) ([]string, func()) {
	t.Helper()
	servers := make([]server, numServers)
	addrs := make([]string, numServers)
	for i := 0; i < numServers; i++ {
		var srv server
		var ok bool
		if srv, ok = srvFunc().(server); !ok {
			t.Fatal("Incompatible server type. You should use a GorumsServer or grpc.Server")
		}
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
