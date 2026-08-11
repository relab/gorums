package gorums_test

import (
	"bytes"
	"context"
	"log"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/gorumstest"
	"github.com/relab/gorums/internal/testutils/mock"
	gorumsimpl "github.com/relab/gorums/runtime/gorumsimpl"
	"google.golang.org/grpc/metadata"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

func TestServerCallback(t *testing.T) {
	var message string
	signal := make(chan struct{})

	srvOption := gorums.WithConnectCallback(func(ctx context.Context) {
		m, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return
		}
		message = m.Get("message")[0]
		signal <- struct{}{}
	})
	dialOption := gorums.WithMetadata(metadata.New(map[string]string{"message": "hello"}))

	gorumstest.Node(t, nil, srvOption, dialOption)

	select {
	case <-time.After(100 * time.Millisecond):
	case <-signal:
	}

	if message != "hello" {
		t.Errorf("incorrect message: got '%s', want 'hello'", message)
	}
}

func appendStringInterceptor(inStr, outStr string) gorums.ServerInterceptor {
	return func(ctx gorums.ServerContext, in *gorums.Message, next gorums.Handler) (*gorums.Message, error) {
		req := gorums.AsProto[*pb.StringValue](in)
		// update the underlying request gorums.Message's message field (pb.StringValue in this case)
		req.Value += inStr

		// We do not need to re-marshal into the payload here.
		// The next handler in the chain will access req via gorums.AsProto(in) which reads in.Proto.

		// call the next handler
		out, err := next(ctx, in)
		if err != nil {
			return nil, err
		}
		resp := gorums.AsProto[*pb.StringValue](out)
		// update the underlying response gorums.Message's message field (pb.StringValue in this case)
		resp.Value += outStr
		// We do not need to re-marshal the response into the payload either.
		// SendMessage will lazily marshal it before sending it on the wire.
		return out, err
	}
}

type interceptorSrv struct{}

func (interceptorSrv) Test(_ gorums.ServerContext, req *pb.StringValue) (*pb.StringValue, error) {
	return pb.String(req.GetValue() + "server-"), nil
}

func TestServerInterceptorsChain(t *testing.T) {
	// set up a server with two interceptors: i1, i2
	interceptorServerFn := func(_ int) gorums.ServerIface {
		interceptorSrv := &interceptorSrv{}
		s := gorums.NewServer(gorums.WithServerInterceptors(
			appendStringInterceptor("i1in-", "i1out"),
			appendStringInterceptor("i2in-", "i2out-"),
		))
		// register final handler which appends "final-" to the request value
		s.RegisterHandler(mock.TestMethod, func(ctx gorums.ServerContext, in *gorums.Message) (*gorums.Message, error) {
			req := gorums.AsProto[*pb.StringValue](in)
			resp, err := interceptorSrv.Test(ctx, req)
			if err != nil {
				return nil, err
			}
			return gorums.NewResponseMessage(in, resp), nil
		})
		return s
	}
	node := gorumstest.Node(t, interceptorServerFn)

	ctx := gorumstest.Context(t, 5*time.Second)
	nodeCtx := node.Context(ctx)
	res, err := gorumsimpl.RemoteCall[*pb.StringValue, *pb.StringValue](nodeCtx, pb.String("client-"), mock.TestMethod)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatalf("unexpected nil response")
	}
	want := "client-i1in-i2in-server-i2out-i1out"
	if res.GetValue() != want {
		t.Fatalf("unexpected response value: got %q, want %q", res.GetValue(), want)
	}
}

// TestWithBufferSizesProcessesRequests verifies that WithBufferSizes is accepted by
// NewServer and that the server correctly processes concurrent requests for each
// combination of receive and send buffer sizes, including the zero (unbuffered) case.
func TestWithBufferSizesProcessesRequests(t *testing.T) {
	const concurrency = 16
	tests := []struct {
		name     string
		recvSize uint
		sendSize uint
	}{
		{name: "unbuffered", recvSize: 0, sendSize: 0},
		{name: "recv-only", recvSize: 1, sendSize: 0},
		{name: "send-only", recvSize: 0, sendSize: 1},
		{name: "both-buffered", recvSize: concurrency, sendSize: concurrency},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := gorumstest.Node(t, nil, gorums.WithBufferSizes(tt.recvSize, tt.sendSize))
			ctx := gorumstest.Context(t, 5*time.Second)

			var wg sync.WaitGroup
			errs := make([]error, concurrency)
			for i := range concurrency {
				wg.Go(func() {
					nodeCtx := node.Context(ctx)
					_, errs[i] = gorumsimpl.RemoteCall[*pb.StringValue, *pb.StringValue](nodeCtx, pb.String(""), mock.TestMethod)
				})
			}
			wg.Wait()

			for i, err := range errs {
				if err != nil {
					t.Errorf("request %d failed: %v", i, err)
				}
			}
		})
	}
}

// TestTCPReconnection verifies that a node can reconnect after the
// underlying TCP connection is broken.
func TestTCPReconnection(t *testing.T) {
	srv := gorums.NewServer()
	srv.RegisterHandler(mock.TestMethod, func(_ gorums.ServerContext, in *gorums.Message) (*gorums.Message, error) {
		req := gorums.AsProto[*pb.StringValue](in)
		return gorums.NewResponseMessage(in, req), nil
	})

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	addr := lis.Addr().String()

	go func() {
		_ = srv.Serve(lis)
	}()

	cfg, err := gorums.NewConfig(gorums.WithNodeList([]string{addr}), gorumstest.InsecureDialOptions(t))
	if err != nil {
		t.Fatalf("NewConfig failed: %v", err)
	}
	t.Cleanup(gorumstest.Closer(t, cfg))
	node := cfg.Nodes()[0]

	// Send first message
	ctx := gorumstest.Context(t, time.Second)
	nodeCtx := node.Context(ctx)
	_, err = gorumsimpl.RemoteCall[*pb.StringValue, *pb.StringValue](nodeCtx, pb.String("1"), mock.TestMethod)
	if err != nil {
		t.Fatalf("First call failed: %v", err)
	}

	// Stop server
	srv.Stop()
	lis.Close()

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Sending now should fail or timeout
	ctx2 := gorumstest.Context(t, 200*time.Millisecond)
	nodeCtx2 := node.Context(ctx2)
	_, err = gorumsimpl.RemoteCall[*pb.StringValue, *pb.StringValue](nodeCtx2, pb.String("2"), mock.TestMethod)
	if err == nil {
		// It might succeed if it just queued it? But we wait for response.
	} else {
		t.Logf("Got expected error during downtime: %v", err)
	}

	// Restart server
	lis2, err := net.Listen("tcp", addr)
	if err != nil {
		t.Skipf("Could not re-bind to %s: %v", addr, err)
	}

	srv2 := gorums.NewServer()
	srv2.RegisterHandler(mock.TestMethod, func(_ gorums.ServerContext, in *gorums.Message) (*gorums.Message, error) {
		req := gorums.AsProto[*pb.StringValue](in)
		return gorums.NewResponseMessage(in, req), nil
	})
	go func() {
		_ = srv2.Serve(lis2)
	}()
	defer srv2.Stop()

	// Wait for client backoff/reconnect
	time.Sleep(2 * time.Second)

	// Send message again
	ctx3 := gorumstest.Context(t, 2*time.Second)
	nodeCtx3 := node.Context(ctx3)
	_, err = gorumsimpl.RemoteCall[*pb.StringValue, *pb.StringValue](nodeCtx3, pb.String("3"), mock.TestMethod)
	if err != nil {
		t.Errorf("Call after reconnection failed: %v", err)
	}
}

// TestNewLocalServersStopBeforeServeClosesListeners verifies that the stop
// function returned by NewLocalServers closes all pre-allocated listeners even
// when none of the servers has had ListenAndServe called yet, so no file
// descriptors are leaked.
func TestNewLocalServersStopBeforeServeClosesListeners(t *testing.T) {
	servers, stop, err := gorums.NewLocalServers(3, gorums.WithLocalDialOptions(gorumstest.InsecureDialOptions(t)))
	if err != nil {
		t.Fatalf("NewLocalServers: %v", err)
	}
	addrs := make([]string, len(servers))
	for i, srv := range servers {
		addrs[i] = srv.Addr()
	}
	stop() // called before any Serve()
	// Every pre-allocated listener must be closed. Assert this by dialing each
	// address and expecting a refused connection, rather than re-binding a new
	// listener to it: re-binding races with anything else on the machine that
	// might grab the now-free ephemeral port, which caused flakiness before.
	for _, addr := range addrs {
		if conn, err := net.DialTimeout("tcp", addr, 2*time.Second); err == nil {
			_ = conn.Close()
			t.Errorf("connected to %s after stop (without Serve); expected the listener to be closed", addr)
		}
	}
}

// TestNewLocalServersAssignsSequentialNodeIDs verifies that NewLocalServers
// assigns node IDs 1..n in the order the servers are returned.
func TestNewLocalServersAssignsSequentialNodeIDs(t *testing.T) {
	const n = 4
	servers, stop, err := gorums.NewLocalServers(n, gorums.WithLocalDialOptions(gorumstest.InsecureDialOptions(t)))
	if err != nil {
		t.Fatalf("NewLocalServers: %v", err)
	}
	t.Cleanup(stop)

	for i, srv := range servers {
		if want := uint32(i + 1); srv.NodeID() != want {
			t.Errorf("servers[%d].NodeID() = %d, want %d", i, srv.NodeID(), want)
		}
	}
}

// TestNewLocalServersPeerConfigSize verifies that each server's peer
// configuration includes every node in the symmetric group, without stream
// deduplication enabled.
func TestNewLocalServersPeerConfigSize(t *testing.T) {
	const n = 4
	servers, stop, err := gorums.NewLocalServers(n, gorums.WithLocalDialOptions(gorumstest.InsecureDialOptions(t)))
	if err != nil {
		t.Fatalf("NewLocalServers: %v", err)
	}
	t.Cleanup(stop)

	for i, srv := range servers {
		if got := srv.PeerConfig().Size(); got != n {
			t.Errorf("servers[%d].PeerConfig().Size() = %d, want %d", i, got, n)
		}
	}
}

// TestNewLocalServersAppliesServerOptions verifies that a ServerOption passed
// via WithLocalServerOptions is applied to every server, not just the first.
func TestNewLocalServersAppliesServerOptions(t *testing.T) {
	const n = 3
	var connects atomic.Int32
	servers, stop, err := gorums.NewLocalServers(
		n,
		gorums.WithLocalServerOptions(gorums.WithConnectCallback(func(context.Context) { connects.Add(1) })),
		gorums.WithLocalDialOptions(gorumstest.InsecureDialOptions(t)),
	)
	if err != nil {
		t.Fatalf("NewLocalServers: %v", err)
	}
	t.Cleanup(stop)
	for _, srv := range servers {
		go func() { _ = srv.ListenAndServe() }()
	}

	// Wait on the callback counter directly: WaitForPeers observes this
	// server's own connections, which can be established before any peer's
	// inbound connect callback has run.
	if !gorumstest.WaitUntil(t, 5*time.Second, func() bool { return connects.Load() > 0 }) {
		t.Error("WithConnectCallback never fired; ServerOption was not applied to the local servers")
	}
}

// TestNewLocalServersAppliesDialOptions verifies that a DialOption passed via
// WithLocalDialOptions is applied to every server's outbound configuration.
// WithLogger's "ready" line is written synchronously when the outbound
// manager is constructed, so this does not depend on any network activity.
func TestNewLocalServersAppliesDialOptions(t *testing.T) {
	const n = 3
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	_, stop, err := gorums.NewLocalServers(
		n,
		gorums.WithLocalDialOptions(gorumstest.InsecureDialOptions(t), gorums.WithLogger(logger)),
	)
	if err != nil {
		t.Fatalf("NewLocalServers: %v", err)
	}
	t.Cleanup(stop)

	if got := strings.Count(buf.String(), "ready"); got != n {
		t.Errorf("logger recorded %d \"ready\" lines, want %d (one per server); DialOption was not applied to every server", got, n)
	}
}

// TestNewLocalServersStopIsIdempotent verifies that the stop function
// returned by NewLocalServers can be called more than once without panicking.
func TestNewLocalServersStopIsIdempotent(t *testing.T) {
	_, stop, err := gorums.NewLocalServers(3, gorums.WithLocalDialOptions(gorumstest.InsecureDialOptions(t)))
	if err != nil {
		t.Fatalf("NewLocalServers: %v", err)
	}
	stop()
	stop()
}

// TestServerAddrBeforeAndAfterBinding verifies that Addr returns the configured
// listen address before binding and the concrete bound address after
// ListenAndServe binds a port-0 listener.
func TestServerAddrBeforeAndAfterBinding(t *testing.T) {
	srv := gorums.NewServer(gorums.WithAddr("127.0.0.1:0"))
	t.Cleanup(srv.Stop)

	if got := srv.Addr(); got != "127.0.0.1:0" {
		t.Errorf("Addr before binding = %q, want %q", got, "127.0.0.1:0")
	}

	go func() { _ = srv.ListenAndServe() }()

	if !gorumstest.WaitUntil(t, 2*time.Second, func() bool {
		return srv.Addr() != "127.0.0.1:0"
	}) {
		t.Fatalf("Addr did not update after binding; still %q", srv.Addr())
	}
	if _, port, err := net.SplitHostPort(srv.Addr()); err != nil || port == "" || port == "0" {
		t.Errorf("Addr after binding = %q, want a concrete bound port", srv.Addr())
	}
}

// TestListenAndServeAfterStopReturnsError verifies that calling ListenAndServe
// after Stop has already been called returns an error instead of silently
// binding and serving on an already-stopped server, and that the listener
// ListenAndServe binds in that case does not leak: grpc.Server.Serve closes
// any listener handed to an already-stopped server.
func TestListenAndServeAfterStopReturnsError(t *testing.T) {
	srv := gorums.NewServer(gorums.WithAddr("127.0.0.1:0"))
	srv.Stop()

	err := srv.ListenAndServe()
	if err == nil {
		t.Fatal("ListenAndServe after Stop = nil error, want error")
	}

	addr := srv.Addr()
	if _, port, splitErr := net.SplitHostPort(addr); splitErr != nil || port == "" || port == "0" {
		t.Fatalf("Addr after ListenAndServe = %q, want a concrete bound port", addr)
	}
	// The listener ListenAndServe bound must already be closed (by grpc's Serve
	// on a stopped server), not leaked: dialing it must be refused.
	if conn, dialErr := net.DialTimeout("tcp", addr, 2*time.Second); dialErr == nil {
		_ = conn.Close()
		t.Fatalf("connected to %s after ListenAndServe on a stopped server; expected the listener to be closed", addr)
	}
}

// TestListenAndServeWithoutAddrReturnsError verifies that ListenAndServe returns
// a clear error when no listen address was configured and no listener was
// preallocated.
func TestListenAndServeWithoutAddrReturnsError(t *testing.T) {
	srv := gorums.NewServer()
	t.Cleanup(srv.Stop)
	if err := srv.ListenAndServe(); err == nil {
		t.Fatal("ListenAndServe without a listen address = nil error, want error")
	}
}

// TestServeRecordsListenerForAddrAndStop verifies that Serve records the
// externally supplied listener so that Addr reports its address and Stop closes
// it, matching the folded lifecycle semantics.
func TestServeRecordsListenerForAddrAndStop(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := lis.Addr().String()
	srv := gorums.NewServer()
	go func() { _ = srv.Serve(lis) }()

	if !gorumstest.WaitUntil(t, 2*time.Second, func() bool {
		return srv.Addr() == addr
	}) {
		t.Fatalf("Addr = %q, want %q after Serve", srv.Addr(), addr)
	}
	srv.Stop()
	// Stop must close the recorded listener. Assert this by dialing the address
	// and expecting a refused connection, rather than re-binding a new listener
	// to it: re-binding races with anything else on the machine that might grab
	// the now-free ephemeral port, which caused flakiness before.
	if conn, err := net.DialTimeout("tcp", addr, 2*time.Second); err == nil {
		_ = conn.Close()
		t.Fatalf("connected to %s after Stop; expected the listener to be closed", addr)
	}
}

// TestOutboundInvalidPanics verifies that an invalid outbound node source
// configured via WithPeers panics during NewServer.
func TestOutboundInvalidPanics(t *testing.T) {
	// Duplicate address makes the node source invalid.
	invalid := gorums.WithNodeList([]string{"127.0.0.1:1", "127.0.0.1:1"})
	assertPanics(t, "WithPeers", func() {
		gorums.NewServer(gorums.WithPeers(1, invalid))
	})
}

func assertPanics(t *testing.T, name string, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("NewServer with invalid %s did not panic", name)
		}
	}()
	fn()
}
