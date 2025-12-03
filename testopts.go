package gorums

import "testing"

// TestOption is a marker interface that can hold ManagerOption,
// ServerOption, or NodeListOption. This allows test helpers to accept
// a single variadic parameter that can be filtered and passed to the
// appropriate constructors (NewManager, NewServer, NewConfiguration).
//
// Each option type (ManagerOption, ServerOption, NodeListOption) embeds
// this interface, so they can be passed directly without wrapping:
//
//	SetupConfiguration(t, 3, nil,
//		WithBackoff(...),           // ManagerOption
//		WithReceiveBufferSize(10),  // ServerOption
//		WithNodeMap(...),           // NodeListOption
//	)
type TestOption any

// testOptions holds extracted options from a slice of TestOption.
type testOptions struct {
	managerOpts  []ManagerOption
	serverOpts   []ServerOption
	nodeListOpts []NodeListOption
	existingMgr  *Manager
}

// hasManager returns true if an existing manager was provided.
func (to *testOptions) hasManager() bool {
	return to.existingMgr != nil
}

// serverFunc returns a server creation function based on the server options.
// If no server options are provided, it returns nil (use default server).
func (to *testOptions) serverFunc(srvFn func(i int) ServerIface) func(i int) ServerIface {
	if srvFn == nil && len(to.serverOpts) > 0 {
		return func(_ int) ServerIface {
			return NewServer(to.serverOpts...)
		}
	}
	return srvFn
}

// preConnectHook extracts and returns the preConnect hook from manager options, if set.
func (to *testOptions) preConnectHook() func(stopServers func()) {
	var tempOpts managerOptions
	for _, opt := range to.managerOpts {
		opt(&tempOpts)
	}
	return tempOpts.preConnect
}

// getOrCreateManager returns the existing manager or creates a new one.
// If a new manager is created, its cleanup is registered via t.Cleanup.
func (to *testOptions) getOrCreateManager(t testing.TB) *Manager {
	if to.existingMgr != nil {
		// Don't register cleanup - caller is responsible for closing the manager
		return to.existingMgr
	}
	// Create manager and register its cleanup LAST so it runs FIRST (LIFO)
	mgrOpts := append([]ManagerOption{InsecureGrpcDialOptions(t)}, to.managerOpts...)
	mgr := NewManager(mgrOpts...)
	t.Cleanup(mgr.Close)
	return mgr
}

// nodeListOption returns the appropriate NodeListOption for the configuration.
// It uses provided options if available, or generates defaults based on whether
// an existing manager is being reused.
func (to *testOptions) nodeListOption(addrs []string) NodeListOption {
	if len(to.nodeListOpts) > 0 {
		// Use the last provided NodeListOption (allows overriding)
		return to.nodeListOpts[len(to.nodeListOpts)-1]
	}
	if to.existingMgr != nil {
		// When reusing a manager, use WithNodeList to auto-generate unique IDs
		// based on addresses, avoiding conflicts with existing nodes
		return WithNodeList(addrs)
	}
	// Default for new manager: build nodeMap with sequential IDs (0, 1, 2, ...)
	nodeMap := make(map[string]uint32, len(addrs))
	for i, addr := range addrs {
		nodeMap[addr] = uint32(i)
	}
	return WithNodeMap(nodeMap)
}

// extractTestOptions separates a slice of TestOption into their specific types.
func extractTestOptions(opts []TestOption) testOptions {
	var result testOptions
	for _, opt := range opts {
		switch o := opt.(type) {
		case ManagerOption:
			result.managerOpts = append(result.managerOpts, o)
		case ServerOption:
			result.serverOpts = append(result.serverOpts, o)
		case NodeListOption:
			result.nodeListOpts = append(result.nodeListOpts, o)
		case managerProvider:
			result.existingMgr = o.mgr
		}
	}
	return result
}

// managerProvider is a TestOption that provides an existing manager.
type managerProvider struct {
	mgr *Manager
}

// WithManager returns a TestOption that provides an existing manager to use
// instead of creating a new one. This is useful when creating multiple
// configurations that should share the same manager.
//
// When using WithManager, the caller is responsible for closing the manager.
// SetupConfiguration will NOT register a cleanup function for the manager.
//
// This option is intended for testing purposes only.
func WithManager(_ testing.TB, mgr *Manager) TestOption {
	if mgr == nil {
		panic("gorums: WithManager called with nil manager")
	}
	return managerProvider{mgr: mgr}
}
