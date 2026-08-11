package gorums_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/gorumstest"
	"github.com/relab/gorums/internal/testutils/mock"
	gorumsimpl "github.com/relab/gorums/runtime/gorumsimpl"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

// TestOnewayNoResourceLeak verifies that a one-way multicast does not register
// a router entry, so no pending calls are left behind. One-way sends are
// confirmed directly on the reply channel and never round-trip through the
// router, which is what keeps the pending set empty.
func TestOnewayNoResourceLeak(t *testing.T) {
	servers := gorumstest.LocalServers(t, 3)
	for _, srv := range servers {
		srv.RegisterHandler(mock.TestMethod, func(_ gorums.ServerContext, _ *gorums.Message) (*gorums.Message, error) {
			return nil, nil
		})
	}
	for _, srv := range servers {
		srv.WaitForPeers(t.Context(), func(cfg gorums.Config) bool {
			return cfg.Size() == 3
		})
	}
	cfg := servers[0].PeerConfig()
	ctx := gorumstest.Context(t, 5*time.Second)
	for i := range 1000 {
		if err := gorumsimpl.Multicast(cfg.Context(ctx), pb.String(fmt.Sprintf("mc-%d", i)), mock.TestMethod).Send(); err != nil {
			t.Fatalf("Multicast %d: %v", i, err)
		}
	}
	gorumstest.WaitUntil(t, 5*time.Second, func() bool {
		for _, node := range cfg.Nodes() {
			if node.PendingCount() > 0 {
				return false
			}
		}
		return true
	})

	for _, node := range cfg.Nodes() {
		if pc := node.PendingCount(); pc > 0 {
			t.Errorf("node %d: pending = %d; expected 0", node.ID(), pc)
		}
	}
}

// TestOnewayDroppedHandleDoesNotDispatch verifies that a one-way call handle
// dropped without consuming it never dispatches the request.
func TestOnewayDroppedHandleDoesNotDispatch(t *testing.T) {
	var received atomic.Int32
	servers := gorumstest.LocalServers(t, 3)
	for _, srv := range servers {
		srv.RegisterHandler(mock.TestMethod, func(_ gorums.ServerContext, _ *gorums.Message) (*gorums.Message, error) {
			received.Add(1)
			return nil, nil
		})
	}
	for _, srv := range servers {
		srv.WaitForPeers(t.Context(), func(cfg gorums.Config) bool {
			return cfg.Size() == 3
		})
	}
	cfg := servers[0].PeerConfig()
	ctx := gorumstest.Context(t, 2*time.Second)
	// Drop the handle without consuming it: nothing must be sent.
	_ = gorumsimpl.Multicast(cfg.Context(ctx), pb.String("dropped"), mock.TestMethod)
	// Allow time for any erroneous dispatch to reach the servers.
	time.Sleep(200 * time.Millisecond)
	if got := received.Load(); got != 0 {
		t.Errorf("dropped one-way handle dispatched to %d nodes; want 0", got)
	}
}

// TestOnewayCallDoubleDispatchPanics verifies that consuming the same handle a
// second time panics, in every combination of the two terminals, rather than
// silently re-sending the request or blocking on confirmations the first
// dispatch already drained.
func TestOnewayCallDoubleDispatchPanics(t *testing.T) {
	servers := gorumstest.LocalServers(t, 3)
	for _, srv := range servers {
		srv.RegisterHandler(mock.TestMethod, func(_ gorums.ServerContext, _ *gorums.Message) (*gorums.Message, error) {
			return nil, nil
		})
	}
	for _, srv := range servers {
		srv.WaitForPeers(t.Context(), func(cfg gorums.Config) bool {
			return cfg.Size() == 3
		})
	}
	cfg := servers[0].PeerConfig()

	tests := []struct {
		name string
		run  func(call *gorums.OnewayCall[*pb.StringValue])
	}{
		{"SendAfterSend", func(c *gorums.OnewayCall[*pb.StringValue]) {
			_ = c.Send()
			_ = c.Send()
		}},
		{"AsyncAfterSend", func(c *gorums.OnewayCall[*pb.StringValue]) {
			_ = c.Send()
			c.Async()
		}},
		{"SendAfterAsync", func(c *gorums.OnewayCall[*pb.StringValue]) {
			_ = c.Async().Wait()
			_ = c.Send()
		}},
		{"AsyncAfterAsync", func(c *gorums.OnewayCall[*pb.StringValue]) {
			_ = c.Async().Wait()
			c.Async()
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := gorumstest.Context(t, 2*time.Second)
			call := gorumsimpl.Multicast(cfg.Context(ctx), pb.String("x"), mock.TestMethod)
			defer func() {
				if recover() == nil {
					t.Errorf("%s did not panic", tt.name)
				}
			}()
			tt.run(call)
		})
	}
}

// TestOnewayCallAsync verifies the two properties that make Async worth having
// over Send: it returns before the sends complete, so a single goroutine can
// keep several calls in flight, and the deferred Wait still reports every send
// error that Send would have reported.
func TestOnewayCallAsync(t *testing.T) {
	const calls = 20
	var received atomic.Int32
	servers := gorumstest.LocalServers(t, 3)
	for _, srv := range servers {
		srv.RegisterHandler(mock.TestMethod, func(_ gorums.ServerContext, _ *gorums.Message) (*gorums.Message, error) {
			received.Add(1)
			return nil, nil
		})
	}
	for _, srv := range servers {
		srv.WaitForPeers(t.Context(), func(cfg gorums.Config) bool {
			return cfg.Size() == 3
		})
	}
	cfg := servers[0].PeerConfig()
	ctx := gorumstest.Context(t, 5*time.Second)

	// Dispatch every call before collecting any of them: with Send this loop
	// could not proceed until each multicast had reached all three nodes.
	handles := make([]*gorums.OnewayAsync, calls)
	for i := range handles {
		handles[i] = gorumsimpl.Multicast(cfg.Context(ctx), pb.String(fmt.Sprintf("async-%d", i)), mock.TestMethod).Async()
	}
	for i, h := range handles {
		if err := h.Wait(); err != nil {
			t.Errorf("handle %d: Wait = %v, want nil", i, err)
		}
		// Wait is idempotent: a second call returns the same result.
		if err := h.Wait(); err != nil {
			t.Errorf("handle %d: second Wait = %v, want nil", i, err)
		}
	}
	gorumstest.WaitUntil(t, 5*time.Second, func() bool {
		return received.Load() == calls*int32(cfg.Size())
	})
	if got, want := received.Load(), int32(calls*cfg.Size()); got != want {
		t.Errorf("servers received %d messages; want %d", got, want)
	}
}

// TestOnewayCallAsyncReportsSendError verifies that a send failure reaches the
// caller through the deferred Wait, so Async does not reintroduce the silent
// drop that a fire-and-forget terminal had.
func TestOnewayCallAsyncReportsSendError(t *testing.T) {
	const numServers = 3
	var stopNodes func(...int)
	config := gorumstest.Config(t, numServers, gorumstest.DefaultServer, gorumstest.WithStopFunc(t, &stopNodes))
	ctx := config.Context(t.Context())

	// Warm up so the streams are established before they are torn down.
	if err := gorumsimpl.Multicast(ctx, pb.String("warmup"), mock.TestMethod).Send(); err != nil {
		t.Fatalf("warmup: %v", err)
	}
	stopNodes(slices.Collect(gorumstest.Range(numServers))...)

	// Retry until the torn-down streams are observed, as the quorum-call
	// failure tests do; the send fails once the stream is gone, not the
	// instant the server stops.
	var err error
	for range 5 {
		if err = gorumsimpl.Multicast(ctx, pb.String("x"), mock.TestMethod).Async().Wait(); err != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err == nil {
		t.Fatal("Wait = nil, want a send failure after the servers stopped")
	}
	if !errors.Is(err, gorums.ErrSendFailure) {
		t.Errorf("Wait = %v, want %v", err, gorums.ErrSendFailure)
	}
}

// TestCallInterceptAfterDispatchPanics verifies that calling Intercept after a
// terminal method has started dispatch panics, since interceptors can no longer
// influence the in-flight call.
func TestCallInterceptAfterDispatchPanics(t *testing.T) {
	config := gorumstest.Config(t, 3, gorumstest.EchoServerFn)
	ctx := gorumstest.Context(t, 5*time.Second)
	call := gorumsimpl.QuorumCall[*pb.StringValue, *pb.StringValue](config.Context(ctx), pb.String("x"), mock.TestMethod)
	if _, err := call.Majority(); err != nil {
		t.Fatalf("Majority: %v", err)
	}
	defer func() {
		if recover() == nil {
			t.Error("Intercept after dispatch did not panic")
		}
	}()
	call.Intercept(gorums.MapResponse[*pb.StringValue](func(r *pb.StringValue, _ *gorums.Node) *pb.StringValue { return r }))
}

// TestCallInterceptAfterResultsPanics verifies that calling Intercept after
// Results panics, even though Results itself does not dispatch: without this,
// an interceptor registered on the handle after Results had already been
// called would silently fail to apply to the iterator the caller is holding.
func TestCallInterceptAfterResultsPanics(t *testing.T) {
	config := gorumstest.Config(t, 3, gorumstest.EchoServerFn)
	ctx := gorumstest.Context(t, 5*time.Second)
	call := gorumsimpl.QuorumCall[*pb.StringValue, *pb.StringValue](config.Context(ctx), pb.String("x"), mock.TestMethod)
	_ = call.Results()
	defer func() {
		if recover() == nil {
			t.Error("Intercept after Results did not panic")
		}
	}()
	call.Intercept(gorums.MapResponse[*pb.StringValue](func(r *pb.StringValue, _ *gorums.Node) *pb.StringValue { return r }))
}

// TestCallInterceptNilIgnored verifies that nil interceptors are ignored rather
// than causing a panic or affecting the result.
func TestCallInterceptNilIgnored(t *testing.T) {
	config := gorumstest.Config(t, 3, gorumstest.EchoServerFn)
	ctx := gorumstest.Context(t, 5*time.Second)
	resp, err := gorumsimpl.QuorumCall[*pb.StringValue, *pb.StringValue](config.Context(ctx), pb.String("test"), mock.TestMethod).
		Intercept(nil).
		Majority()
	if err != nil {
		t.Fatalf("Majority: %v", err)
	}
	if resp.GetValue() != "echo: test" {
		t.Errorf("got %q, want %q", resp.GetValue(), "echo: test")
	}
}

func TestRemoteCallSuccess(t *testing.T) {
	node := gorumstest.Node(t, gorumstest.DefaultServer)

	ctx := gorumstest.Context(t, 5*time.Second)
	nodeCtx := node.Context(ctx)
	response, err := gorumsimpl.RemoteCall[*pb.StringValue, *pb.StringValue](nodeCtx, pb.String(""), mock.TestMethod)
	if err != nil {
		t.Fatalf("Unexpected error, got: %v, want: %v", err, nil)
	}
	if response == nil {
		t.Fatalf("Unexpected response, got: %v, want: non-nil", nil)
	}
}

func TestRemoteCallDownedNode(t *testing.T) {
	node := gorumstest.Node(t, gorumstest.DefaultServer, gorumstest.WithPreConnect(t, func(stopServers func()) {
		stopServers()
		time.Sleep(300 * time.Millisecond) // wait for servers to fully stop
	}))

	ctx := gorumstest.Context(t, 5*time.Second)
	nodeCtx := node.Context(ctx)
	response, err := gorumsimpl.RemoteCall[*pb.StringValue, *pb.StringValue](nodeCtx, pb.String(""), mock.TestMethod)
	if err == nil {
		t.Fatalf("Expected error, got: %v, want: %v", err, fmt.Errorf("rpc error: code = Unavailable desc = stream is down"))
	}
	if response != nil {
		t.Fatalf("Unexpected response, got: %v, want: %v", response, nil)
	}
}

func TestRemoteCallTimedOut(t *testing.T) {
	node := gorumstest.Node(t, gorumstest.DefaultServer)

	ctx, cancel := context.WithTimeout(t.Context(), 0*time.Second)
	time.Sleep(50 * time.Millisecond)
	defer cancel()
	nodeCtx := node.Context(ctx)
	response, err := gorumsimpl.RemoteCall[*pb.StringValue, *pb.StringValue](nodeCtx, pb.String(""), mock.TestMethod)
	if err == nil {
		t.Fatalf("Expected error, got: %v, want: %v", err, fmt.Errorf("context deadline exceeded"))
	}
	if response != nil {
		t.Fatalf("Unexpected response, got: %v, want: %v", response, nil)
	}
}

func TestRemoteCallTypeMismatch(t *testing.T) {
	node := gorumstest.Node(t, gorumstest.DefaultServer)

	ctx := gorumstest.Context(t, 5*time.Second)
	nodeCtx := node.Context(ctx)
	response, err := gorumsimpl.RemoteCall[*pb.StringValue, *pb.Int32Value](nodeCtx, pb.String(""), mock.TestMethod)
	if err != gorums.ErrTypeMismatch {
		t.Fatalf("Expected error, got: %v, want: %v", err, gorums.ErrTypeMismatch)
	}
	if response != nil {
		t.Fatalf("Unexpected response, got: %v, want: %v", response, nil)
	}
}

func TestRemoteCallConcurrentAccess(t *testing.T) {
	node := gorumstest.Node(t, gorumstest.DefaultServer)

	concurrency := 10
	errCh := make(chan error, concurrency)
	var wg sync.WaitGroup
	for range concurrency {
		wg.Go(func() {
			_, err := gorumsimpl.RemoteCall[*pb.StringValue, *pb.StringValue](node.Context(t.Context()), pb.String(""), mock.TestMethod)
			if err != nil {
				errCh <- err
			}
		})
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
}
