//go:build !integration

package gorums

import (
	"context"
	"fmt"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1024 * 1024

// testSetupServers is the bufconn implementation of server setup.
// It starts servers using bufconn and returns addresses and a variadic stop function.
// When called from within synctest.Test, it automatically integrates with synctest.
func testSetupServers(t testing.TB, numServers int, srvFn func(i int) ServerIface) ([]string, func(...int)) {
	t.Helper()

	addrToListener := make(map[string]*bufconn.Listener, numServers)

	listenFn := func(i int) net.Listener {
		lis := bufconn.Listen(bufSize)
		addr := bufconnAddress(i)
		addrToListener[addr] = lis
		return &bufconnListener{
			Listener: lis,
			addr:     bufconnAddr{network: "bufconn", addr: addr},
		}
	}

	// Create and store the dialer for this test
	testBufconnDialers[t] = func(ctx context.Context, addr string) (net.Conn, error) {
		if listener, ok := addrToListener[addr]; ok {
			return listener.DialContext(ctx)
		}
		return nil, fmt.Errorf("no bufconn listener for address: %s", addr)
	}
	t.Cleanup(func() { delete(testBufconnDialers, t) })

	return setupServers(t, numServers, srvFn, listenFn)
}

// Global storage for bufconn dialers per test
var testBufconnDialers = make(map[testing.TB]func(context.Context, string) (net.Conn, error))

// getOrCreateManager returns the existing manager or creates a new one with bufconn dialing.
// If a new manager is created, its cleanup is registered via t.Cleanup.
func (to *testOptions) getOrCreateManager(t testing.TB) *Manager[uint32] {
	if to.existingMgr != nil {
		// Don't register cleanup - caller is responsible for closing the manager
		return to.existingMgr
	}

	// Get the bufconn dialer for this test
	bufconnDialer, ok := testBufconnDialers[t]
	if !ok {
		panic("bufconn dialer not found for test - this is a bug")
	}

	// Create manager with bufconn dialer and register its cleanup LAST so it runs FIRST (LIFO)
	dialOpts := []grpc.DialOption{
		grpc.WithContextDialer(bufconnDialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}
	mgrOpts := append([]ManagerOption{WithDialOptions(dialOpts...)}, to.managerOpts...)
	mgr := NewManager[uint32](mgrOpts...)
	t.Cleanup(func() { Closer(t, mgr)() })
	return mgr
}

// bufconnAddress generates a fake address for bufconn listener at index i.
func bufconnAddress(i int) string {
	return fmt.Sprintf("127.0.0.1:%d", 10000+i)
}

// bufconnListener wraps bufconn.Listener to implement net.Listener
type bufconnListener struct {
	*bufconn.Listener
	addr net.Addr
}

func (bl *bufconnListener) Addr() net.Addr {
	return bl.addr
}

// bufconnAddr implements net.Addr for bufconn
type bufconnAddr struct {
	network string
	addr    string
}

func (ba bufconnAddr) Network() string { return ba.network }
func (ba bufconnAddr) String() string  { return ba.addr }
