package gorums

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/relab/gorums/internal/stream"
	"github.com/relab/gorums/internal/testutils/mock"
	"google.golang.org/grpc/metadata"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

// mockBidiStream is a minimal stream.BidiStream for testing inboundManager.
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

// newTestInboundManager creates an inboundManager with myID and three known peers.
func newTestInboundManager(t *testing.T, myID uint32) *inboundManager {
	t.Helper()
	im := newInboundManager(myID, WithNodes(map[uint32]testNode{
		1: {"127.0.0.1:9081"},
		2: {"127.0.0.1:9082"},
		3: {"127.0.0.1:9083"},
	}), 0, nil, nil)
	return im
}

func TestNewInboundManager(t *testing.T) {
	tests := []struct {
		name       string
		opt        NodeListOption
		wantIDs    []uint32
		wantCfgIDs []uint32 // expected Config IDs after construction
		wantPanic  string   // if non-empty, expect panic containing this substring
	}{
		{
			name: "ValidNodes",
			opt: WithNodes(map[uint32]testNode{
				1: {"127.0.0.1:9081"},
				2: {"127.0.0.1:9082"},
				3: {"127.0.0.1:9083"},
			}),
			wantIDs:    []uint32{1, 2, 3},
			wantCfgIDs: []uint32{1}, // only self-node until peers connect
		},
		{
			name:      "EmptyMapRejected",
			opt:       WithNodes(map[uint32]testNode{}),
			wantPanic: "missing required node map",
		},
		{
			name: "NodeZeroRejected",
			opt: WithNodes(map[uint32]testNode{
				0: {"127.0.0.1:9080"},
				1: {"127.0.0.1:9081"},
			}),
			wantPanic: "node 0 is reserved",
		},
		{
			name: "DuplicateAddressRejected",
			opt: WithNodes(map[uint32]testNode{
				1: {"127.0.0.1:9081"},
				2: {"127.0.0.1:9081"}, // same address as ID 1
			}),
			wantPanic: "already in use by node",
		},
		{
			name: "InvalidAddressRejected",
			opt: WithNodes(map[uint32]testNode{
				1: {"not-an-address"},
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
					newInboundManager(1, tc.opt, 0, nil, nil)
				})
				return
			}
			im := newInboundManager(1, tc.opt, 0, nil, nil)
			nodes := im.Nodes()
			if len(nodes) != len(tc.wantIDs) {
				t.Fatalf("len(im.Nodes()) = %d; want %d", len(nodes), len(tc.wantIDs))
			}
			for i, node := range nodes {
				if node.ID() != tc.wantIDs[i] {
					t.Errorf("Node %d ID = %d; want %d", i, node.ID(), tc.wantIDs[i])
				}
			}
			if got := im.Config().NodeIDs(); !slices.Equal(got, tc.wantCfgIDs) {
				t.Errorf("Config().NodeIDs() = %v; want %v", got, tc.wantCfgIDs)
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

// TestAcceptPeerUpdatesConfig checks that the Configuration is correctly
// updated through sequences of peer connections and disconnections
// (via AcceptPeer and its returned cleanup function), including out-of-order
// connection, stream breakage followed by reconnect, and idempotent cleanups.
//
// The self-node (myID=1) is always present after construction,
// and is included in every wantIDs slice.
func TestAcceptPeerUpdatesConfig(t *testing.T) {
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
			checkIDs(t, im.Config(), []uint32{1}, "initial")

			cleanups := make(map[uint32]func())
			for i, s := range tc.steps {
				switch s.op {
				case "register":
					inStream := newMockBidiStream()
					t.Cleanup(inStream.close)
					_, cleanup, _ := im.AcceptPeer(inboundCtx(t.Context(), s.id), inStream)
					cleanups[s.id] = cleanup
				case "unregister":
					if cleanup, ok := cleanups[s.id]; ok {
						cleanup()
					}
				default:
					t.Fatalf("unknown op %q in step %d", s.op, i)
				}
				checkIDs(t, im.Config(), s.wantIDs, fmt.Sprintf("step %d (%s id=%d)", i, s.op, s.id))
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
			name:     "UntrackedClientNoMetadata",
			ctx:      t.Context(), // no gorums-node-id metadata: regular client, not tracked in ClientConfig
			wantNode: false,
		},
		{
			name:     "PeerClientAccepted",
			ctx:      inboundCtx(t.Context(), 0), // gorums-node-id: 0 => back-channel to peer client
			wantNode: true,
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
			node, cleanup, err := im.AcceptPeer(tc.ctx, inStream)
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

// TestAcceptPeerReplacesExistingStream verifies that calling AcceptPeer for a
// peer that already has an active channel atomically replaces the old channel
// with the new one. This is the correct behavior for reconnects: the old
// (potentially broken) stream may be lingering when the new one connects and takes over.
func TestAcceptPeerReplacesExistingStream(t *testing.T) {
	im := newTestInboundManager(t, 1)

	// First connection for peer 3.
	first := newMockBidiStream()
	t.Cleanup(first.close)
	im.AcceptPeer(inboundCtx(t.Context(), 3), first)
	checkIDs(t, im.Config(), []uint32{1, 3}, "after first connect")

	// Peer 3 reconnects — second AcceptPeer must replace the first channel.
	second := newMockBidiStream()
	t.Cleanup(second.close)
	im.AcceptPeer(inboundCtx(t.Context(), 3), second)

	checkIDs(t, im.Config(), []uint32{1, 3}, "after replacement")
	node := im.nodes[3]
	if ch := node.channel.Load(); ch == nil {
		t.Fatal("channel should not be nil after replacement")
	}
}

// TestAcceptPeerStaleCleanupDoesNotDetachReplacement verifies that when a peer
// reconnects, the cleanup function returned for the old stream cannot detach
// the replacement channel.
func TestAcceptPeerStaleCleanupDoesNotDetachReplacement(t *testing.T) {
	im := newTestInboundManager(t, 1)

	first := newMockBidiStream()
	t.Cleanup(first.close)
	_, cleanupFirst, err := im.AcceptPeer(inboundCtx(t.Context(), 2), first)
	if err != nil {
		t.Fatalf("AcceptPeer(first) error: %v", err)
	}
	checkIDs(t, im.Config(), []uint32{1, 2}, "after first connect")

	second := newMockBidiStream()
	t.Cleanup(second.close)
	_, cleanupSecond, err := im.AcceptPeer(inboundCtx(t.Context(), 2), second)
	if err != nil {
		t.Fatalf("AcceptPeer(second) error: %v", err)
	}
	checkIDs(t, im.Config(), []uint32{1, 2}, "after replacement")

	// Stale cleanup from the first connection must not detach the replacement.
	cleanupFirst()
	checkIDs(t, im.Config(), []uint32{1, 2}, "after stale cleanup")
	if im.nodes[2].channel.Load() == nil {
		t.Fatal("stale cleanup detached the replacement channel")
	}

	// Current cleanup should detach the active channel.
	cleanupSecond()
	checkIDs(t, im.Config(), []uint32{1}, "after current cleanup")
	if im.nodes[2].channel.Load() != nil {
		t.Fatal("current cleanup should detach the active channel")
	}
}

// testPeerServer creates a Server with WithPeers(1, peerNodes()), starts it
// via TestServers, and returns the server and its addresses.
func testPeerServer(t *testing.T) (*Server, []string) {
	t.Helper()
	var srv *Server
	addrs := TestServers(t, 1, func(_ int) ServerIface {
		srv = NewServer(WithConfig(1, peerNodes()))
		return srv
	})
	return srv, addrs
}

func equalNodeIDs(ids []uint32) func(Configuration) bool {
	return func(cfg Configuration) bool {
		return slices.Equal(cfg.NodeIDs(), ids)
	}
}

// peerNodes creates the peer NodeListOption used by the E2E tests.
func peerNodes() NodeListOption {
	return WithNodes(map[uint32]testNode{
		1: {"127.0.0.1:9001"},
		2: {"127.0.0.1:9002"},
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

// TestKnownPeerConnects verifies the end-to-end path:
// a client that sends gorums-node-id metadata connects to a gorums Server
// with WithPeers; NodeStream calls AcceptPeer; the peer appears in
// Config alongside the self-node.
func TestKnownPeerConnects(t *testing.T) {
	srv, addrs := testPeerServer(t)

	checkIDs(t, srv.Config(), []uint32{1}, "before connect")

	connectAsPeer(t, 2, addrs)

	WaitForConfigCondition(t, srv.Config, equalNodeIDs([]uint32{1, 2}))
	checkIDs(t, srv.Config(), []uint32{1, 2}, "after connect")
}

// TestKnownPeerDisconnects verifies that when a peer closes its
// connection the cleanup deferred in NodeStream fires and removes the peer
// from Config.
func TestKnownPeerDisconnects(t *testing.T) {
	srv, addrs := testPeerServer(t)

	mgr := connectAsPeer(t, 2, addrs)
	WaitForConfigCondition(t, srv.Config, equalNodeIDs([]uint32{1, 2}))

	// Close the peer manager to trigger disconnect; Close is idempotent so
	// t.Cleanup (registered by connectAsPeer via TestManager) is harmless.
	if err := mgr.Close(); err != nil {
		t.Fatalf("mgr.Close() error: %v", err)
	}
	WaitForConfigCondition(t, srv.Config, equalNodeIDs([]uint32{1}))
	checkIDs(t, srv.Config(), []uint32{1}, "after disconnect")
}

// TestUnknownPeerIgnored verifies that a client sending an
// unknown or zero node ID does not affect Config.
func TestUnknownPeerIgnored(t *testing.T) {
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
	checkIDs(t, srv.Config(), []uint32{1}, "external and unknown peers must not appear")
}

type mockRequestHandler struct {
	handlers map[string]Handler
}

func (m mockRequestHandler) HandleRequest(ctx context.Context, msg *stream.Message, release func(), send func(*stream.Message)) {
	srvCtx := ServerCtx{Context: ctx, release: release, send: send}
	handler, ok := m.handlers[msg.GetMethod()]
	if !ok {
		release()
		return
	}
	defer release()
	inMsg, err := unmarshalRequest(msg)
	in := &Message{Msg: inMsg, Message: msg}
	if err != nil {
		_ = srvCtx.SendMessage(MessageWithError(in, nil, err))
		return
	}
	out, err := handler(srvCtx, in)
	if out != nil || err != nil {
		_ = srvCtx.SendMessage(MessageWithError(in, out, err))
	}
}

// TestKnownPeerServerCallsClient verifies the full symmetric communication path:
// server sends a request to a connected client via an inbound channel,
// the client's Channel.receiver dispatches to a registered handler,
// the handler responds, and the server receives the response via RouteResponse.
func TestKnownPeerServerCallsClient(t *testing.T) {
	srv, addrs := testPeerServer(t)

	// Client connects as peer 2 with a handler injected via withRequestHandler.
	clientHandlers := map[string]Handler{
		mock.TestMethod: func(_ ServerCtx, in *Message) (*Message, error) {
			req := AsProto[*pb.StringValue](in)
			return NewResponseMessage(in, pb.String("echo: "+req.GetValue())), nil
		},
	}
	peerMD := metadata.Pairs(gorumsNodeIDKey, "2")
	mgr := TestManager(t, WithMetadata(peerMD), withRequestHandler(mockRequestHandler{handlers: clientHandlers}, 0))
	_, err := NewConfiguration(mgr, WithNodeList(addrs))
	if err != nil {
		t.Fatalf("NewConfiguration() error: %v", err)
	}

	// Wait for the peer to appear in the inbound config.
	WaitForConfigCondition(t, srv.Config, equalNodeIDs([]uint32{1, 2}))

	// Server sends a request to the client via the inbound node.
	inboundCfg := srv.Config()
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
	reqMsg, err := stream.NewMessage(ctx, srv.getMsgID(), mock.TestMethod, pb.String("hello"))
	if err != nil {
		t.Fatalf("NewMessage() error: %v", err)
	}
	replyChan := make(chan NodeResponse[*stream.Message], 1)
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
		respMsg, err := unmarshalResponse(resp.Value)
		if err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}
		sv, ok := respMsg.(*pb.StringValue)
		if !ok {
			t.Fatalf("response type = %T; want *pb.StringValue", respMsg)
		}
		if got, want := sv.GetValue(), "echo: hello"; got != want {
			t.Errorf("response = %q; want %q", got, want)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for response")
	}
}

// testClientServer creates a Server, starts it via TestServers, and returns
// the server and its addresses. The server automatically accepts anonymous clients.
func testClientServer(t *testing.T) (*Server, []string) {
	t.Helper()
	var srv *Server
	addrs := TestServers(t, 1, func(_ int) ServerIface {
		srv = NewServer()
		return srv
	})
	return srv, addrs
}

// connectAsPeerClient creates a Manager that advertises back-channel
// capability by sending the gorums-node-id key (via [withRequestHandler]),
// connects to addrs, and returns the manager. The server will include it in
// ClientConfig and may dispatch server-initiated calls to it.
func connectAsPeerClient(t *testing.T, addrs []string) *Manager {
	t.Helper()
	mgr := TestManager(t, withRequestHandler(NewServer(), 0))
	_, err := NewConfiguration(mgr, WithNodeList(addrs))
	if err != nil {
		t.Fatalf("NewConfiguration() error: %v", err)
	}
	return mgr
}

// TestClientConfigConnects verifies that a server accepts a peer-capable
// client (with gorums-node-id: 0) and includes it in ClientConfig.
func TestClientConfigConnects(t *testing.T) {
	srv, addrs := testClientServer(t)

	// Initially no peers (no self-node since myID == 0)
	checkIDs(t, srv.ClientConfig(), []uint32{}, "before connect")

	connectAsPeerClient(t, addrs)

	// Client peer should appear with auto-assigned ID >= clientIDStart.
	WaitForConfigCondition(t, srv.ClientConfig, func(cfg Configuration) bool { return len(cfg) > 0 })
	cfg := srv.ClientConfig()
	if len(cfg) != 1 {
		t.Fatalf("ClientConfig has %d nodes; want 1", len(cfg))
	}
	if cfg[0].ID() < clientIDStart {
		t.Errorf("Client peer ID = %d; want >= %d", cfg[0].ID(), clientIDStart)
	}
}

// TestClientConfigDisconnects verifies that client peers are removed from
// ClientConfig and the nodes map when they disconnect.
func TestClientConfigDisconnects(t *testing.T) {
	srv, addrs := testClientServer(t)

	mgr := connectAsPeerClient(t, addrs)

	// Wait for the client peer to appear.
	WaitForConfigCondition(t, srv.ClientConfig, func(cfg Configuration) bool { return len(cfg) > 0 })
	if len(srv.ClientConfig()) != 1 {
		t.Fatalf("ClientConfig has %d nodes; want 1", len(srv.ClientConfig()))
	}

	// Disconnect the client peer.
	if err := mgr.Close(); err != nil {
		t.Fatalf("mgr.Close() error: %v", err)
	}

	// Wait for config to become empty.
	WaitForConfigCondition(t, srv.ClientConfig, func(cfg Configuration) bool { return len(cfg) == 0 })
	checkIDs(t, srv.ClientConfig(), []uint32{}, "after disconnect")
}

// TestClientConfigMixedMode verifies that a server with both WithConfig and
// WithClientConfig accepts known peers by ID and unknown clients dynamically.
func TestClientConfigMixedMode(t *testing.T) {
	srv, addrs := testPeerServer(t)

	// Self-node (ID 1) is present initially.
	checkIDs(t, srv.Config(), []uint32{1}, "before connect")

	// Connect known peer (ID 2).
	connectAsPeer(t, 2, addrs)
	WaitForConfigCondition(t, srv.Config, equalNodeIDs([]uint32{1, 2}))

	// Connect peer-capable anonymous client (dynamic peer).
	connectAsPeerClient(t, addrs)

	// Wait for 1 dynamic node.
	WaitForConfigCondition(t, srv.ClientConfig, func(cfg Configuration) bool { return len(cfg) == 1 })
	dynCfg := srv.ClientConfig()
	if len(dynCfg) != 1 {
		t.Fatalf("ClientConfig has %d nodes; want 1", len(dynCfg))
	}
	if dynCfg[0].ID() < clientIDStart {
		t.Errorf("Client peer ID = %d; want >= %d", dynCfg[0].ID(), clientIDStart)
	}
	cfg := srv.Config()
	if len(cfg) != 2 {
		t.Fatalf("Config has %d nodes; want 2", len(cfg))
	}
	ids := cfg.NodeIDs()
	if ids[0] != 1 || ids[1] != 2 {
		t.Errorf("known IDs = %v; want [1, 2]", ids[:2])
	}
}

// TestClientConfigServerCallsClient verifies that a server dispatches a reverse-direction
// multicast to a connected client via [ServerCtx.ClientConfigContext].
func TestClientConfigServerCallsClient(t *testing.T) {
	// Register the server handler before starting so it is present before clients arrive.
	srv := NewServer()
	srv.RegisterHandler(mock.TestMethod, func(ctx ServerCtx, _ *Message) (*Message, error) {
		if cc := ctx.ClientConfigContext(); cc != nil {
			_ = Multicast(cc, pb.String("ping"), mock.Stream)
		}
		return nil, nil // one-way
	})
	addrs := TestServers(t, 1, func(_ int) ServerIface { return srv })

	var wg sync.WaitGroup
	wg.Add(1)

	// Client: a Server whose reverse-direction mock.Stream handler is wired in via withRequestHandler.
	clientSrv := NewServer()
	clientSrv.RegisterHandler(mock.Stream, func(_ ServerCtx, _ *Message) (*Message, error) {
		wg.Done()
		return nil, nil
	})
	mgr := TestManager(t, withRequestHandler(clientSrv, 0))
	clientConfig, err := NewConfiguration(mgr, WithNodeList(addrs))
	if err != nil {
		t.Fatalf("NewConfiguration() error: %v", err)
	}

	// Wait for the client to appear in the server's ClientConfig.
	WaitForConfigCondition(t, srv.ClientConfig, func(cfg Configuration) bool { return len(cfg) > 0 })

	// Trigger: client multicasts TestMethod to the server; server fans it back via ClientConfig.
	ctx := TestContext(t, 2*time.Second)
	if err := Multicast(clientConfig.Context(ctx), pb.String("trigger"), mock.TestMethod); err != nil {
		t.Fatalf("Multicast error: %v", err)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("timed out waiting for reverse-direction handler")
	}
}
