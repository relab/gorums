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
// order. It returns the values collected so far when timeout elapses in total
// or ch is closed, so a test that waits for effects a failure may never
// produce, such as one-way messages, can report the shortfall.
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
		case v, ok := <-ch:
			if !ok {
				return got
			}
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

// Config creates servers and a configuration for testing.
// Both server and configuration cleanup are handled via t.Cleanup in the correct
// order: the configuration is closed first, then servers are stopped.
//
// The provided srvFn is used to create and register the server handlers.
// If srvFn is nil, a default mock server implementation is used.
//
// Optional [Option] values can be provided to customize the dial options, server, or configuration.
//
// By default, nodes are assigned sequential IDs (1, 2, 3, ...) matching the server
// creation order. This can be overridden by providing a [gorums.NodeSource].
//
// This is the recommended way to set up tests that need both servers and a configuration.
// It ensures proper cleanup and detects goroutine leaks.
func Config(t testing.TB, numServers int, srvFn func(i int) ServerIface, opts ...Option) gorums.Config {
	t.Helper()

	testOpts := extractTestOptions(opts)

	// Register goleak check FIRST so it runs LAST (LIFO order)
	// Skip it for benchmarks and when goleak checks are explicitly skipped
	if _, ok := t.(*testing.B); !ok && !testOpts.shouldSkipGoleak() {
		t.Cleanup(func() { goleak.VerifyNone(t) })
	}

	// Start servers and register cleanup.
	addrs, stopFn := servers.Start(t, numServers, testOpts.serverFunc(srvFn))
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
	dialOptions := append([]gorums.DialOption{DialOptions(t)}, testOpts.dialOpts...)
	cfg, closeFn, err := gorums.NewConfig(testOpts.nodeSource(addrs), dialOptions...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(closeFn)
	return cfg
}

// unreachableSentinelAddr is a loopback address with no server.
const unreachableSentinelAddr = "127.0.0.1:1"

// UnreachableConfig returns a [gorums.Config] over addrs with no server
// behind any address. Tests use it to obtain a valid configuration whose calls
// can never complete. If addrs is empty, the configuration uses a single
// loopback address with no server.
func UnreachableConfig(t testing.TB, addrs ...string) gorums.Config {
	t.Helper()
	if len(addrs) == 0 {
		addrs = []string{unreachableSentinelAddr}
	}
	cfg, closeFn, err := gorums.NewConfig(gorums.WithNodeList(addrs), InsecureDialOptions(t))
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}
	t.Cleanup(closeFn)
	return cfg
}

// Node creates a single server and returns the node for testing.
// Both server and configuration cleanup are handled via t.Cleanup in the correct order.
//
// The provided srvFn is used to create and register the server handler.
// If srvFn is nil, a default mock server implementation is used.
//
// Optional [Option] values can be provided to customize the dial options, server, or configuration.
//
// This is the recommended way to set up tests that need only a single server node.
// It ensures proper cleanup and detects goroutine leaks.
func Node(t testing.TB, srvFn func(i int) ServerIface, opts ...Option) *gorums.Node {
	t.Helper()
	return Config(t, 1, srvFn, opts...).Nodes()[0]
}

// PeerNode returns the node with the given id in cfg, or fails the test.
func PeerNode(t testing.TB, cfg gorums.Config, id gorums.ID) *gorums.Node {
	t.Helper()
	if node := cfg.Node(id); node != nil {
		return node
	}
	t.Fatalf("node %d not in config %v", id, cfg.NodeIDs())
	return nil
}
