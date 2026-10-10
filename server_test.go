package gorums_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/gorumstest"
	"github.com/relab/gorums/internal/stream"
	"github.com/relab/gorums/internal/testutils/mock"
	gorumsimpl "github.com/relab/gorums/runtime/gorumsimpl"
	"go.uber.org/goleak"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

// TestServerConnectCallback verifies that the connect callback receives the
// stream context carrying the metadata the client dialed with.
func TestServerConnectCallback(t *testing.T) {
	messages := make(chan string, 1)
	srvOption := gorums.WithConnectCallback(func(ctx context.Context) {
		if m, ok := metadata.FromIncomingContext(ctx); ok {
			messages <- m.Get("message")[0]
		}
	})
	dialOption := gorums.WithMetadata(metadata.New(map[string]string{"message": "hello"}))

	gorumstest.Node(t, nil, srvOption, dialOption)

	select {
	case message := <-messages:
		if message != "hello" {
			t.Errorf("connect callback message = %q, want %q", message, "hello")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("connect callback was not called")
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
	interceptorServerFn := func(_ int) gorumstest.ServerIface {
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

// TestServerBufferSizesProcessRequests verifies that the server processes
// concurrent requests for each combination of receive and send buffer sizes,
// including size 0, which selects the default size.
func TestServerBufferSizesProcessRequests(t *testing.T) {
	const concurrency = 16
	tests := []struct {
		name     string
		recvSize uint
		sendSize uint
	}{
		{name: "Defaults", recvSize: 0, sendSize: 0},
		{name: "RecvSize1", recvSize: 1, sendSize: 0},
		{name: "SendSize1", recvSize: 0, sendSize: 1},
		{name: "BothSizesConcurrency", recvSize: concurrency, sendSize: concurrency},
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

// TestServerListenAndServeAfterStopReturnsError verifies that calling ListenAndServe
// after Stop has already been called returns an error instead of silently
// binding and serving on an already-stopped server, and that the listener
// ListenAndServe binds in that case does not leak: grpc.Server.Serve closes
// any listener handed to an already-stopped server.
func TestServerListenAndServeAfterStopReturnsError(t *testing.T) {
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

// TestServerListenAndServeWithoutAddrReturnsError verifies that ListenAndServe returns
// a clear error when no listen address was configured and no listener was
// preallocated.
func TestServerListenAndServeWithoutAddrReturnsError(t *testing.T) {
	srv := gorums.NewServer()
	t.Cleanup(srv.Stop)
	if err := srv.ListenAndServe(); err == nil {
		t.Fatal("ListenAndServe without a listen address = nil error, want error")
	}
}

// TestServerServeRecordsListenerForAddrAndStop verifies that Serve records the
// externally supplied listener so that Addr reports its address and Stop closes
// it, matching the folded lifecycle semantics.
func TestServerServeRecordsListenerForAddrAndStop(t *testing.T) {
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
	// and expecting a refused connection; re-binding the port would race with
	// anything else on the machine that grabs the free ephemeral port.
	if conn, err := net.DialTimeout("tcp", addr, 2*time.Second); err == nil {
		_ = conn.Close()
		t.Fatalf("connected to %s after Stop; expected the listener to be closed", addr)
	}
}

// TestServerInvalidPeersPanics verifies that an invalid outbound node source
// configured via WithPeers panics during NewServer.
func TestServerInvalidPeersPanics(t *testing.T) {
	// Duplicate address makes the node source invalid.
	invalid := gorums.WithNodeList([]string{"127.0.0.1:1", "127.0.0.1:1"})
	assertPanic(t, "gorums: invalid peer configuration:", func() {
		gorums.NewServer(gorums.WithPeers(1, invalid))
	})
}

// TestServerPeerChangeDeliversUsableConfig verifies that the last WithPeerChange
// snapshot delivered while NewServer runs holds the nodes of PeerConfig, which
// can place calls, not placeholder nodes of the inbound view.
func TestServerPeerChangeDeliversUsableConfig(t *testing.T) {
	var snapshots []gorums.Config
	srv := gorums.NewServer(
		gorums.WithPeers(1, gorums.WithNodeList([]string{"127.0.0.1:1", "127.0.0.1:2"}), gorumstest.InsecureDialOptions(t)),
		gorums.WithPeerChange(func(c gorums.Config) { snapshots = append(snapshots, c) }),
	)
	defer srv.Stop()
	if len(snapshots) == 0 {
		t.Fatal("WithPeerChange was not called during NewServer")
	}
	last := snapshots[len(snapshots)-1]
	for _, node := range last.Nodes() {
		if want := gorumstest.PeerNode(t, srv.PeerConfig(), node.ID()); node != want {
			t.Errorf("snapshot node %d is not PeerConfig's node", node.ID())
		}
	}
}

// TestServerHandleRequestRelease verifies that HandleRequest calls release
// only when the handler does, and otherwise leaves the release on return to
// its caller, whose dispatcher then runs the next request on the same
// goroutine.
func TestServerHandleRequestRelease(t *testing.T) {
	payload, err := proto.Marshal(pb.String("x"))
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	tests := []struct {
		name         string
		method       string
		handler      gorums.Handler
		wantReleases int
	}{
		{
			name:   "HandlerReturns",
			method: mock.TestMethod,
			handler: func(_ gorums.ServerContext, in *gorums.Message) (*gorums.Message, error) {
				return gorums.NewResponseMessage(in, gorums.AsProto[*pb.StringValue](in)), nil
			},
			wantReleases: 0,
		},
		{
			name:   "HandlerReleases",
			method: mock.TestMethod,
			handler: func(ctx gorums.ServerContext, in *gorums.Message) (*gorums.Message, error) {
				ctx.Release()
				return gorums.NewResponseMessage(in, gorums.AsProto[*pb.StringValue](in)), nil
			},
			wantReleases: 1,
		},
		{
			name:         "UnknownMethod",
			method:       "unknown.Service/Method",
			wantReleases: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := gorums.NewServer()
			if tt.handler != nil {
				srv.RegisterHandler(mock.TestMethod, tt.handler)
			}
			req := stream.Message_builder{Method: tt.method, Payload: payload}.Build()
			releases, sends := 0, 0
			srv.HandleRequest(t.Context(), 0, req, func() { releases++ }, func(*stream.Message) { sends++ })
			if releases != tt.wantReleases {
				t.Errorf("release called %d times, want %d", releases, tt.wantReleases)
			}
			if sends != 1 {
				t.Errorf("send called %d times, want 1", sends)
			}
		})
	}
}

// TestServerConnectedPeersDropsStoppedPeer verifies that a stopped peer
// disappears from the other servers' ConnectedPeers once their connections
// to it drop, and that WaitForPeers observes the change.
func TestServerConnectedPeersDropsStoppedPeer(t *testing.T) {
	servers := gorumstest.LocalServers(t, 3)
	gorumstest.WaitForPeers(t, servers)

	servers[2].Stop()
	for i, srv := range servers[:2] {
		ctx := gorumstest.Context(t, 5*time.Second)
		if err := srv.WaitForPeers(ctx, func(cfg gorums.Config) bool {
			return !cfg.Contains(3)
		}); err != nil {
			t.Errorf("server %d still lists stopped peer 3: %v; ConnectedPeers = %v",
				i+1, err, srv.ConnectedPeers().NodeIDs())
		}
	}
}

// waitWithTimeout waits for wg to reach zero or calls t.Fatal if the timeout elapses.
func waitWithTimeout(t *testing.T, wg *sync.WaitGroup) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for handlers to be invoked")
	}
}

// awaitClientReady waits until the server's ConnectedClients contains n connected peers.
func awaitClientReady(t *testing.T, srv *gorums.Server, n int) {
	t.Helper()
	ctx := gorumstest.Context(t, 5*time.Second)
	if err := srv.WaitForClients(ctx, func(cfg gorums.Config) bool {
		return cfg.Size() == n
	}); err != nil {
		t.Fatalf("awaitClientReady: %v", err)
	}
}

// createServerAndClient creates a server and a client for back-channel testing.
// The server automatically tracks anonymous clients and can dispatch back-channel
// calls to them via [ServerContext.ConnectedClients].
// The client is a standalone [*gorums.Server] (no listener needed) whose registered handlers
// are reachable by the server over the existing bidirectional gRPC stream. The returned
// [gorums.Config] is the client's outbound config pointing at the server.
func createServerAndClient(t *testing.T) (*gorums.Server, *gorums.Server, gorums.Config) {
	t.Helper()

	// Server side: accepts anonymous clients for back-channel calls. Bind an
	// explicit listener (allocated once and kept open for the server's lifetime)
	// so its address is known when constructing the client below.
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := gorums.NewServer()

	// Client side: a plain handler-only Server (no listener, no peers) that the
	// server can invoke via the back-channel. WithBackChannel installs it as the
	// back-channel dispatcher; dispatch is over the client's outbound gRPC
	// stream. The client has no node ID, so the server tracks it as an
	// anonymous client reachable via ConnectedClients.
	clientSrv := gorums.NewServer()
	cfg, closeFn, err := gorums.NewConfig(
		gorums.WithNodeList([]string{lis.Addr().String()}),
		gorums.WithBackChannel(clientSrv),
		gorumstest.InsecureDialOptions(t),
	)
	if err != nil {
		t.Fatal(err)
	}

	go func() { _ = srv.Serve(lis) }()

	// Registered in reverse order so cleanup (LIFO) closes the client's
	// outbound config, cfg, before stopping clientSrv and then srv (which
	// stops the server and closes lis).
	t.Cleanup(srv.Stop)
	t.Cleanup(clientSrv.Stop)
	t.Cleanup(closeFn)

	return srv, clientSrv, cfg
}

func configContext(ctx gorums.ServerContext, client bool) (*gorums.ConfigContext, error) {
	if client {
		cfg := ctx.ConnectedClients()
		if len(cfg) == 0 {
			return nil, errors.New("ConnectedClients: expected non-empty config")
		}
		return cfg.Context(ctx), nil
	}
	cfg := ctx.PeerConfig()
	if len(cfg) == 0 {
		return nil, errors.New("PeerConfig: expected non-empty config")
	}
	return cfg.Context(ctx), nil
}

// outerChainedHandler returns an outer handler that fans out an inner quorum call
// on innerMethod, then combines the outer request value with the inner result.
// Unlike innerQuorumCallHandler, routing is done entirely by method registration:
// no message-content inspection is needed.
func outerChainedHandler(
	t *testing.T,
	myID int,
	client bool,
	innerMethod string,
	respFn func(*gorums.Responses[*pb.StringValue]) (*pb.StringValue, error),
) gorums.Handler {
	t.Helper()
	return func(ctx gorums.ServerContext, in *gorums.Message) (*gorums.Message, error) {
		req := gorums.AsProto[*pb.StringValue](in)
		t.Logf("Server %d received outer request: %s", myID, req.GetValue())
		// Release before making the inner quorum call, so the next request on
		// this stream can start while this handler waits for the inner-call
		// responses.
		ctx.Release()
		configCtx, err := configContext(ctx, client)
		if err != nil {
			return nil, err
		}
		responses := gorumsimpl.QuorumCall[*pb.StringValue, *pb.StringValue](
			configCtx,
			req,
			innerMethod,
		)
		res, err := respFn(responses.Responses)
		if err != nil {
			return nil, err
		}
		return gorums.NewResponseMessage(in, pb.String(req.GetValue()+" | "+res.GetValue())), nil
	}
}

func TestServerSymmetricConfigurationRoutesQuorumCalls(t *testing.T) {
	servers := gorumstest.LocalServers(t, 3)

	// Register mock handler to each server
	for _, srv := range servers {
		srv.RegisterHandler(mock.TestMethod, gorumstest.EchoHandler("echo"))
	}

	gorumstest.WaitForPeers(t, servers)

	// type alias short hand for the responses type
	type respType = *gorums.Responses[*pb.StringValue]
	tests := []struct {
		name      string
		call      func(respType) (*pb.StringValue, error)
		wantValue string
	}{
		{
			name:      "Majority",
			call:      respType.Majority,
			wantValue: "echo: test",
		},
		{
			name:      "First",
			call:      respType.First,
			wantValue: "echo: test",
		},
		{
			name:      "All",
			call:      respType.All,
			wantValue: "echo: test",
		},
	}

	// Use the auto-created outbound config from server 0.
	cfg := servers[0].PeerConfig()
	// Sub tests for each response type logic across symmetric routing
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := gorumstest.Context(t, 2*time.Second)

			responses := gorumsimpl.QuorumCall[*pb.StringValue, *pb.StringValue](
				cfg.Context(ctx),
				pb.String("test"),
				mock.TestMethod,
			)

			result, err := tt.call(responses.Responses)
			if err != nil {
				t.Fatalf("quorum call error: %v", err)
			}

			if result.GetValue() != tt.wantValue {
				t.Errorf("Expected %q, got %q", tt.wantValue, result.GetValue())
			}
		})
	}
}

func TestServerSymmetricConfigurationRoutesMulticast(t *testing.T) {
	servers := gorumstest.LocalServers(t, 3)

	var wg sync.WaitGroup
	wg.Add(len(servers))

	// Register mock handler to each server
	for _, srv := range servers {
		srv.RegisterHandler(mock.StreamMethod, func(_ gorums.ServerContext, _ *gorums.Message) (*gorums.Message, error) {
			wg.Done()
			return nil, nil
		})
	}

	gorumstest.WaitForPeers(t, servers)

	cfg := servers[0].PeerConfig()
	ctx := gorumstest.Context(t, 2*time.Second)
	err := gorumsimpl.Multicast(
		cfg.Context(ctx),
		pb.String("test"),
		mock.StreamMethod,
	).Send()
	if err != nil {
		t.Fatalf("multicast error: %v", err)
	}

	waitWithTimeout(t, &wg)
}

func TestServerHandlerCanMulticastViaConfig(t *testing.T) {
	servers := gorumstest.LocalServers(t, 3)

	// 3 servers receive the outer multicast. Each server multicasts to a config of 3 nodes.
	// The self-node's handler is invoked locally, so each server sends to all 3 nodes.
	// Total = 3 * 3 = 9 messages received.
	var wg sync.WaitGroup
	wg.Add(9)

	for i, srv := range servers {
		srv.RegisterHandler(mock.TestMethod, func(ctx gorums.ServerContext, in *gorums.Message) (*gorums.Message, error) {
			t.Logf("Server %d received multicast on %v: %v", i+1, mock.TestMethod, in.Proto)
			// Release before the nested multicast: the peer configuration
			// includes the local node, whose in-process dispatch waits for
			// this handler's dispatch lock.
			ctx.Release()
			if cfg := ctx.PeerConfig(); cfg.Size() == 3 {
				err := gorumsimpl.Multicast(
					cfg.Context(t.Context()),
					pb.String("inner-multicast"),
					mock.StreamMethod,
				).Send()
				if err != nil {
					return nil, err // failed to multicast
				}
			}
			return nil, nil // one-way
		})

		srv.RegisterHandler(mock.StreamMethod, func(_ gorums.ServerContext, in *gorums.Message) (*gorums.Message, error) {
			t.Logf("Server %d received multicast on %v: %v", i+1, mock.StreamMethod, in.Proto)
			wg.Done()
			return nil, nil
		})
	}

	gorumstest.WaitForPeers(t, servers)

	cfg := servers[0].PeerConfig()
	ctx := gorumstest.Context(t, 2*time.Second)
	err := gorumsimpl.Multicast(
		cfg.Context(ctx),
		pb.String("outer-multicast"),
		mock.TestMethod,
	).Send()
	if err != nil {
		t.Fatalf("multicast error: %v", err)
	}

	waitWithTimeout(t, &wg)
}

func TestServerHandlerCanChainQuorumCallViaConfig(t *testing.T) {
	type respType = *gorums.Responses[*pb.StringValue]

	// seqAll drains the Results iterator to exhaustion and returns the last value.
	// This is the regression path for the self-node dispatch bug where .Results()
	// and .All() would time out when self was included in the quorum.
	seqAll := func(r respType) (*pb.StringValue, error) {
		var last *pb.StringValue
		for result := range r.Results() {
			if result.Err != nil {
				return nil, result.Err
			}
			last = result.Value
		}
		if last == nil {
			return nil, errors.New("Results: no responses received")
		}
		return last, nil
	}

	tests := []struct {
		name    string
		innerFn func(respType) (*pb.StringValue, error)
		outerFn func(respType) (*pb.StringValue, error)
	}{
		{name: "Majority", innerFn: respType.Majority, outerFn: respType.Majority},
		{name: "All", innerFn: respType.All, outerFn: respType.All}, // Regression: .All() must not time out when self-node is included.
		{name: "Results", innerFn: seqAll, outerFn: respType.All},   // Regression: draining .Results() to exhaustion must not time out when self-node is included.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			servers := gorumstest.LocalServers(t, 3)

			for i, srv := range servers {
				myID := i + 1
				srv.RegisterHandler(mock.TestMethod, outerChainedHandler(t, myID, false, mock.EchoMethod, tt.innerFn))
				srv.RegisterHandler(mock.EchoMethod, gorumstest.EchoHandler("inner-echo"))
			}

			gorumstest.WaitForPeers(t, servers)

			cfg := servers[0].PeerConfig()
			ctx := gorumstest.Context(t, 2*time.Second)

			responses := gorumsimpl.QuorumCall[*pb.StringValue, *pb.StringValue](
				cfg.Context(ctx),
				pb.String("outer-call"),
				mock.TestMethod,
			)
			result, err := tt.outerFn(responses.Responses)
			if err != nil {
				t.Fatalf("quorum call error: %v", err)
			}
			t.Logf("Final result: %s", result.GetValue())

			wantResult := "outer-call | inner-echo: outer-call"
			if !strings.Contains(result.GetValue(), wantResult) {
				t.Errorf("Expected %q in result, got: %s", wantResult, result.GetValue())
			}
		})
	}
}

func TestServerHandlerCanChainQuorumCallViaConnectedClients(t *testing.T) {
	srv, clientSrv, cfgClient := createServerAndClient(t)

	// Server: outer handler fans out an inner quorum call on EchoMethod to all
	// client peers and returns whichever responds first.
	srv.RegisterHandler(mock.TestMethod, outerChainedHandler(t, 1, true, mock.EchoMethod, (*gorums.Responses[*pb.StringValue]).First))

	// Client: handles EchoMethod calls dispatched back by the server via ConnectedClients.
	clientSrv.RegisterHandler(mock.EchoMethod, gorumstest.EchoHandler("client-echo"))

	awaitClientReady(t, srv, 1)

	ctx := gorumstest.Context(t, 2*time.Second)
	responses := gorumsimpl.QuorumCall[*pb.StringValue, *pb.StringValue](
		cfgClient.Context(ctx),
		pb.String("outer-call"),
		mock.TestMethod,
	)
	result, err := responses.First()
	if err != nil {
		t.Fatalf("quorum call error: %v", err)
	}
	t.Logf("CLIENT final result: %v", result.GetValue())
	// The server fans out EchoMethod to ConnectedClients (client only).
	// Result: "outer-call | client-echo: outer-call"
	if !strings.HasPrefix(result.GetValue(), "outer-call | ") {
		t.Errorf("Expected result to start with %q, got %q", "outer-call | ", result.GetValue())
	}
}

func TestServerHandlerCanMulticastViaConnectedClients(t *testing.T) {
	srv, clientSrv, cfgClient := createServerAndClient(t)

	// Outer multicast from client triggers the server handler once.
	// The server fans out an inner multicast via ConnectedClients (1 client) -> 1 message.
	var wg sync.WaitGroup
	wg.Add(1)

	srv.RegisterHandler(mock.TestMethod, func(ctx gorums.ServerContext, in *gorums.Message) (*gorums.Message, error) {
		t.Logf("SERVER received multicast: %v", in.Proto)
		if cfg := ctx.ConnectedClients(); cfg.Size() == 1 {
			err := gorumsimpl.Multicast(
				cfg.Context(t.Context()),
				pb.String("inner-call"),
				mock.StreamMethod,
			).Send()
			if err != nil {
				return nil, err // failed to multicast
			}
		}
		return nil, nil // one-way
	})

	// Client handles the back-channel multicast dispatched by the server.
	clientSrv.RegisterHandler(mock.StreamMethod, func(_ gorums.ServerContext, _ *gorums.Message) (*gorums.Message, error) {
		t.Log("CLIENT received inner multicast")
		wg.Done()
		return nil, nil
	})

	awaitClientReady(t, srv, 1)

	ctx := gorumstest.Context(t, 2*time.Second)
	err := gorumsimpl.Multicast(
		cfgClient.Context(ctx),
		pb.String("trigger"),
		mock.TestMethod,
	).Send()
	if err != nil {
		t.Fatalf("multicast error: %v", err)
	}

	waitWithTimeout(t, &wg)
}

// TestServerLocalDispatchContention verifies that sequential quorum calls remain
// correct when an earlier call returns before all replicas have replied and the
// next call starts immediately on the same configuration.
//
// Gorums only guarantees FIFO ordering for sequentially issued quorum calls.
// Concurrent quorum calls (from separate goroutines) violate the FIFO ordering
// contract and are therefore not tested.
//
// Each subtest creates its own isolated servers so that goroutines left over
// from one subtest cannot contaminate the next.
func TestServerLocalDispatchContention(t *testing.T) {
	startServers := func(t *testing.T) gorums.Config {
		t.Helper()
		servers := gorumstest.LocalServers(t, 3)
		for _, srv := range servers {
			srv.RegisterHandler(mock.TestMethod, func(_ gorums.ServerContext, in *gorums.Message) (*gorums.Message, error) {
				req := gorums.AsProto[*pb.StringValue](in)
				return gorums.NewResponseMessage(in, pb.String("echo: "+req.GetValue())), nil
			})
		}
		gorumstest.WaitForPeers(t, servers)
		return servers[0].PeerConfig()
	}

	const delay = 2000 * time.Millisecond

	// SequentialMajorityThenAll exercises the common case where a quorum-sized
	// result is returned first and a full-result call follows immediately after.
	t.Run("SequentialMajorityThenAll", func(t *testing.T) {
		cfg := startServers(t)
		const iterations = 500
		for i := range iterations {
			ctx, cancel := context.WithTimeout(t.Context(), delay)
			responses := gorumsimpl.QuorumCall[*pb.StringValue, *pb.StringValue](
				cfg.Context(ctx),
				pb.String(fmt.Sprintf("m-%d", i)),
				mock.TestMethod,
			)
			if _, err := responses.Majority(); err != nil {
				cancel()
				t.Fatalf("iteration %d: Majority: %v", i, err)
			}
			cancel()

			ctx, cancel = context.WithTimeout(t.Context(), delay)
			responses = gorumsimpl.QuorumCall[*pb.StringValue, *pb.StringValue](
				cfg.Context(ctx),
				pb.String(fmt.Sprintf("a-%d", i)),
				mock.TestMethod,
			)
			if _, err := responses.All(); err != nil {
				cancel()
				t.Fatalf("iteration %d: All: %v", i, err)
			}
			cancel()
		}
	})

	// GOMAXPROCS1 uses the earliest-returning terminal method first and then
	// immediately requires all replies, while constraining scheduling to
	// amplify timing-sensitive ordering bugs.
	t.Run("GOMAXPROCS1", func(t *testing.T) {
		prev := runtime.GOMAXPROCS(1)
		defer runtime.GOMAXPROCS(prev)

		cfg := startServers(t)
		const iterations = 500
		for i := range iterations {
			ctx, cancel := context.WithTimeout(t.Context(), delay)
			responses := gorumsimpl.QuorumCall[*pb.StringValue, *pb.StringValue](
				cfg.Context(ctx),
				pb.String(fmt.Sprintf("g-%d", i)),
				mock.TestMethod,
			)
			if _, err := responses.First(); err != nil {
				cancel()
				t.Fatalf("iteration %d: First: %v", i, err)
			}
			cancel()

			ctx, cancel = context.WithTimeout(t.Context(), delay)
			responses = gorumsimpl.QuorumCall[*pb.StringValue, *pb.StringValue](
				cfg.Context(ctx),
				pb.String(fmt.Sprintf("ga-%d", i)),
				mock.TestMethod,
			)
			if _, err := responses.All(); err != nil {
				cancel()
				t.Fatalf("iteration %d: All: %v", i, err)
			}
			cancel()
		}
	})
}

// TestServerLocalDispatchContentionSlowReplica verifies that a slow local
// replica does not prevent a new quorum call from making progress on replies
// from the remote replicas.
//
// The test first lets a remote reply satisfy an early-returning call while the
// local replica is intentionally delayed, then immediately issues an All call.
// The All call must observe remote progress right away and complete once the
// delayed local reply is finally allowed through.
func TestServerLocalDispatchContentionSlowReplica(t *testing.T) {
	servers := gorumstest.LocalServers(t, 3)

	// blocker delays server 0's handler so its reply is intentionally late.
	blocker := make(chan struct{})
	closeBlocker := sync.OnceFunc(func() { close(blocker) })
	t.Cleanup(closeBlocker) // safety net: unblock handler goroutines on test exit

	for i, srv := range servers {
		if i == 0 {
			// Server 0 (self-node): block until signaled.
			srv.RegisterHandler(mock.TestMethod, func(_ gorums.ServerContext, in *gorums.Message) (*gorums.Message, error) {
				<-blocker
				req := gorums.AsProto[*pb.StringValue](in)
				return gorums.NewResponseMessage(in, pb.String("echo: "+req.GetValue())), nil
			})
		} else {
			// Servers 1, 2 (remote): respond immediately.
			srv.RegisterHandler(mock.TestMethod, gorumstest.EchoHandler("echo"))
		}
	}

	gorumstest.WaitForPeers(t, servers)
	cfg := servers[0].PeerConfig()

	// Step 1: First(1) succeeds from a remote response while the local reply is
	// still blocked.
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	responses := gorumsimpl.QuorumCall[*pb.StringValue, *pb.StringValue](
		cfg.Context(ctx),
		pb.String("first-call"),
		mock.TestMethod,
	)
	result, err := responses.First()
	cancel()
	if err != nil {
		closeBlocker()
		t.Fatalf("First: %v", err)
	}
	if result.GetValue() != "echo: first-call" {
		closeBlocker()
		t.Fatalf("First: got %q, want %q", result.GetValue(), "echo: first-call")
	}

	// Step 2: Release the delayed local reply after a short pause.
	go func() {
		time.Sleep(100 * time.Millisecond)
		closeBlocker()
	}()

	// Step 3: All(3) should succeed. The remote replies are expected promptly,
	// and the delayed local reply should arrive once the blocker releases.
	ctx, cancel = context.WithTimeout(t.Context(), 2*time.Second)
	responses = gorumsimpl.QuorumCall[*pb.StringValue, *pb.StringValue](
		cfg.Context(ctx),
		pb.String("all-call"),
		mock.TestMethod,
	)
	result, err = responses.All()
	cancel()
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if result.GetValue() != "echo: all-call" {
		t.Errorf("All: got %q, want %q", result.GetValue(), "echo: all-call")
	}
}

func TestServerWaitForPeers(t *testing.T) {
	t.Run("ConditionAlreadyMet", func(t *testing.T) {
		servers := gorumstest.LocalServers(t, 3)
		gorumstest.WaitForPeers(t, servers)

		ctx := gorumstest.Context(t, 2*time.Second)
		if err := servers[0].WaitForPeers(ctx, func(cfg gorums.Config) bool {
			return cfg.Size() == 3
		}); err != nil {
			t.Fatalf("WaitForPeers: %v", err)
		}
	})

	t.Run("ConditionMetAfterConnect", func(t *testing.T) {
		servers := gorumstest.LocalServers(t, 3)

		ctx := gorumstest.Context(t, 5*time.Second)
		if err := servers[0].WaitForPeers(ctx, func(cfg gorums.Config) bool {
			return cfg.Size() == 3
		}); err != nil {
			t.Fatalf("WaitForPeers: %v", err)
		}
	})

	t.Run("ContextCancelled", func(t *testing.T) {
		srv := gorums.NewServer(gorums.WithAddr("127.0.0.1:0"))
		go func() { _ = srv.ListenAndServe() }()
		t.Cleanup(srv.Stop)

		ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
		defer cancel()
		err := srv.WaitForPeers(ctx, func(cfg gorums.Config) bool {
			return cfg.Size() == 3 // never true
		})
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected DeadlineExceeded, got: %v", err)
		}
	})

	t.Run("ServerStopped", func(t *testing.T) {
		srv := gorums.NewServer(gorums.WithAddr("127.0.0.1:0"))
		go func() { _ = srv.ListenAndServe() }()

		errCh := make(chan error, 1)
		go func() {
			errCh <- srv.WaitForPeers(context.Background(), func(cfg gorums.Config) bool {
				return cfg.Size() == 3 // never true
			})
		}()

		// Give WaitForPeers time to enter the select.
		time.Sleep(20 * time.Millisecond)
		srv.Stop()

		select {
		case err := <-errCh:
			if !errors.Is(err, gorums.ErrStopped) {
				t.Fatalf("expected ErrStopped, got: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("WaitForPeers did not return after Stop")
		}
	})

	t.Run("ConcurrentWaiters", func(t *testing.T) {
		servers := gorumstest.LocalServers(t, 3)

		const waiters = 5
		errCh := make(chan error, waiters)
		for range waiters {
			ctx := gorumstest.Context(t, 5*time.Second)
			go func(ctx context.Context) {
				errCh <- servers[0].WaitForPeers(ctx, func(cfg gorums.Config) bool {
					return cfg.Size() == 3
				})
			}(ctx)
		}

		for range waiters {
			if err := <-errCh; err != nil {
				t.Errorf("WaitForPeers: %v", err)
			}
		}
	})

	t.Run("ConnectedClients", func(t *testing.T) {
		srv, _, _ := createServerAndClient(t)

		ctx := gorumstest.Context(t, 5*time.Second)
		if err := srv.WaitForClients(ctx, func(cfg gorums.Config) bool {
			return cfg.Size() == 1
		}); err != nil {
			t.Fatalf("WaitForClients: %v", err)
		}
	})
}

// TestServerGracefulStopReleasesPeerConfig verifies that GracefulStop unblocks
// WaitForPeers and stops the peer configuration's connection goroutines.
// WithNodeList assigns the single unreachable peer ID 1, so myID is 2 and
// that peer is a real outbound channel rather than the in-process local node.
func TestServerGracefulStopReleasesPeerConfig(t *testing.T) {
	t.Cleanup(func() { goleak.VerifyNone(t) })

	const myID uint32 = 2
	srv := gorums.NewServer(gorums.WithPeers(
		myID,
		gorums.WithNodeList([]string{"127.0.0.1:1"}),
		gorumstest.InsecureDialOptions(t),
	))

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.WaitForPeers(context.Background(), func(gorums.Config) bool {
			return false
		})
	}()

	// Let WaitForPeers block before stopping.
	time.Sleep(20 * time.Millisecond)
	srv.GracefulStop()

	select {
	case err := <-errCh:
		if !errors.Is(err, gorums.ErrStopped) {
			t.Fatalf("WaitForPeers returned %v, want ErrStopped", err)
		}
	case <-time.After(time.Second):
		t.Fatal("WaitForPeers did not return after GracefulStop")
	}
}

// TestServerGracefulStopWithOpenClientStream verifies that GracefulStop returns
// while a client stream is open, including when the client keeps calling
// during the drain.
func TestServerGracefulStopWithOpenClientStream(t *testing.T) {
	srv := gorums.NewServer()
	srv.RegisterHandler(mock.TestMethod, func(_ gorums.ServerContext, in *gorums.Message) (*gorums.Message, error) {
		return gorums.NewResponseMessage(in, pb.String("echo")), nil
	})
	cfg := gorumstest.Config(t, 1, func(int) gorumstest.ServerIface { return srv })
	node := cfg.Nodes()[0]

	call := func() error {
		ctx := gorumstest.Context(t, 2*time.Second)
		_, err := gorumsimpl.RemoteCall[*pb.StringValue, *pb.StringValue](node.Context(ctx), pb.String("x"), mock.TestMethod)
		return err
	}
	if err := call(); err != nil {
		t.Fatalf("call before GracefulStop: %v", err)
	}

	stopped := make(chan struct{})
	go func() {
		srv.GracefulStop()
		close(stopped)
	}()
	// The client keeps the stream open; calls during the drain may fail, but
	// must not keep GracefulStop from returning.
	_ = call()

	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("GracefulStop did not return with a client stream open")
	}
}

// TestServerBackChannelNestedCallBeforeRelease verifies that a client handler can
// call the server that sent the request before calling Release, and that a
// second in-flight request does not stop the reply from being read.
func TestServerBackChannelNestedCallBeforeRelease(t *testing.T) {
	srv := gorums.NewServer()
	srv.RegisterHandler(mock.EchoMethod, func(_ gorums.ServerContext, in *gorums.Message) (*gorums.Message, error) {
		return gorums.NewResponseMessage(in, pb.String("echo")), nil
	})
	addrs := gorumstest.Servers(t, 1, func(int) gorumstest.ServerIface { return srv })

	var (
		clientCfg      gorums.Config
		handlers       atomic.Int32
		firstStarted   = make(chan struct{})
		secondInFlight = make(chan struct{})
	)
	clientSrv := gorums.NewServer()
	clientSrv.RegisterHandler(mock.TestMethod, func(_ gorums.ServerContext, in *gorums.Message) (*gorums.Message, error) {
		if handlers.Add(1) == 1 {
			close(firstStarted)
			<-secondInFlight
		}
		nctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		resp, err := gorumsimpl.RemoteCall[*pb.StringValue, *pb.StringValue](clientCfg.Nodes()[0].Context(nctx), pb.String("nested"), mock.EchoMethod)
		if err != nil {
			return nil, err
		}
		return gorums.NewResponseMessage(in, resp), nil
	})
	cfg, closeFn, err := gorums.NewConfig(gorums.WithNodeList(addrs), gorumstest.DialOptions(t), gorums.WithBackChannel(clientSrv))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(closeFn)
	clientCfg = cfg

	if err := srv.WaitForClients(gorumstest.Context(t, 5*time.Second), func(c gorums.Config) bool { return c.Size() == 1 }); err != nil {
		t.Fatal(err)
	}
	clientNode := srv.ConnectedClients().Nodes()[0]

	errCh := make(chan error, 2)
	call := func() {
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		_, callErr := gorumsimpl.RemoteCall[*pb.StringValue, *pb.StringValue](clientNode.Context(ctx), pb.String("bc"), mock.TestMethod)
		errCh <- callErr
	}
	go call()
	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("first back-channel handler did not start")
	}
	go call()
	// Let the second request reach the client before the first handler calls
	// back, so the reply is not already waiting when that call is made.
	time.Sleep(50 * time.Millisecond)
	close(secondInFlight)

	for range 2 {
		select {
		case err := <-errCh:
			if err != nil {
				t.Errorf("back-channel call: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("back-channel call did not return")
		}
	}
}

// TestServerHandlerNestedCallBeforeRelease verifies that a server handler can
// call the client that sent the request before calling Release. The reply
// arrives on the same stream the handler's request was read from.
func TestServerHandlerNestedCallBeforeRelease(t *testing.T) {
	srv := gorums.NewServer()
	srv.RegisterHandler(mock.TestMethod, func(ctx gorums.ServerContext, in *gorums.Message) (*gorums.Message, error) {
		clients := ctx.ConnectedClients()
		if clients.Size() == 0 {
			t.Error("ConnectedClients is empty")
			return nil, context.DeadlineExceeded
		}
		nctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		resp, err := gorumsimpl.RemoteCall[*pb.StringValue, *pb.StringValue](clients.Nodes()[0].Context(nctx), pb.String("nested"), mock.EchoMethod)
		if err != nil {
			return nil, err
		}
		return gorums.NewResponseMessage(in, resp), nil
	})
	addrs := gorumstest.Servers(t, 1, func(int) gorumstest.ServerIface { return srv })

	clientSrv := gorums.NewServer()
	clientSrv.RegisterHandler(mock.EchoMethod, func(_ gorums.ServerContext, in *gorums.Message) (*gorums.Message, error) {
		return gorums.NewResponseMessage(in, pb.String("echo")), nil
	})
	cfg, closeFn, err := gorums.NewConfig(gorums.WithNodeList(addrs), gorumstest.DialOptions(t), gorums.WithBackChannel(clientSrv))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(closeFn)
	if err := srv.WaitForClients(gorumstest.Context(t, 5*time.Second), func(c gorums.Config) bool { return c.Size() == 1 }); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if _, err := gorumsimpl.RemoteCall[*pb.StringValue, *pb.StringValue](cfg.Nodes()[0].Context(ctx), pb.String("trigger"), mock.TestMethod); err != nil {
		t.Fatalf("server handler nested call: %v", err)
	}
}

// senderRecord is the sender ID that a receiving server's handler observed.
type senderRecord struct {
	receiver gorums.ID
	sender   gorums.ID
}

// senderRecorder returns a one-way handler that reports the request's
// [gorums.ServerContext.SenderID] as observed by receiver.
func senderRecorder(receiver gorums.ID, records chan<- senderRecord) gorums.Handler {
	return func(ctx gorums.ServerContext, _ *gorums.Message) (*gorums.Message, error) {
		records <- senderRecord{receiver: receiver, sender: ctx.SenderID()}
		return nil, nil
	}
}

// multicastString sends a one-way multicast to cfg and fails the test on error.
func multicastString(t *testing.T, cfg gorums.Config, method string) {
	t.Helper()
	ctx := gorumstest.Context(t, 2*time.Second)
	if err := gorumsimpl.Multicast(cfg.Context(ctx), pb.String("sender"), method).Send(); err != nil {
		t.Fatalf("Multicast: %v", err)
	}
}

// TestServerSenderID verifies that a handler observes the sender of a request
// on every path a request can take to a server.
func TestServerSenderID(t *testing.T) {
	// peerSenderIDs multicasts from every server to its peer configuration,
	// which includes the server itself, and checks that every receiver
	// observes the multicasting server's ID.
	peerSenderIDs := func(t *testing.T, opts ...gorums.ServerOption) {
		servers := gorumstest.LocalServers(t, 3, opts...)
		records := make(chan senderRecord, len(servers))
		for _, srv := range servers {
			srv.RegisterHandler(mock.StreamMethod, senderRecorder(srv.NodeID(), records))
		}
		gorumstest.WaitForPeers(t, servers)
		for _, srv := range servers {
			multicastString(t, srv.PeerConfig(), mock.StreamMethod)
			for _, r := range gorumstest.Collect(t, 2*time.Second, len(servers), records) {
				if r.sender != srv.NodeID() {
					t.Errorf("server %d: SenderID() = %d, want %d", r.receiver, r.sender, srv.NodeID())
				}
			}
		}
	}

	// clientSenderID multicasts from a client that dials with opts and
	// returns the sender ID that the server observed.
	clientSenderID := func(t *testing.T, opts ...gorumstest.Option) gorums.ID {
		records := make(chan senderRecord, 1)
		srvFn := func(int) gorumstest.ServerIface {
			srv := gorums.NewServer()
			srv.RegisterHandler(mock.StreamMethod, senderRecorder(0, records))
			return srv
		}
		cfg := gorumstest.Config(t, 1, srvFn, opts...)
		multicastString(t, cfg, mock.StreamMethod)
		return gorumstest.Collect(t, 2*time.Second, 1, records)[0].sender
	}

	// Inbound streams from known peers, and the in-process self node.
	t.Run("KnownPeersAndSelf", func(t *testing.T) { peerSenderIDs(t) })
	// A higher-ID peer's requests arrive on the stream the receiver dialed.
	t.Run("StreamDedup", func(t *testing.T) { peerSenderIDs(t, gorums.WithStreamDedup()) })

	t.Run("RegularClient", func(t *testing.T) {
		if got := clientSenderID(t); got != 0 {
			t.Errorf("SenderID() = %d, want 0", got)
		}
	})
	t.Run("UnknownPeerID", func(t *testing.T) {
		forged := gorums.WithMetadata(metadata.Pairs("gorums-node-id", "99"))
		if got := clientSenderID(t, forged); got != 0 {
			t.Errorf("SenderID() = %d, want 0", got)
		}
	})

	t.Run("BackChannelClient", func(t *testing.T) {
		srv, _, cfg := createServerAndClient(t)
		records := make(chan senderRecord, 1)
		srv.RegisterHandler(mock.StreamMethod, senderRecorder(0, records))
		awaitClientReady(t, srv, 1)
		multicastString(t, cfg, mock.StreamMethod)
		got := gorumstest.Collect(t, 2*time.Second, 1, records)[0].sender
		if want := srv.ConnectedClients().NodeIDs()[0]; got != want {
			t.Errorf("SenderID() = %d, want %d (the client's assigned ID)", got, want)
		}
	})
	t.Run("BackChannelServer", func(t *testing.T) {
		srv, clientSrv, cfg := createServerAndClient(t)
		records := make(chan senderRecord, 1)
		clientSrv.RegisterHandler(mock.StreamMethod, senderRecorder(0, records))
		awaitClientReady(t, srv, 1)
		multicastString(t, srv.ConnectedClients(), mock.StreamMethod)
		got := gorumstest.Collect(t, 2*time.Second, 1, records)[0].sender
		if want := cfg.NodeIDs()[0]; got != want {
			t.Errorf("SenderID() = %d, want %d (the client's ID for the server)", got, want)
		}
	})
}
