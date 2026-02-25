package gorums

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/relab/gorums/internal/stream"
	"github.com/relab/gorums/internal/testutils/mock"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

// inboundTestNode is a minimal NodeAddress for use in InboundManager tests.
type inboundTestNode struct {
	addr string
}

func (n inboundTestNode) Addr() string { return n.addr }

// Compile-time assertions: both node providers satisfy NodeListOption.
var (
	_ NodeListOption = nodeMap[inboundTestNode](nil)
	_ NodeListOption = nodeList(nil)
)

// mockBidiStream is a minimal stream.BidiStream for testing InboundManager.
// Recv blocks until a message is sent or the stream is closed.
type mockBidiStream struct {
	ch chan *stream.Message
}

func newMockBidiStream() *mockBidiStream {
	return &mockBidiStream{ch: make(chan *stream.Message, 10)}
}

func (m *mockBidiStream) close() { close(m.ch) }

func (m *mockBidiStream) Send(*stream.Message) error { return nil }
func (m *mockBidiStream) Recv() (*stream.Message, error) {
	msg, ok := <-m.ch
	if !ok {
		return nil, io.EOF
	}
	return msg, nil
}

// shouldPanic asserts that fn panics with a message containing wantSubstr.
func shouldPanic(t *testing.T, wantSubstr string, fn func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected panic containing %q; got no panic", wantSubstr)
		}
		msg := fmt.Sprint(r)
		if !strings.Contains(msg, wantSubstr) {
			t.Fatalf("panic = %q; want it to contain %q", msg, wantSubstr)
		}
	}()
	fn()
}

// newTestInboundManager creates an InboundManager with myID and three known peers.
func newTestInboundManager(t *testing.T, myID uint32) *InboundManager {
	t.Helper()
	im := newInboundManager(myID, WithNodes(map[uint32]inboundTestNode{
		1: {addr: "127.0.0.1:9081"},
		2: {addr: "127.0.0.1:9082"},
		3: {addr: "127.0.0.1:9083"},
	}), 0, nil, false)
	return im
}

func TestNewInboundManager(t *testing.T) {
	tests := []struct {
		name       string
		opt        NodeListOption
		wantIDs    []uint32
		wantCfgIDs []uint32 // expected InboundConfig IDs after construction
		wantPanic  string   // if non-empty, expect panic containing this substring
	}{
		{
			name: "ValidNodes",
			opt: WithNodes(map[uint32]inboundTestNode{
				1: {addr: "127.0.0.1:9081"},
				2: {addr: "127.0.0.1:9082"},
				3: {addr: "127.0.0.1:9083"},
			}),
			wantIDs:    []uint32{1, 2, 3},
			wantCfgIDs: []uint32{1}, // only self-node until peers connect
		},
		{
			name:      "EmptyMapRejected",
			opt:       WithNodes(map[uint32]inboundTestNode{}),
			wantPanic: "missing required node map",
		},
		{
			name: "NodeZeroRejected",
			opt: WithNodes(map[uint32]inboundTestNode{
				0: {addr: "127.0.0.1:9080"},
				1: {addr: "127.0.0.1:9081"},
			}),
			wantPanic: "node 0 is reserved",
		},
		{
			name: "DuplicateAddressRejected",
			opt: WithNodes(map[uint32]inboundTestNode{
				1: {addr: "127.0.0.1:9081"},
				2: {addr: "127.0.0.1:9081"}, // same address as ID 1
			}),
			wantPanic: "already in use by node",
		},
		{
			name: "InvalidAddressRejected",
			opt: WithNodes(map[uint32]inboundTestNode{
				1: {addr: "not-an-address"},
			}),
			wantPanic: "invalid address",
		},
		{
			// WithNodeList assigns IDs starting at 1.
			name:       "WithNodeListAssignsIDs",
			opt:        WithNodeList([]string{"127.0.0.1:9081", "127.0.0.1:9082", "127.0.0.1:9083"}),
			wantIDs:    []uint32{1, 2, 3},
			wantCfgIDs: []uint32{1}, // only self-node until peers connect
		},
		{
			name:      "WithNodeListEmptyRejected",
			opt:       WithNodeList([]string{}),
			wantPanic: "missing required node addresses",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.wantPanic != "" {
				shouldPanic(t, tc.wantPanic, func() {
					newInboundManager(1, tc.opt, 0, nil, false)
				})
				return
			}
			im := newInboundManager(1, tc.opt, 0, nil, false)
			if got := im.KnownIDs(); !slices.Equal(got, tc.wantIDs) {
				t.Errorf("KnownIDs() = %v; want %v", got, tc.wantIDs)
			}
			if got := im.InboundConfig().NodeIDs(); !slices.Equal(got, tc.wantCfgIDs) {
				t.Errorf("InboundConfig().NodeIDs() = %v; want %v", got, tc.wantCfgIDs)
			}
		})
	}
}

// inboundCtx returns a context carrying nodeID metadata, rooted at parent.
func inboundCtx(parent context.Context, id uint32) context.Context {
	md := metadata.Pairs(gorumsNodeIDKey, strconv.FormatUint(uint64(id), 10))
	return metadata.NewIncomingContext(parent, md)
}

// nodeIDCtx builds a context carrying incoming gorums-node-id metadata.
func nodeIDCtx(id string) context.Context {
	md := metadata.Pairs(gorumsNodeIDKey, id)
	return metadata.NewIncomingContext(context.Background(), md)
}

func TestNodeID(t *testing.T) {
	tests := []struct {
		name   string
		ctx    context.Context
		wantID uint32
	}{
		{name: "ValidID", ctx: nodeIDCtx("3"), wantID: 3},
		{name: "ValidIDOne", ctx: nodeIDCtx("1"), wantID: 1},
		{name: "ValidIDLarge", ctx: nodeIDCtx("4294967295"), wantID: 4294967295}, // max uint32
		{name: "ExternalClientNoMeta", ctx: context.Background(), wantID: 0},
		{name: "ReservedIDZero", ctx: nodeIDCtx("0"), wantID: 0},
		{name: "NegativeValue", ctx: nodeIDCtx("-1"), wantID: 0},
		{name: "NonNumericValue", ctx: nodeIDCtx("abc"), wantID: 0},
		{name: "Overflow", ctx: nodeIDCtx("4294967296"), wantID: 0}, // max uint32 + 1
		{name: "EmptyString", ctx: nodeIDCtx(""), wantID: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := nodeID(tc.ctx); got != tc.wantID {
				t.Errorf("nodeID() = %d; want %d", got, tc.wantID)
			}
		})
	}
}

// checkIDs asserts that cfg.NodeIDs() equals wantIDs, reporting label in any
// failure message.
func checkIDs(t *testing.T, cfg Configuration, wantIDs []uint32, label string) {
	t.Helper()
	if got := cfg.NodeIDs(); !slices.Equal(got, wantIDs) {
		t.Errorf("%s: config IDs = %v; want %v", label, got, wantIDs)
	}
}

// TestInboundManagerRegisterUnregister checks that the Configuration is
// correctly updated through sequences of peer registrations and unregistrations,
// including out-of-order registration (sorted config), stream breakage followed
// by reconnect, and idempotent unregister calls.
//
// The self-node (myID=1) is always present after construction,
// and is included in every wantIDs slice.
func TestInboundManagerRegisterUnregister(t *testing.T) {
	type configStep struct {
		op      string   // "register" or "unregister"
		id      uint32   // peer ID
		wantIDs []uint32 // expected config IDs after this step
	}

	tests := []struct {
		name  string
		steps []configStep
	}{
		{
			name: "RegisterAndUnregister",
			steps: []configStep{
				{op: "register", id: 2, wantIDs: []uint32{1, 2}},
				{op: "unregister", id: 2, wantIDs: []uint32{1}},
			},
		},
		{
			name: "RegisterAllPeers",
			steps: []configStep{
				{op: "register", id: 2, wantIDs: []uint32{1, 2}},
				{op: "register", id: 3, wantIDs: []uint32{1, 2, 3}},
				{op: "unregister", id: 2, wantIDs: []uint32{1, 3}},
				{op: "unregister", id: 3, wantIDs: []uint32{1}},
			},
		},
		{
			// Peers connect in reverse order; config must always be sorted.
			name: "RegisterOutOfOrderSorted",
			steps: []configStep{
				{op: "register", id: 3, wantIDs: []uint32{1, 3}},
				{op: "register", id: 2, wantIDs: []uint32{1, 2, 3}},
				{op: "unregister", id: 2, wantIDs: []uint32{1, 3}},
				{op: "unregister", id: 3, wantIDs: []uint32{1}},
			},
		},
		{
			// Simulates a stream breaking and the peer reconnecting.
			name: "StreamBreakageAndReconnect",
			steps: []configStep{
				{op: "register", id: 2, wantIDs: []uint32{1, 2}},
				{op: "unregister", id: 2, wantIDs: []uint32{1}},  // stream broken
				{op: "register", id: 2, wantIDs: []uint32{1, 2}}, // peer reconnects
				{op: "unregister", id: 2, wantIDs: []uint32{1}},
			},
		},
		{
			// Calling UnregisterPeer multiple times must be a no-op
			// after the first invocation (detachStream is idempotent).
			name: "IdempotentUnregister",
			steps: []configStep{
				{op: "register", id: 3, wantIDs: []uint32{1, 3}},
				{op: "unregister", id: 3, wantIDs: []uint32{1}},
				{op: "unregister", id: 3, wantIDs: []uint32{1}}, // second call: no-op
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			im := newTestInboundManager(t, 1)
			checkIDs(t, im.InboundConfig(), []uint32{1}, "initial")

			for i, s := range tc.steps {
				switch s.op {
				case "register":
					inStream := newMockBidiStream()
					t.Cleanup(inStream.close)
					im.registerPeer(s.id, inStream, t.Context())
				case "unregister":
					im.unregisterPeer(s.id)
				default:
					t.Fatalf("unknown op %q in step %d", s.op, i)
				}
				checkIDs(t, im.InboundConfig(), s.wantIDs, fmt.Sprintf("step %d (%s id=%d)", i, s.op, s.id))
			}
		})
	}
}

func TestAcceptPeer(t *testing.T) {
	im := newTestInboundManager(t, 1)

	tests := []struct {
		name     string
		ctx      context.Context
		wantNode bool
	}{
		{
			name:     "ExternalClientNoMetadata",
			ctx:      t.Context(), // no gorums-node-id metadata
			wantNode: false,
		},
		{
			name:     "UnknownPeerID",
			ctx:      inboundCtx(t.Context(), 99), // not in configured set
			wantNode: false,
		},
		{
			name:     "KnownPeer",
			ctx:      inboundCtx(t.Context(), 2),
			wantNode: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inStream := newMockBidiStream()
			defer inStream.close()
			node, cleanup, err := im.AcceptPeer(tc.ctx, inStream, nil)
			if err != nil {
				t.Fatalf("AcceptPeer() unexpected error: %v", err)
			}
			if (node != nil) != tc.wantNode {
				t.Errorf("AcceptPeer() node non-nil = %v; want %v", node != nil, tc.wantNode)
			}
			if node != nil {
				cleanup()
			}
		})
	}
}

func TestAcceptPeerReturnsNode(t *testing.T) {
	im := newTestInboundManager(t, 1)
	inStream := newMockBidiStream()
	defer inStream.close()

	node, cleanup, err := im.AcceptPeer(inboundCtx(t.Context(), 3), inStream, nil)
	if err != nil {
		t.Fatalf("AcceptPeer() unexpected error: %v", err)
	}
	if node == nil {
		t.Fatal("AcceptPeer() node = nil; want non-nil for known peer")
	}
	checkIDs(t, im.InboundConfig(), []uint32{1, 3}, "after AcceptPeer")

	cleanup()
	checkIDs(t, im.InboundConfig(), []uint32{1}, "after cleanup")

	// Calling cleanup again must be a no-op (idempotent detachStream).
	cleanup()
	checkIDs(t, im.InboundConfig(), []uint32{1}, "after second cleanup")
}

// TestRegisterPeerReplacesExisting verifies that calling registerPeer for a
// peer that already has an active channel atomically replaces the old channel
// with the new one. This is the correct behavior for reconnects: the old
// (potentially broken) stream is closed and the new one takes over.
func TestRegisterPeerReplacesExisting(t *testing.T) {
	im := newTestInboundManager(t, 1)

	// First registration for peer 3.
	first := newMockBidiStream()
	t.Cleanup(first.close)
	im.registerPeer(3, first, t.Context())
	checkIDs(t, im.InboundConfig(), []uint32{1, 3}, "after first register")

	// Peer 3 reconnects — second registerPeer must replace the first.
	second := newMockBidiStream()
	t.Cleanup(second.close)
	im.registerPeer(3, second, t.Context())

	checkIDs(t, im.InboundConfig(), []uint32{1, 3}, "after replacement")
	node := im.nodes[3]
	if ch := node.channel.Load(); ch == nil {
		t.Fatal("channel should not be nil after replacement")
	}
}

// testPeerServer creates a Server with WithPeers(1, peerNodes()), starts it
// via TestServers, and returns the server and its addresses.
func testPeerServer(t *testing.T) (*Server, []string) {
	t.Helper()
	var srv *Server
	addrs := TestServers(t, 1, func(_ int) ServerIface {
		srv = NewServer(WithPeers(1, peerNodes()))
		return srv
	})
	return srv, addrs
}

// waitForServerConfig polls srv.InboundConfig until the condition cond returns true
// or the timeout elapses.
func waitForServerConfig(t *testing.T, srv *Server, cond func(Configuration) bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond(srv.InboundConfig()) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Errorf("timeout waiting for config; got %v", srv.InboundConfig().NodeIDs())
}

func equalNodeIDs(ids []uint32) func(Configuration) bool {
	return func(cfg Configuration) bool {
		return slices.Equal(cfg.NodeIDs(), ids)
	}
}

// peerNodes creates the peer NodeListOption used by the E2E tests.
func peerNodes() NodeListOption {
	return WithNodes(map[uint32]inboundTestNode{
		1: {addr: "127.0.0.1:9001"},
		2: {addr: "127.0.0.1:9002"},
	})
}

// connectAsPeer creates a Manager that identifies itself as peerID by sending
// gorumsNodeIDKey metadata, connects to addrs, and returns the manager.
// Manager cleanup is registered via t.Cleanup; callers may also close it
// explicitly (e.g., to test disconnect) — Close is idempotent.
func connectAsPeer(t *testing.T, peerID uint32, addrs []string) *Manager {
	t.Helper()
	peerMD := metadata.Pairs(gorumsNodeIDKey, strconv.FormatUint(uint64(peerID), 10))
	mgr := TestManager(t, WithMetadata(peerMD))
	_, err := NewConfiguration(mgr, WithNodeList(addrs))
	if err != nil {
		t.Fatalf("NewConfiguration() error: %v", err)
	}
	return mgr
}

// TestInboundManagerPeerConnects verifies the end-to-end path:
// a client that sends gorums-node-id metadata connects to a gorums Server
// with WithPeers; NodeStream calls AcceptPeer; the peer appears in
// InboundConfig alongside the self-node.
func TestInboundManagerPeerConnects(t *testing.T) {
	srv, addrs := testPeerServer(t)

	checkIDs(t, srv.InboundConfig(), []uint32{1}, "before connect")

	connectAsPeer(t, 2, addrs)

	waitForServerConfig(t, srv, equalNodeIDs([]uint32{1, 2}))
	checkIDs(t, srv.InboundConfig(), []uint32{1, 2}, "after connect")
}

// TestInboundManagerPeerDisconnects verifies that when a peer closes its
// connection the cleanup deferred in NodeStream fires and removes the peer
// from InboundConfig.
func TestInboundManagerPeerDisconnects(t *testing.T) {
	srv, addrs := testPeerServer(t)

	mgr := connectAsPeer(t, 2, addrs)
	waitForServerConfig(t, srv, equalNodeIDs([]uint32{1, 2}))

	// Close the peer manager to trigger disconnect; Close is idempotent so
	// t.Cleanup (registered by connectAsPeer via TestManager) is harmless.
	if err := mgr.Close(); err != nil {
		t.Fatalf("mgr.Close() error: %v", err)
	}
	waitForServerConfig(t, srv, equalNodeIDs([]uint32{1}))
	checkIDs(t, srv.InboundConfig(), []uint32{1}, "after disconnect")
}

// TestInboundManagerUnknownPeerIgnored verifies that a client sending an
// unknown or zero node ID does not affect InboundConfig.
func TestInboundManagerUnknownPeerIgnored(t *testing.T) {
	srv, addrs := testPeerServer(t)

	// Connect without metadata (external client) and with an unknown ID.
	external := TestManager(t)
	_, err := NewConfiguration(external, WithNodeList(addrs))
	if err != nil {
		t.Fatalf("NewConfiguration() error: %v", err)
	}

	connectAsPeer(t, 99, addrs) // ID 99 not in known set

	// Give the server time to process both connections.
	time.Sleep(50 * time.Millisecond)
	checkIDs(t, srv.InboundConfig(), []uint32{1}, "external and unknown peers must not appear")
}

// TestServerCallsClient verifies the full symmetric communication path:
// server sends a request to a connected client via an inbound channel,
// the client's Channel.receiver dispatches to a registered handler,
// the handler responds, and the server receives the response via RouteResponse.
func TestServerCallsClient(t *testing.T) {
	srv, addrs := testPeerServer(t)

	// Client connects as peer 2 and registers a handler on its outbound nodes.
	clientHandlers := map[string]stream.Handler{
		mock.TestMethod: func(_ ServerCtx, in *Message) (*Message, error) {
			req := AsProto[*pb.StringValue](in)
			return NewResponseMessage(in, pb.String("echo: "+req.GetValue())), nil
		},
	}
	peerMD := metadata.Pairs(gorumsNodeIDKey, "2")
	mgr := TestManager(t, WithMetadata(peerMD))
	cfg, err := NewConfiguration(mgr, WithNodeList(addrs))
	if err != nil {
		t.Fatalf("NewConfiguration() error: %v", err)
	}
	// Set client-side handlers on all outbound nodes.
	for _, node := range cfg.Nodes() {
		node.setHandlers(clientHandlers)
	}

	// Wait for the peer to appear in the inbound config.
	waitForServerConfig(t, srv, equalNodeIDs([]uint32{1, 2}))

	// Server sends a request to the client via the inbound node.
	inboundCfg := srv.InboundConfig()
	var peerNode *Node
	for _, n := range inboundCfg.Nodes() {
		if n.ID() == 2 {
			peerNode = n
			break
		}
	}
	if peerNode == nil {
		t.Fatal("peer node 2 not found in inbound config")
	}

	// Create request message and register it for response routing.
	ctx := TestContext(t, 5*time.Second)
	reqMsg, err := stream.NewMessage(ctx, srv.inboundMgr.getMsgID(), mock.TestMethod, pb.String("hello"))
	if err != nil {
		t.Fatalf("NewMessage() error: %v", err)
	}
	replyChan := make(chan stream.NodeResponse[proto.Message], 1)
	peerNode.router.Register(reqMsg.GetMessageSeqNo(), stream.Request{
		Ctx:          ctx,
		Msg:          reqMsg,
		ResponseChan: replyChan,
	})

	// Send the request through the inbound channel.
	peerNode.Enqueue(stream.Request{Ctx: ctx, Msg: reqMsg})

	// Wait for the response from the client handler.
	select {
	case resp := <-replyChan:
		if resp.Err != nil {
			t.Fatalf("response error: %v", resp.Err)
		}
		sv, ok := resp.Value.(*pb.StringValue)
		if !ok {
			t.Fatalf("response type = %T; want *pb.StringValue", resp.Value)
		}
		if got, want := sv.GetValue(), "echo: hello"; got != want {
			t.Errorf("response = %q; want %q", got, want)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for response")
	}
}

// testDynamicServer creates a Server with WithDynamicPeers() only, starts it
// via TestServers, and returns the server and its addresses.
func testDynamicServer(t *testing.T) (*Server, []string) {
	t.Helper()
	var srv *Server
	addrs := TestServers(t, 1, func(_ int) ServerIface {
		srv = NewServer(WithDynamicPeers())
		return srv
	})
	return srv, addrs
}

// testMixedServer creates a Server with both WithPeers and WithDynamicPeers,
// starts it via TestServers, and returns the server and its addresses.
func testMixedServer(t *testing.T) (*Server, []string) {
	t.Helper()
	var srv *Server
	addrs := TestServers(t, 1, func(_ int) ServerIface {
		srv = NewServer(WithPeers(1, peerNodes()), WithDynamicPeers())
		return srv
	})
	return srv, addrs
}

// connectAsExternal creates a Manager without NodeID metadata (an unknown
// client), connects to addrs, and returns the manager.
func connectAsExternal(t *testing.T, addrs []string) *Manager {
	t.Helper()
	mgr := TestManager(t)
	_, err := NewConfiguration(mgr, WithNodeList(addrs))
	if err != nil {
		t.Fatalf("NewConfiguration() error: %v", err)
	}
	return mgr
}

// TestDynamicPeerConnects verifies that a server with WithDynamicPeers()
// accepts an unknown client and includes it in InboundConfig.
func TestDynamicPeerConnects(t *testing.T) {
	srv, addrs := testDynamicServer(t)

	// Initially no peers (no self-node since myID == 0)
	checkIDs(t, srv.InboundConfig(), []uint32{}, "before connect")

	connectAsExternal(t, addrs)

	// Dynamic peer should appear with auto-assigned ID >= dynamicIDStart.
	waitForServerConfig(t, srv, func(cfg Configuration) bool { return len(cfg) > 0 })
	cfg := srv.InboundConfig()
	if len(cfg) != 1 {
		t.Fatalf("InboundConfig has %d nodes; want 1", len(cfg))
	}
	if cfg[0].ID() < dynamicIDStart {
		t.Errorf("dynamic peer ID = %d; want >= %d", cfg[0].ID(), dynamicIDStart)
	}
}

// TestDynamicPeerDisconnects verifies that dynamic peers are removed from
// InboundConfig and the nodes map when they disconnect.
func TestDynamicPeerDisconnects(t *testing.T) {
	srv, addrs := testDynamicServer(t)

	mgr := connectAsExternal(t, addrs)

	// Wait for the dynamic peer to appear.
	waitForServerConfig(t, srv, func(cfg Configuration) bool { return len(cfg) > 0 })
	if len(srv.InboundConfig()) != 1 {
		t.Fatalf("InboundConfig has %d nodes; want 1", len(srv.InboundConfig()))
	}

	// Disconnect the dynamic peer.
	if err := mgr.Close(); err != nil {
		t.Fatalf("mgr.Close() error: %v", err)
	}

	// Wait for config to become empty.
	waitForServerConfig(t, srv, func(cfg Configuration) bool { return len(cfg) == 0 })
	checkIDs(t, srv.InboundConfig(), []uint32{}, "after disconnect")
}

// TestDynamicPeerMixedMode verifies that a server with both WithPeers and
// WithDynamicPeers accepts known peers by ID and unknown clients dynamically.
func TestDynamicPeerMixedMode(t *testing.T) {
	srv, addrs := testMixedServer(t)

	// Self-node (ID 1) is present initially.
	checkIDs(t, srv.InboundConfig(), []uint32{1}, "before connect")

	// Connect known peer (ID 2).
	connectAsPeer(t, 2, addrs)
	waitForServerConfig(t, srv, equalNodeIDs([]uint32{1, 2}))

	// Connect unknown client (dynamic peer).
	connectAsExternal(t, addrs)

	// Wait for 3 nodes.
	waitForServerConfig(t, srv, func(cfg Configuration) bool { return len(cfg) == 3 })
	cfg := srv.InboundConfig()
	if len(cfg) != 3 {
		t.Fatalf("InboundConfig has %d nodes; want 3", len(cfg))
	}
	// Known peer IDs 1 and 2 should be present; dynamic peer ID >= dynamicIDStart.
	ids := cfg.NodeIDs()
	if ids[0] != 1 || ids[1] != 2 {
		t.Errorf("first two IDs = %v; want [1, 2]", ids[:2])
	}
	if ids[2] < dynamicIDStart {
		t.Errorf("dynamic peer ID = %d; want >= %d", ids[2], dynamicIDStart)
	}
}

// TestDynamicPeerServerCallsClient verifies that a server can send a request
// to a dynamically connected client and receive a response.
func TestDynamicPeerServerCallsClient(t *testing.T) {
	srv, addrs := testDynamicServer(t)

	// Client connects and registers a handler.
	clientHandlers := map[string]stream.Handler{
		mock.TestMethod: func(_ ServerCtx, in *Message) (*Message, error) {
			req := AsProto[*pb.StringValue](in)
			return NewResponseMessage(in, pb.String("dynamic: "+req.GetValue())), nil
		},
	}
	mgr := TestManager(t)
	cfg, err := NewConfiguration(mgr, WithNodeList(addrs))
	if err != nil {
		t.Fatalf("NewConfiguration() error: %v", err)
	}
	for _, node := range cfg.Nodes() {
		node.setHandlers(clientHandlers)
	}

	// Wait for the dynamic peer to appear.
	waitForServerConfig(t, srv, func(cfg Configuration) bool { return len(cfg) > 0 })
	inboundCfg := srv.InboundConfig()
	if len(inboundCfg) != 1 {
		t.Fatalf("InboundConfig has %d nodes; want 1", len(inboundCfg))
	}
	peerNode := inboundCfg[0]

	// Create request and register for response routing.
	ctx := TestContext(t, 5*time.Second)
	reqMsg, err := stream.NewMessage(ctx, srv.inboundMgr.getMsgID(), mock.TestMethod, pb.String("hello"))
	if err != nil {
		t.Fatalf("NewMessage() error: %v", err)
	}
	replyChan := make(chan stream.NodeResponse[proto.Message], 1)
	peerNode.router.Register(reqMsg.GetMessageSeqNo(), stream.Request{
		Ctx:          ctx,
		Msg:          reqMsg,
		ResponseChan: replyChan,
	})

	// Send the request through the inbound channel.
	peerNode.Enqueue(stream.Request{Ctx: ctx, Msg: reqMsg})

	// Wait for the response.
	select {
	case resp := <-replyChan:
		if resp.Err != nil {
			t.Fatalf("response error: %v", resp.Err)
		}
		sv, ok := resp.Value.(*pb.StringValue)
		if !ok {
			t.Fatalf("response type = %T; want *pb.StringValue", resp.Value)
		}
		if got, want := sv.GetValue(), "dynamic: hello"; got != want {
			t.Errorf("response = %q; want %q", got, want)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for response")
	}
}
