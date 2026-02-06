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
	managerOpts    []ManagerOption
	serverOpts     []ServerOption
	nodeListOpts   []NodeListOption[uint32]
	existingMgr    *Manager[uint32]
	stopFuncPtr    *func(...int)       // pointer to capture the variadic stop function
	preConnectHook func(stopFn func()) // called before connecting to servers
	skipGoleak     bool                // skip goleak checks (useful for synctest)
}

// shouldSkipGoleak returns true if goleak checks should be skipped.
// This includes cases where an existing manager is reused (since it may
// already have its own goleak checks) or when SkipGoleak option is set.
func (to *testOptions) shouldSkipGoleak() bool {
	return to.existingMgr != nil || to.skipGoleak
}

// serverFunc returns a server creation function based on the server options.
// If srvFn is nil, it returns a default server function that creates servers
// with the provided server options and registers default handlers.
// If srvFn is not nil and server options are provided, it panics since
// options cannot be applied to a custom server function.
func (to *testOptions) serverFunc(srvFn func(i int) ServerIface) func(i int) ServerIface {
	if srvFn == nil {
		// Use default server, potentially with custom options
		return func(i int) ServerIface {
			return defaultTestServer(i, to.serverOpts...)
		}
	}
	if len(to.serverOpts) > 0 {
		// You need to pass nil as the server function to use server options with the default server
		panic("gorums: cannot use server options with a custom server function; use nil to use the default test server")
	}
	return srvFn
}

// nodeListOption returns the appropriate NodeListOption for the configuration.
// It uses provided options if available, or generates defaults based on whether
// an existing manager is being reused.
func (to *testOptions) nodeListOption(addrs []string) NodeListOption[uint32] {
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
		case NodeListOption[uint32]:
			result.nodeListOpts = append(result.nodeListOpts, o)
		case *Manager[uint32]:
			result.existingMgr = o
		case stopFuncProvider:
			result.stopFuncPtr = o.stopFunc
		case preConnectProvider:
			result.preConnectHook = o.hook
		case skipGoleakProvider:
			result.skipGoleak = true
		}
	}
	return result
}

// WithManager returns a TestOption that provides an existing manager to use
// instead of creating a new one. This is useful when creating multiple
// configurations that should share the same manager.
//
// When using WithManager, the caller is responsible for closing the manager.
// SetupConfiguration will NOT register a cleanup function for the manager.
//
// This option is intended for testing purposes only.
func WithManager(_ testing.TB, mgr *Manager[uint32]) TestOption {
	if mgr == nil {
		panic("gorums: WithManager called with nil manager")
	}
	return mgr
}

// stopFuncProvider is a TestOption that captures the server stop function.
type stopFuncProvider struct {
	stopFunc *func(...int)
}

// WithStopFunc returns a TestOption that captures the variadic server stop function,
// allowing tests to stop servers at any point during test execution.
// Call with no arguments to stop all servers, or with specific indices to stop those servers.
// This is useful for testing server failure scenarios.
//
// Usage:
//
//	var stopServers func(...int)
//	config := gorums.TestConfiguration(t, 3, nil, gorums.WithStopFunc(t, &stopServers))
//	// ... send some messages ...
//	stopServers() // stop all servers
//	// OR
//	stopServers(0, 2) // stop servers at indices 0 and 2
//	// ... verify error handling ...
//
// This option is intended for testing purposes only.
func WithStopFunc(_ testing.TB, fn *func(...int)) TestOption {
	if fn == nil {
		panic("gorums: WithStopFunc called with nil pointer")
	}
	return stopFuncProvider{stopFunc: fn}
}

// preConnectProvider is a TestOption that registers a pre-connect hook.
type preConnectProvider struct {
	hook func(stopFn func())
}

// WithPreConnect returns a TestOption that registers a function to be called
// after servers are started but before nodes attempt to connect. The function
// receives a stopServers callback that can be used to stop the test servers.
//
// This is useful for testing error handling when servers are unavailable:
//
//	node := gorums.TestNode(t, nil, gorums.WithPreConnect(t, func(stopServers func()) {
//		stopServers()
//		time.Sleep(300 * time.Millisecond) // wait for server to fully stop
//	}))
//
// This option is intended for testing purposes only.
func WithPreConnect(_ testing.TB, fn func(stopServers func())) TestOption {
	if fn == nil {
		panic("gorums: WithPreConnect called with nil function")
	}
	return preConnectProvider{hook: fn}
}

// skipGoleakProvider is a TestOption that disables goleak checks.
type skipGoleakProvider struct{}

// SkipGoleak returns a TestOption that disables goleak checks for the test.
// This is useful when using synctest, which creates goroutines that goleak
// cannot properly track.
//
// Usage:
//
//	config := gorums.TestConfiguration(t, 3, nil, gorums.SkipGoleak())
//
// This option is intended for testing purposes only.
func SkipGoleak() TestOption {
	return skipGoleakProvider{}
}
