package gorumstest

import (
	"testing"

	"github.com/relab/gorums"
)

// Option is a marker interface that can hold a [gorums.DialOption],
// [gorums.ServerOption], or [gorums.NodeSource]. This allows test helpers
// to accept a single variadic parameter that can be filtered and passed to the
// appropriate constructors: [gorums.NewServer] or [gorums.NewConfig].
//
// Each option type (gorums.DialOption, gorums.ServerOption,
// gorums.NodeSource) satisfies this interface already, since it is just an
// alias for any, so they can be passed directly without wrapping:
//
//	gorumstest.Config(t, 3, nil,
//		gorums.WithBackoff(...),        // DialOption
//		gorums.WithBufferSizes(10, 10), // ServerOption
//		gorums.WithNodes(...),          // NodeSource
//	)
type Option any

// testOptions holds extracted options from a slice of Option.
type testOptions struct {
	managerOpts    []gorums.DialOption
	serverOpts     []gorums.ServerOption
	nodeListOpts   []gorums.NodeSource
	stopFuncPtr    *func(...int)       // pointer to capture the variadic stop function
	preConnectHook func(stopFn func()) // called before connecting to servers
	skipGoleak     bool                // skip goleak checks (useful for synctest)
}

// shouldSkipGoleak returns true if goleak checks should be skipped.
func (to *testOptions) shouldSkipGoleak() bool {
	return to.skipGoleak
}

// serverFunc returns a server creation function based on the server options.
// If srvFn is nil, it returns a default server function that creates servers
// with the provided server options and registers default handlers.
// If srvFn is not nil and server options are provided, it panics since
// options cannot be applied to a custom server function.
func (to *testOptions) serverFunc(srvFn func(i int) gorums.ServerIface) func(i int) gorums.ServerIface {
	if srvFn == nil {
		// Use default server, potentially with custom options
		return func(i int) gorums.ServerIface {
			return defaultTestServer(i, to.serverOpts...)
		}
	}
	if len(to.serverOpts) > 0 {
		// You need to pass nil as the server function to use server options with the default server
		panic("gorumstest: cannot use server options with a custom server function")
	}
	return srvFn
}

// nodeListOption returns the appropriate NodeSource for the configuration.
// It uses provided options if available, otherwise defaults to WithNodeList.
func (to *testOptions) nodeListOption(addrs []string) gorums.NodeSource {
	if len(to.nodeListOpts) > 0 {
		// Use the last provided NodeSource (allows overriding)
		return to.nodeListOpts[len(to.nodeListOpts)-1]
	}
	// Default: use WithNodeList which generates unique IDs based on max(manager.NodeIDs()) + 1
	return gorums.WithNodeList(addrs)
}

// extractTestOptions separates a slice of Option into their specific types.
func extractTestOptions(opts []Option) testOptions {
	var result testOptions
	for _, opt := range opts {
		switch o := opt.(type) {
		case gorums.DialOption:
			result.managerOpts = append(result.managerOpts, o)
		case gorums.ServerOption:
			result.serverOpts = append(result.serverOpts, o)
		case gorums.NodeSource:
			result.nodeListOpts = append(result.nodeListOpts, o)
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

// stopFuncProvider is an Option that captures the server stop function.
type stopFuncProvider struct {
	stopFunc *func(...int)
}

// WithStopFunc returns an Option that captures the variadic server stop function,
// allowing tests to stop servers at any point during test execution.
// Call with no arguments to stop all servers, or with specific indices to stop those servers.
// This is useful for testing server failure scenarios.
//
// Usage:
//
//	var stopServers func(...int)
//	config := gorumstest.Config(t, 3, nil, gorumstest.WithStopFunc(t, &stopServers))
//	// ... send some messages ...
//	stopServers() // stop all servers
//	// OR
//	stopServers(0, 2) // stop servers at indices 0 and 2
//	// ... verify error handling ...
//
// This option is intended for testing purposes only.
func WithStopFunc(_ testing.TB, fn *func(...int)) Option {
	if fn == nil {
		panic("gorumstest: WithStopFunc called with nil function pointer")
	}
	return stopFuncProvider{stopFunc: fn}
}

// preConnectProvider is an Option that registers a pre-connect hook.
type preConnectProvider struct {
	hook func(stopFn func())
}

// WithPreConnect returns an Option that registers a function to be called
// after servers are started but before nodes attempt to connect. The function
// receives a stopServers callback that can be used to stop the test servers.
//
// This is useful for testing error handling when servers are unavailable:
//
//	node := gorumstest.Node(t, nil, gorumstest.WithPreConnect(t, func(stopServers func()) {
//		stopServers()
//		time.Sleep(300 * time.Millisecond) // wait for server to fully stop
//	}))
//
// This option is intended for testing purposes only.
func WithPreConnect(_ testing.TB, fn func(stopServers func())) Option {
	if fn == nil {
		panic("gorumstest: WithPreConnect called with nil function")
	}
	return preConnectProvider{hook: fn}
}

// skipGoleakProvider is an Option that disables goleak checks.
type skipGoleakProvider struct{}

// SkipGoleak returns an Option that disables goleak checks for the test.
// This is useful when using synctest, which creates goroutines that goleak
// cannot properly track.
//
// Usage:
//
//	config := gorumstest.Config(t, 3, nil, gorumstest.SkipGoleak())
//
// This option is intended for testing purposes only.
func SkipGoleak() Option {
	return skipGoleakProvider{}
}
