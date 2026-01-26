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

// getOrCreateManager returns the existing manager or creates a new one with real network dialing.
// If a new manager is created, its cleanup is registered via t.Cleanup.
func (to *testOptions) getOrCreateManager(t testing.TB) *Manager[uint32] {
	if to.existingMgr != nil {
		// Don't register cleanup - caller is responsible for closing the manager
		return to.existingMgr
	}
	// Create manager and register its cleanup LAST so it runs FIRST (LIFO)
	mgrOpts := append([]ManagerOption{InsecureDialOptions(t)}, to.managerOpts...)
	mgr := NewManager[uint32](mgrOpts...)
	t.Cleanup(Closer(t, mgr))
	return mgr
}
