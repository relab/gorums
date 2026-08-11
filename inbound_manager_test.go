package gorums

import (
	"context"
	"io"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/relab/gorums/internal/conn"
	"github.com/relab/gorums/internal/impl"
	"github.com/relab/gorums/internal/stream"
	"github.com/relab/gorums/internal/testutils/mock"
	"github.com/relab/gorums/internal/testutils/servers"
	"go.uber.org/goleak"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

// This file tests unexported gorums internals (withServer, srv.im, ...)
// directly, so it must stay in package gorums rather than an external
// gorums_test package. That means it cannot import the gorumstest package built
// on top of the root gorums package: doing so would create an import cycle
// (building package gorums's tests would require gorumstest, which requires
// gorums). The helpers below build the server and dial-option setup this file
// needs directly on top of the gorums-independent internal/testutils/servers
// package instead of gorumstest.Servers, gorumstest.DialOptions,
// gorumstest.Closer, and gorumstest.Context, so this file has no dependency on
// gorumstest.

// testStartServers starts numServers servers via srvFn and stops them, and
// verifies no goroutines were leaked, when the test finishes.
func testStartServers(t testing.TB, numServers int, srvFn func(i int) ServerIface) []string {
	t.Helper()
	if _, ok := t.(*testing.B); !ok {
		t.Cleanup(func() { goleak.VerifyNone(t) })
	}
	addrs, stopFn := servers.Start(t, numServers, func(i int) servers.ServerIface { return srvFn(i) })
	t.Cleanup(func() { stopFn() })
	return addrs
}

// testDialOptions returns a DialOption for connecting to servers started by
// testStartServers.
func testDialOptions(t testing.TB) DialOption {
	return WithGRPCDialOptions(servers.DialOptions(t)...)
}

// testCloser returns a cleanup function that closes the given io.Closer.
func testCloser(t testing.TB, c io.Closer) func() {
	t.Helper()
	return func() {
		if err := c.Close(); err != nil {
			t.Errorf("c.Close() = %q, expected no error", err.Error())
		}
	}
}

// testTimeoutContext creates a context with timeout, using t.Context() as the
// parent, that automatically cancels on cleanup.
func testTimeoutContext(t testing.TB, timeout time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	t.Cleanup(cancel)
	return ctx
}

// mockBidiStream is a minimal stream.BidiStream for testing InboundManager.
// Recv blocks until a message is sent or the stream is closed.
type mockBidiStream struct {
	ch chan *stream.Message
}

func newMockBidiStream() *mockBidiStream {
	return &mockBidiStream{ch: make(chan *stream.Message, 10)}
}

func (m *mockBidiStream) close() { close(m.ch) }

func (*mockBidiStream) Send(*stream.Message) error { return nil }
func (m *mockBidiStream) Recv() (*stream.Message, error) {
	msg, ok := <-m.ch
	if !ok {
		return nil, io.EOF
	}
	return msg, nil
}

// testNode is a minimal NodeSource for use in tests.
type testNode struct {
	addr string
}

func (n testNode) Addr() string { return n.addr }

// inboundCtx returns a context carrying nodeID metadata, rooted at parent.
func inboundCtx(parent context.Context, id uint32) context.Context {
	return metadata.NewIncomingContext(parent, conn.MetadataWithNodeID(id))
}

// checkIDs asserts that cfg.NodeIDs() equals wantIDs, reporting label in any
// failure message.
func checkIDs(t *testing.T, cfg Config, wantIDs []uint32, label string) {
	t.Helper()
	if got := cfg.NodeIDs(); !slices.Equal(got, wantIDs) {
		t.Errorf("%s: config IDs = %v; want %v", label, got, wantIDs)
	}
}

// inboundPeers returns the server's inbound peer [Config]: the known peers with
// an inbound stream open to this server, plus the local node. Test-only.
func inboundPeers(srv *Server) Config {
	return srv.im.InboundPeers()
}

// mustWaitForInbound blocks until cond returns true for srv's inbound peer
// Config, or fails the test after a 2-second timeout.
func mustWaitForInbound(t *testing.T, srv *Server, cond func(Config) bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	if err := srv.im.WaitForInbound(ctx, cond); err != nil {
		t.Fatalf("WaitForInbound: %v", err)
	}
}

// mustWaitForClients blocks until cond returns true for srv's client-peer
// Config, or fails the test after a 2-second timeout.
func mustWaitForClients(t *testing.T, srv *Server, cond func(Config) bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	if err := srv.WaitForClients(ctx, cond); err != nil {
		t.Fatalf("WaitForClients: %v", err)
	}
}

// testPeerServer creates a Server with WithPeers(1, peerNodes()), starts it
// via testStartServers, and returns the server and its addresses.
func testPeerServer(t *testing.T) (*Server, []string) {
	t.Helper()
	insecureDialOpts := WithGRPCDialOptions(grpc.WithTransportCredentials(insecure.NewCredentials()))
	var srv *Server
	addrs := testStartServers(t, 1, func(_ int) ServerIface {
		srv = NewServer(WithPeers(1, peerNodes(), insecureDialOpts))
		return srv
	})
	return srv, addrs
}

func equalNodeIDs(ids []uint32) func(Config) bool {
	return func(cfg Config) bool {
		return slices.Equal(cfg.NodeIDs(), ids)
	}
}

// peerNodes creates the peer NodeSource used by the E2E tests.
func peerNodes() NodeSource {
	return WithNodes(map[uint32]testNode{
		1: {"127.0.0.1:9001"},
		2: {"127.0.0.1:9002"},
	})
}

// connectAsPeer creates a Config that identifies itself as peerID by sending
// gorums-node-id metadata, connects to addrs, and returns the configuration.
// Config cleanup is registered via t.Cleanup; callers may also close it
// explicitly (e.g., to test disconnect) — Close is idempotent.
func connectAsPeer(t *testing.T, peerID uint32, addrs []string) Config {
	t.Helper()
	peerMD := conn.MetadataWithNodeID(peerID)
	cfg, err := NewConfig(WithNodeList(addrs), testDialOptions(t), WithMetadata(peerMD))
	if err != nil {
		t.Fatalf("NewConfig() error: %v", err)
	}
	t.Cleanup(testCloser(t, cfg))
	return cfg
}

// TestConfigurationExtendUsesKnownDedupPeer verifies that extending a dedup
// configuration with a lower-ID peer from the server's peer configuration
// yields a born-shared node whether or not that peer is currently connected:
// the node borrows the peer's inbound channel slot and never dials. A
// connected peer backs the shared node with its live inbound stream; a
// disconnected peer leaves it without a channel until the peer connects.
func TestConfigurationExtendUsesKnownDedupPeer(t *testing.T) {
	peers := map[uint32]testNode{
		1: {"127.0.0.1:9081"},
		2: {"127.0.0.1:9082"},
	}
	tests := []struct {
		name      string
		connected bool
	}{
		{name: "ConnectedInbound", connected: true},
		{name: "DisconnectedInbound"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			insecureDialOpts := WithGRPCDialOptions(grpc.WithTransportCredentials(insecure.NewCredentials()))
			srv := NewServer(WithPeers(2, WithNodes(peers), insecureDialOpts), WithStreamDedup())
			t.Cleanup(srv.Stop)
			if tt.connected {
				peerStream := newMockBidiStream()
				t.Cleanup(peerStream.close)
				_, cleanup, err := srv.im.AcceptPeer(inboundCtx(t.Context(), 1), peerStream)
				if err != nil {
					t.Fatalf("AcceptPeer: %v", err)
				}
				t.Cleanup(cleanup)
			}

			initial, err := NewConfig(WithNodes(map[uint32]testNode{2: peers[2]}), insecureDialOpts, withServer(srv), conn.WithStreamDedup())
			if err != nil {
				t.Fatalf("initial configuration: %v", err)
			}
			t.Cleanup(testCloser(t, initial))

			extended, err := initial.Extend(WithNodes(map[uint32]testNode{1: peers[1]}))
			if err != nil {
				t.Fatalf("Extend: %v", err)
			}
			var added *Node
			for _, node := range extended {
				if node.ID() == 1 {
					added = node
					break
				}
			}
			if added == nil {
				t.Fatal("extended configuration does not contain node 1")
			}
			if !added.IsShared() {
				t.Fatal("node 1 IsShared = false, want born-shared node for known lower-ID peer")
			}
			if added.IsOutbound() {
				t.Fatal("born-shared node created a redundant outbound channel")
			}
			if got := added.IsInbound(); got != tt.connected {
				t.Fatalf("node 1 IsInbound = %t, want %t", got, tt.connected)
			}
		})
	}
}

// TestStreamDedupBorrowValidatesPeerAddress verifies that building a
// deduplicated outbound node fails when the lower-ID node it would borrow from
// is not a configured peer, or is a configured peer at a different address.
// Without this check a dedup node could silently carry its calls onto a peer
// channel that reaches a different process.
func TestStreamDedupBorrowValidatesPeerAddress(t *testing.T) {
	peers := map[uint32]testNode{
		2: {"127.0.0.1:9082"},
		3: {"127.0.0.1:9083"}, // self (localID 3)
	}
	insecureDialOpts := WithGRPCDialOptions(grpc.WithTransportCredentials(insecure.NewCredentials()))
	srv := NewServer(WithPeers(3, WithNodes(peers), insecureDialOpts), WithStreamDedup())
	t.Cleanup(srv.Stop)

	tests := []struct {
		name    string
		nodes   map[uint32]testNode
		wantErr string
	}{
		{
			name:    "AddressMismatch",
			nodes:   map[uint32]testNode{2: {"127.0.0.1:9999"}},
			wantErr: "does not match peer address",
		},
		{
			name:    "MissingPeer",
			nodes:   map[uint32]testNode{1: {"127.0.0.1:9081"}},
			wantErr: "is not a configured peer",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := NewConfig(WithNodes(tt.nodes), insecureDialOpts, withServer(srv), conn.WithStreamDedup())
			if err == nil {
				t.Cleanup(testCloser(t, cfg))
				t.Fatalf("newConfig succeeded; want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("newConfig error = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

// TestSelfNodeIDStreamRejectedEndToEnd verifies that a client presenting the
// server's own node ID has its stream rejected end to end: the RPC terminates,
// the server never dispatches an application handler for it, and the server's
// in-process self-node channel is left intact.
func TestSelfNodeIDStreamRejectedEndToEnd(t *testing.T) {
	insecureDialOpts := WithGRPCDialOptions(grpc.WithTransportCredentials(insecure.NewCredentials()))
	var handlerCalls atomic.Int32
	var srv *Server
	addrs := testStartServers(t, 1, func(_ int) ServerIface {
		srv = NewServer(WithPeers(1, peerNodes(), insecureDialOpts))
		srv.RegisterHandler(mock.TestMethod, func(_ ServerContext, in *Message) (*Message, error) {
			handlerCalls.Add(1)
			req := AsProto[*pb.StringValue](in)
			return NewResponseMessage(in, pb.String("echo: "+req.GetValue())), nil
		})
		return srv
	})

	// The client claims the server's own node ID (1). Its stream is rejected,
	// so the call cannot complete.
	cfg := connectAsPeer(t, 1, addrs)
	node := cfg.Nodes()[0]

	// Bound the request itself so it fails fast; wait for that failure on a
	// separate, longer deadline so the two do not race in the select.
	reqCtx := testTimeoutContext(t, 500*time.Millisecond)
	reqMsg, err := stream.NewMessage(reqCtx, conn.NodeTransport(node).NextMsgID(), mock.TestMethod, pb.String("hello"))
	if err != nil {
		t.Fatalf("NewMessage() error: %v", err)
	}
	reply := make(chan NodeResponse[*stream.Message], 1)
	conn.NodeTransport(node).Enqueue(stream.Request{Ctx: reqCtx, Msg: reqMsg, ResponseChan: reply})

	select {
	case resp := <-reply:
		if resp.Err == nil {
			t.Fatal("call over a self-ID stream succeeded; want failure")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("call over a self-ID stream neither failed nor completed")
	}

	if got := handlerCalls.Load(); got != 0 {
		t.Errorf("server dispatched %d handler calls for a rejected self-ID stream; want 0", got)
	}

	// The server's self-node must still dispatch in-process: rejecting the
	// network stream must not have displaced its local channel.
	var self *Node
	for _, n := range srv.im.Nodes() {
		if n.ID() == 1 {
			self = n
			break
		}
	}
	if self == nil {
		t.Fatal("self-node missing from known peers")
	}
	if self.IsInbound() || self.IsOutbound() {
		t.Fatalf("self-node channel is no longer local (inbound=%t outbound=%t)", self.IsInbound(), self.IsOutbound())
	}
	if ch := conn.NodeTransport(self).LoadChannel(); ch == nil || !ch.StreamUp() {
		t.Fatal("self-node channel is not up; local dispatch broken")
	}
}

// TestKnownPeerConnects verifies the end-to-end path:
// a client that sends gorums-node-id metadata connects to a gorums Server
// with WithPeers; NodeStream calls AcceptPeer; the peer appears in
// Config alongside the self-node.
func TestKnownPeerConnects(t *testing.T) {
	srv, addrs := testPeerServer(t)

	checkIDs(t, inboundPeers(srv), []uint32{1}, "before connect")

	connectAsPeer(t, 2, addrs)

	mustWaitForInbound(t, srv, equalNodeIDs([]uint32{1, 2}))
	checkIDs(t, inboundPeers(srv), []uint32{1, 2}, "after connect")
}

// TestKnownPeerDisconnects verifies that when a peer closes its
// connection the cleanup deferred in NodeStream fires and removes the peer
// from Config.
func TestKnownPeerDisconnects(t *testing.T) {
	srv, addrs := testPeerServer(t)

	cfg := connectAsPeer(t, 2, addrs)
	mustWaitForInbound(t, srv, equalNodeIDs([]uint32{1, 2}))

	// Close the configuration to trigger disconnect; Close is idempotent so
	// t.Cleanup (registered by connectAsPeer) is harmless.
	if err := cfg.Close(); err != nil {
		t.Fatalf("cfg.Close() error: %v", err)
	}
	mustWaitForInbound(t, srv, equalNodeIDs([]uint32{1}))
	checkIDs(t, inboundPeers(srv), []uint32{1}, "after disconnect")
}

// TestUnknownPeerIgnored verifies that a client sending an
// unknown or zero node ID does not affect Config.
func TestUnknownPeerIgnored(t *testing.T) {
	srv, addrs := testPeerServer(t)

	// Connect without metadata (external client) and with an unknown ID.
	cfg, err := NewConfig(WithNodeList(addrs), testDialOptions(t))
	if err != nil {
		t.Fatalf("NewConfig() error: %v", err)
	}
	t.Cleanup(testCloser(t, cfg))

	connectAsPeer(t, 99, addrs) // ID 99 not in known set

	// Give the server time to process both connections.
	time.Sleep(50 * time.Millisecond)
	checkIDs(t, inboundPeers(srv), []uint32{1}, "external and unknown peers must not appear")
}

// TestKnownPeerServerCallsClient verifies the full symmetric communication path:
// server sends a request to a connected client via an inbound channel,
// the client's Channel.receiver dispatches to a registered handler,
// the handler responds, and the server receives the response via RouteResponse.
func TestKnownPeerServerCallsClient(t *testing.T) {
	srv, addrs := testPeerServer(t)

	// Client connects as peer 2 with handlers registered on a server via WithServer.
	clientSrv := NewServer()
	clientSrv.RegisterHandler(mock.TestMethod, func(_ ServerContext, in *Message) (*Message, error) {
		req := AsProto[*pb.StringValue](in)
		return NewResponseMessage(in, pb.String("echo: "+req.GetValue())), nil
	})
	peerMD := conn.MetadataWithNodeID(2)
	cfg, err := NewConfig(WithNodeList(addrs), testDialOptions(t), WithMetadata(peerMD), withServer(clientSrv))
	if err != nil {
		t.Fatalf("NewConfig() error: %v", err)
	}
	t.Cleanup(testCloser(t, cfg))

	// Wait for the peer to appear in the inbound config.
	mustWaitForInbound(t, srv, equalNodeIDs([]uint32{1, 2}))

	// Server sends a request to the client via the inbound node.
	inboundCfg := inboundPeers(srv)
	var pNode *Node
	for _, n := range inboundCfg.Nodes() {
		if n.ID() == 2 {
			pNode = n
			break
		}
	}
	if pNode == nil {
		t.Fatal("peer node 2 not found in inbound config")
	}

	// Create request message and register it for response routing.
	ctx := testTimeoutContext(t, 5*time.Second)
	reqMsg, err := stream.NewMessage(ctx, conn.NodeTransport(pNode).NextMsgID(), mock.TestMethod, pb.String("hello"))
	if err != nil {
		t.Fatalf("NewMessage() error: %v", err)
	}
	replyChan := make(chan NodeResponse[*stream.Message], 1)
	conn.NodeTransport(pNode).Router().Register(reqMsg.GetMessageSeqNo(), stream.Request{
		Ctx:          ctx,
		Msg:          reqMsg,
		ResponseChan: replyChan,
	})

	// Send the request through the inbound channel.
	conn.NodeTransport(pNode).Enqueue(stream.Request{Ctx: ctx, Msg: reqMsg})

	// Wait for the response from the client handler.
	select {
	case resp := <-replyChan:
		if resp.Err != nil {
			t.Fatalf("response error: %v", resp.Err)
		}
		sv := &pb.StringValue{}
		if err := proto.Unmarshal(resp.Value.GetPayload(), sv); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}
		if got, want := sv.GetValue(), "echo: hello"; got != want {
			t.Errorf("response = %q; want %q", got, want)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for response")
	}
}

// testClientServer creates a Server, starts it via testStartServers, and
// returns the server and its addresses. The server automatically accepts
// anonymous clients.
func testClientServer(t *testing.T) (*Server, []string) {
	t.Helper()
	var srv *Server
	addrs := testStartServers(t, 1, func(_ int) ServerIface {
		srv = NewServer()
		return srv
	})
	return srv, addrs
}

// connectAsPeerClient creates a Config that advertises back-channel
// capability by sending the gorums-node-id key (via [WithServer]),
// connects to addrs, and returns the configuration. The server will include it in
// ConnectedClients and may dispatch server-initiated calls to it.
func connectAsPeerClient(t *testing.T, addrs []string) Config {
	t.Helper()
	cfg, err := NewConfig(WithNodeList(addrs), testDialOptions(t), withServer(NewServer()))
	if err != nil {
		t.Fatalf("NewConfig() error: %v", err)
	}
	t.Cleanup(testCloser(t, cfg))
	return cfg
}

// TestConnectedClientsConnects verifies that a server accepts a peer-capable
// client (with gorums-node-id: 0) and includes it in ConnectedClients.
func TestConnectedClientsConnects(t *testing.T) {
	srv, addrs := testClientServer(t)

	// Initially no peers (no self-node since myID == 0)
	checkIDs(t, srv.ConnectedClients(), []uint32{}, "before connect")

	connectAsPeerClient(t, addrs)

	// Client peer should appear with auto-assigned ID >= clientIDStart.
	mustWaitForClients(t, srv, func(cfg Config) bool { return len(cfg) > 0 })
	cfg := srv.ConnectedClients()
	if len(cfg) != 1 {
		t.Fatalf("ConnectedClients has %d nodes; want 1", len(cfg))
	}
	if cfg[0].ID() < conn.ClientIDStart {
		t.Errorf("Client peer ID = %d; want >= %d", cfg[0].ID(), conn.ClientIDStart)
	}
}

// TestConnectedClientsDisconnects verifies that client peers are removed from
// ConnectedClients and the dynamic client map when they disconnect.
func TestConnectedClientsDisconnects(t *testing.T) {
	srv, addrs := testClientServer(t)

	cfg := connectAsPeerClient(t, addrs)

	// Wait for the client peer to appear.
	mustWaitForClients(t, srv, func(cfg Config) bool { return len(cfg) > 0 })
	if len(srv.ConnectedClients()) != 1 {
		t.Fatalf("ConnectedClients has %d nodes; want 1", len(srv.ConnectedClients()))
	}

	// Disconnect the client peer.
	if err := cfg.Close(); err != nil {
		t.Fatalf("cfg.Close() error: %v", err)
	}

	// Wait for config to become empty.
	mustWaitForClients(t, srv, func(cfg Config) bool { return len(cfg) == 0 })
	checkIDs(t, srv.ConnectedClients(), []uint32{}, "after disconnect")
}

// TestConnectedClientsMixedMode verifies that a server with both WithPeers and
// WithConnectedClients accepts known peers by ID and unknown clients dynamically.
func TestConnectedClientsMixedMode(t *testing.T) {
	srv, addrs := testPeerServer(t)

	// Self-node (ID 1) is present initially.
	checkIDs(t, inboundPeers(srv), []uint32{1}, "before connect")

	// Connect known peer (ID 2).
	connectAsPeer(t, 2, addrs)
	mustWaitForInbound(t, srv, equalNodeIDs([]uint32{1, 2}))

	// Connect peer-capable anonymous client (dynamic peer).
	connectAsPeerClient(t, addrs)

	// Wait for 1 dynamic node.
	mustWaitForClients(t, srv, func(cfg Config) bool { return len(cfg) == 1 })
	dynCfg := srv.ConnectedClients()
	if len(dynCfg) != 1 {
		t.Fatalf("ConnectedClients has %d nodes; want 1", len(dynCfg))
	}
	if dynCfg[0].ID() < conn.ClientIDStart {
		t.Errorf("Client peer ID = %d; want >= %d", dynCfg[0].ID(), conn.ClientIDStart)
	}
	cfg := inboundPeers(srv)
	if len(cfg) != 2 {
		t.Fatalf("Config has %d nodes; want 2", len(cfg))
	}
	ids := cfg.NodeIDs()
	if ids[0] != 1 || ids[1] != 2 {
		t.Errorf("known IDs = %v; want [1, 2]", ids[:2])
	}
}

// TestConnectedClientsServerCallsClient verifies that a server dispatches a back-channel
// multicast to a connected client via [ServerContext.ConnectedClients].
func TestConnectedClientsServerCallsClient(t *testing.T) {
	// Register the server handler before starting so it is present before clients arrive.
	srv := NewServer()
	srv.RegisterHandler(mock.TestMethod, func(ctx ServerContext, _ *Message) (*Message, error) {
		if cfg := ctx.ConnectedClients(); len(cfg) > 0 {
			// Release before the back-channel send: Wait blocks until every
			// client's send completes, and holding the dispatch lock across
			// that wait would stop this connection from reading further
			// inbound frames.
			ctx.Release()
			if err := impl.Multicast(cfg.Context(ctx), pb.String("ping"), mock.Stream).Send(); err != nil {
				t.Errorf("back-channel Multicast: %v", err)
			}
		}
		return nil, nil // one-way
	})
	addrs := testStartServers(t, 1, func(_ int) ServerIface { return srv })

	var wg sync.WaitGroup
	wg.Add(1)

	// Client: a Server whose back-channel mock.Stream handler is wired in via withServer.
	clientSrv := NewServer()
	clientSrv.RegisterHandler(mock.Stream, func(_ ServerContext, _ *Message) (*Message, error) {
		wg.Done()
		return nil, nil
	})
	clientConfig, err := NewConfig(WithNodeList(addrs), testDialOptions(t), withServer(clientSrv))
	if err != nil {
		t.Fatalf("NewConfig() error: %v", err)
	}
	t.Cleanup(testCloser(t, clientConfig))

	// Wait for the client to appear in the server's ConnectedClients.
	mustWaitForClients(t, srv, func(cfg Config) bool { return len(cfg) > 0 })

	// Trigger: client multicasts TestMethod to the server; server fans it back via ConnectedClients.
	ctx := testTimeoutContext(t, 2*time.Second)
	if err := impl.Multicast(clientConfig.Context(ctx), pb.String("trigger"), mock.TestMethod).Send(); err != nil {
		t.Fatalf("Multicast error: %v", err)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("timed out waiting for back-channel handler")
	}
}
