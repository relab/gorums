// Package gorumstest provides test helpers for setting up gorums servers,
// configurations, and nodes, modeled on net/http/httptest.
//
// These helpers pull in goroutine-leak detection (goleak) and, in the
// default build, an in-memory bufconn dialer; keeping them in this separate
// package means importing github.com/relab/gorums alone does not pull in
// those test-only dependencies.
package gorumstest

import (
	"context"
	"io"
	"iter"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/internal/testutils/servers"
	"go.uber.org/goleak"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Context creates a context with timeout for testing.
// It uses t.Context() as the parent and automatically cancels on cleanup.
func Context(t testing.TB, timeout time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	t.Cleanup(cancel)
	return ctx
}

// WaitUntil polls predicate until it returns true or timeout elapses.
// It returns true when predicate succeeds within timeout, and false otherwise.
func WaitUntil(t testing.TB, timeout time.Duration, predicate func() bool) bool {
	t.Helper()

	if predicate() {
		return true
	}

	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return predicate()
		case <-ticker.C:
			if predicate() {
				return true
			}
		}
	}
}

// Collect receives up to want values from ch and returns them in arrival
// order. It gives up when timeout elapses in total, returning the values
// collected so far, so the caller can report a shortfall instead of blocking
// forever. Use it wherever a test waits for effects that a failure may never
// produce, such as one-way messages: [gorums.OnewayCall.Send] discards a
// request it cannot deliver without reporting an error, so an unbounded wait
// would hang the package until the test binary's timeout.
//
// Usage:
//
//	got := gorumstest.Collect(t, time.Second, want, srv.received)
//	if len(got) != want {
//		t.Errorf("server received %d messages, expected %d", len(got), want)
//	}
func Collect[T any](t testing.TB, timeout time.Duration, want int, ch <-chan T) []T {
	t.Helper()
	got := make([]T, 0, want)
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for range want {
		select {
		case v := <-ch:
			got = append(got, v)
		case <-timer.C:
			return got
		}
	}
	return got
}

// InsecureDialOptions returns a [gorums.DialOption] with insecure transport
// credentials for testing.
func InsecureDialOptions(_ testing.TB) gorums.DialOption {
	return gorums.WithGRPCDialOptions(
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
}

// DialOptions returns a [gorums.DialOption] for connecting to servers
// started by [Servers], [Config], or [Node]: an in-memory
// bufconn dialer in the default build, or insecure real-network credentials
// under the integration build tag.
func DialOptions(t testing.TB) gorums.DialOption {
	return gorums.WithGRPCDialOptions(servers.DialOptions(t)...)
}

// startServers starts numServers servers via srvFn, adapting srvFn's
// [gorums.ServerIface] result to the servers package's own, independently
// defined ServerIface (see the doc comment on gorums.ServerIface for why).
func startServers(t testing.TB, numServers int, srvFn func(i int) gorums.ServerIface) ([]string, func(...int)) {
	return servers.Start(t, numServers, func(i int) servers.ServerIface { return srvFn(i) })
}

// Config creates servers and a configuration for testing.
// Both server and manager cleanup are handled via t.Cleanup in the correct order:
// manager is closed first, then servers are stopped.
//
// The provided srvFn is used to create and register the server handlers.
// If srvFn is nil, a default mock server implementation is used.
//
// Optional [Option] values can be provided to customize the manager, server, or configuration.
//
// By default, nodes are assigned sequential IDs (1, 2, 3, ...) matching the server
// creation order. This can be overridden by providing a [gorums.NodeSource].
//
// This is the recommended way to set up tests that need both servers and a configuration.
// It ensures proper cleanup and detects goroutine leaks.
func Config(t testing.TB, numServers int, srvFn func(i int) gorums.ServerIface, opts ...Option) gorums.Config {
	t.Helper()

	testOpts := extractTestOptions(opts)

	// Register goleak check FIRST so it runs LAST (LIFO order)
	// Only register if not reusing an existing manager (to avoid duplicate checks)
	// and if goleak checks are not explicitly skipped
	if _, ok := t.(*testing.B); !ok && !testOpts.shouldSkipGoleak() {
		t.Cleanup(func() { goleak.VerifyNone(t) })
	}

	// Start servers and register cleanup.
	addrs, stopFn := startServers(t, numServers, testOpts.serverFunc(srvFn))
	stopAllFn := func() { stopFn() } // wrap to call without arguments to stop all servers
	t.Cleanup(stopAllFn)

	// Capture the provided stop function to stop individual servers later
	if testOpts.stopFuncPtr != nil {
		*testOpts.stopFuncPtr = stopFn
	}

	// Call preConnect hook if set (before connecting to servers)
	if testOpts.preConnectHook != nil {
		testOpts.preConnectHook(stopAllFn)
	}

	// Create configuration and register its cleanup LAST so it runs FIRST (LIFO)
	dialOptions := append([]gorums.DialOption{DialOptions(t)}, testOpts.managerOpts...)
	cfg, err := gorums.NewConfig(testOpts.nodeSource(addrs), dialOptions...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(Closer(t, cfg))
	return cfg
}

// NoDialedConfig returns a [gorums.Config] over addrs whose nodes are never
// actually dialed: gRPC connections are established lazily on the first RPC,
// so tests that only need a valid Config to construct calls, without ever
// completing one, don't need a running server behind it. If addrs is empty,
// a single unreachable sentinel address is used.
func NoDialedConfig(t testing.TB, addrs ...string) gorums.Config {
	t.Helper()
	if len(addrs) == 0 {
		addrs = []string{"127.0.0.1:65535"}
	}
	cfg, err := gorums.NewConfig(gorums.WithNodeList(addrs), InsecureDialOptions(t))
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}
	t.Cleanup(Closer(t, cfg))
	return cfg
}

// Node creates a single server and returns the node for testing.
// Both server and manager cleanup are handled via t.Cleanup in the correct order.
//
// The provided srvFn is used to create and register the server handler.
// If srvFn is nil, a default mock server implementation is used.
//
// Optional [Option] values can be provided to customize the manager, server, or configuration.
//
// This is the recommended way to set up tests that need only a single server node.
// It ensures proper cleanup and detects goroutine leaks.
func Node(t testing.TB, srvFn func(i int) gorums.ServerIface, opts ...Option) *gorums.Node {
	t.Helper()
	return Config(t, 1, srvFn, opts...).Nodes()[0]
}

// Servers starts numServers gRPC servers using the given registration
// function. Servers are automatically stopped when the test finishes via t.Cleanup.
// The cleanup is registered first, so it runs after any subsequently registered
// cleanups (e.g., manager.Close()), ensuring proper shutdown ordering.
//
// Goroutine leak detection via goleak is automatically enabled and runs after
// all other cleanup functions complete.
//
// The provided srvFn is used to create and register the server handlers.
// If srvFn is nil, a default mock server implementation is used.
//
// Example usage:
//
//	addrs := gorumstest.Servers(t, 3, serverFn)
//	cfg, err := gorums.NewConfig(gorums.WithNodeList(addrs), gorumstest.DialOptions(t))
//	t.Cleanup(gorumstest.Closer(t, cfg))
//	...
//
// This function can be used by other packages for testing purposes, as long as
// the required service, method, and message types are registered in the global
// protobuf registry before calling this function.
func Servers(t testing.TB, numServers int, srvFn func(i int) gorums.ServerIface) []string {
	t.Helper()
	// Skip goleak check for benchmarks
	if _, ok := t.(*testing.B); !ok {
		// Register goleak check FIRST so it runs LAST (after all other cleanup)
		t.Cleanup(func() { goleak.VerifyNone(t) })
	}
	addrs, stopFn := startServers(t, numServers, srvFn)
	// Register server cleanup SECOND so it runs BEFORE goleak check
	t.Cleanup(func() { stopFn() }) // wrap to call without arguments to stop all servers
	return addrs
}

// LocalServers returns n started Gorums servers forming a symmetric peer
// group on random localhost ports (see [gorums.NewLocalServers]). Each
// server auto-creates a peer [gorums.Config] over the group, accessible
// via [gorums.Server.PeerConfig]. The servers are automatically stopped
// when the test finishes via t.Cleanup. Any [gorums.ServerOption]s are
// applied to every server.
func LocalServers(t testing.TB, n int, opts ...gorums.ServerOption) []*gorums.Server {
	t.Helper()

	// Skip goleak check for benchmarks
	if _, ok := t.(*testing.B); !ok {
		// Register goleak check FIRST so it runs LAST (after all other cleanup)
		t.Cleanup(func() { goleak.VerifyNone(t) })
	}

	srvs, stop, err := gorums.NewLocalServers(n, gorums.WithLocalServerOptions(opts...), gorums.WithLocalDialOptions(InsecureDialOptions(t)))
	if err != nil {
		t.Fatal(err)
	}

	// Register server cleanup SECOND so it runs BEFORE goleak check
	t.Cleanup(stop)

	for _, srv := range srvs {
		go srv.ListenAndServe()
	}

	return srvs
}

// Closer returns a cleanup function that closes the given io.Closer.
func Closer(t testing.TB, c io.Closer) func() {
	t.Helper()
	return func() {
		if err := c.Close(); err != nil {
			t.Errorf("c.Close() = %q, expected no error", err.Error())
		}
	}
}

// Range yields the integers [0, n).
func Range(n int) iter.Seq[int] {
	return func(yield func(int) bool) {
		for i := range n {
			if !yield(i) {
				return
			}
		}
	}
}
