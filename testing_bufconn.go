//go:build !integration

package gorums

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1024 * 1024

// bufconnRegistry manages bufconn listeners and dialers for tests.
// It provides unique address generation and allows tests to register
// additional listeners after the initial setup.
type bufconnRegistry struct {
	mu          sync.Mutex
	dialers     map[testing.TB]bufconnDialer
	portCounter atomic.Uint32
}

// bufconnDialer is a function that dials a bufconn address.
type bufconnDialer func(ctx context.Context, addr string) (net.Conn, error)

// globalBufconnRegistry is the singleton registry for all tests.
var globalBufconnRegistry = &bufconnRegistry{
	dialers: make(map[testing.TB]bufconnDialer),
}

// nextAddress generates a unique fake address for a bufconn listener.
func (r *bufconnRegistry) nextAddress() string {
	port := r.portCounter.Add(1)
	return fmt.Sprintf("127.0.0.1:%d", 10000+port)
}

// registerDialer adds or chains a new dialer for the given test.
// If a dialer already exists for this test, the new dialer is chained
// to fall back to the existing one for addresses it doesn't know about.
// Returns true if this is the first dialer registered for this test.
func (r *bufconnRegistry) registerDialer(t testing.TB, dialer bufconnDialer) (isFirst bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	existingDialer, exists := r.dialers[t]
	if exists {
		// Chain the new dialer to fall back to the existing one
		r.dialers[t] = func(ctx context.Context, addr string) (net.Conn, error) {
			conn, err := dialer(ctx, addr)
			if err == nil {
				return conn, nil
			}
			return existingDialer(ctx, addr)
		}
		return false
	}
	r.dialers[t] = dialer
	return true
}

// getDialer returns the dialer for the given test, or an error if no dialer is registered.
func (r *bufconnRegistry) getDialer(t testing.TB) (bufconnDialer, error) {
	r.mu.Lock()
	dialer, ok := r.dialers[t]
	r.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("bufconn dialer not found for test")
	}
	return dialer, nil
}

// cleanup removes the dialer for the given test.
func (r *bufconnRegistry) cleanup(t testing.TB) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.dialers, t)
}

// testSetupServers is the bufconn implementation of server setup.
// It starts servers using bufconn and returns addresses and a variadic stop function.
func testSetupServers(t testing.TB, numServers int, srvFn func(int) ServerIface) ([]string, func(...int)) {
	t.Helper()

	addrToListener := make(map[string]*bufconn.Listener, numServers)

	listenFn := func(_ int) net.Listener {
		lis := bufconn.Listen(bufSize)
		addr := globalBufconnRegistry.nextAddress()
		addrToListener[addr] = lis
		return &bufconnListener{
			Listener: lis,
			addr:     bufconnAddr{network: "bufconn", addr: addr},
		}
	}

	// Create a dialer for the new listeners
	newDialer := func(ctx context.Context, addr string) (net.Conn, error) {
		if listener, ok := addrToListener[addr]; ok {
			return listener.DialContext(ctx)
		}
		return nil, fmt.Errorf("no bufconn listener for address: %s", addr)
	}

	// Register or chain the dialer for this test
	isFirst := globalBufconnRegistry.registerDialer(t, newDialer)
	if isFirst {
		t.Cleanup(func() { globalBufconnRegistry.cleanup(t) })
	}

	return setupServers(t, numServers, srvFn, listenFn)
}

// getOrCreateManager returns the existing manager or creates a new one with bufconn dialing.
// If a new manager is created, its cleanup is registered via t.Cleanup.
func (to *testOptions) getOrCreateManager(t testing.TB) *Manager {
	if to.existingMgr != nil {
		// Don't register cleanup - caller is responsible for closing the manager
		return to.existingMgr
	}

	// Create an indirect dialer that looks up from the registry at dial time.
	// This allows the manager to dial addresses that are registered after manager creation.
	bufconnDialer := func(ctx context.Context, addr string) (net.Conn, error) {
		dialer, err := globalBufconnRegistry.getDialer(t)
		if err != nil {
			return nil, err
		}
		return dialer(ctx, addr)
	}

	// Create manager with bufconn dialer and register its cleanup LAST so it runs FIRST (LIFO)
	dialOpts := []grpc.DialOption{
		grpc.WithContextDialer(bufconnDialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}
	mgrOpts := append([]ManagerOption{WithDialOptions(dialOpts...)}, to.managerOpts...)
	mgr := NewManager(mgrOpts...)
	t.Cleanup(func() { Closer(t, mgr)() })
	return mgr
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
