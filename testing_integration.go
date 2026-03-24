//go:build integration

package gorums

import (
	"net"
	"testing"
)

// testSetupServers is the real network implementation of server setup.
// It starts servers using TCP listeners and returns addresses and a variadic stop function.
func testSetupServers(t testing.TB, numServers int, srvFn func(i int) ServerIface) ([]string, func(...int)) {
	t.Helper()

	listenFn := func(i int) net.Listener {
		lis, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("Failed to listen on port: %v", err)
		}
		return lis
	}
	return setupServers(t, numServers, srvFn, listenFn)
}

// TestDialOptions returns a [DialOption] with insecure TCP credentials for integration tests.
func TestDialOptions(t testing.TB) DialOption {
	return InsecureDialOptions(t)
}
