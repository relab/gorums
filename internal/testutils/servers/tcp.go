//go:build integration

package servers

import (
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Start starts numServers servers using real TCP listeners and returns their
// addresses and a variadic stop function.
func Start(t testing.TB, numServers int, srvFn func(i int) ServerIface) ([]string, func(...int)) {
	t.Helper()

	listenFn := func(_ int) net.Listener {
		lis, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("Failed to listen on port: %v", err)
		}
		return lis
	}
	return startServers(t, numServers, srvFn, listenFn)
}

// ListenFunc returns a function that creates a TCP listener on a random localhost
// port.
func ListenFunc(_ testing.TB) func() (net.Listener, error) {
	return func() (net.Listener, error) { return net.Listen("tcp", "127.0.0.1:0") }
}

// DialOptions returns insecure TCP transport credentials for connecting to
// servers started by [Start].
func DialOptions(_ testing.TB) []grpc.DialOption {
	return []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}
}
