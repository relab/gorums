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
	return setupServers(t, numServers, srvFn, listenFn)
}

// DialOptions returns insecure TCP transport credentials for connecting to
// servers started by [Start].
func DialOptions(_ testing.TB) []grpc.DialOption {
	return []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}
}
