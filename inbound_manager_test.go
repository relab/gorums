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
	"google.golang.org/grpc/metadata"
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

// newTestInboundManager creates an InboundManager with myID and three known peers.
func newTestInboundManager(t *testing.T, myID uint32) *InboundManager {
	t.Helper()
	im, err := NewInboundManager(myID, WithNodes(map[uint32]inboundTestNode{
		1: {addr: "127.0.0.1:9081"},
		2: {addr: "127.0.0.1:9082"},
		3: {addr: "127.0.0.1:9083"},
	}))
	if err != nil {
		t.Fatalf("NewInboundManager() unexpected error: %v", err)
	}
	return im
}

func TestNewInboundManager(t *testing.T) {
	tests := []struct {
		name       string
		opt        NodeListOption
		wantIDs    []uint32
		wantCfgIDs []uint32 // expected InboundConfig IDs after construction
		wantErr    string
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
			name:    "EmptyMapRejected",
			opt:     WithNodes(map[uint32]inboundTestNode{}),
			wantErr: "missing required node map",
		},
		{
			name: "NodeZeroRejected",
			opt: WithNodes(map[uint32]inboundTestNode{
				0: {addr: "127.0.0.1:9080"},
				1: {addr: "127.0.0.1:9081"},
			}),
			wantErr: "node 0 is reserved",
		},
		{
			name: "DuplicateAddressRejected",
			opt: WithNodes(map[uint32]inboundTestNode{
				1: {addr: "127.0.0.1:9081"},
				2: {addr: "127.0.0.1:9081"}, // same address as ID 1
			}),
			wantErr: "already in use by node",
		},
		{
			name: "InvalidAddressRejected",
			opt: WithNodes(map[uint32]inboundTestNode{
				1: {addr: "not-an-address"},
			}),
			wantErr: "invalid address",
		},
		{
			// WithNodeList assigns IDs starting at 1.
			name:       "WithNodeListAssignsIDs",
			opt:        WithNodeList([]string{"127.0.0.1:9081", "127.0.0.1:9082", "127.0.0.1:9083"}),
			wantIDs:    []uint32{1, 2, 3},
			wantCfgIDs: []uint32{1}, // only self-node until peers connect
		},
		{
			name:    "WithNodeListEmptyRejected",
			opt:     WithNodeList([]string{}),
			wantErr: "missing required node addresses",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			im, err := NewInboundManager(1, tc.opt)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("NewInboundManager() error = nil; want error containing %q", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("NewInboundManager() error = %q; want it to contain %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewInboundManager() unexpected error: %v", err)
			}
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
					im.UnregisterPeer(s.id)
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
			node, err := im.AcceptPeer(tc.ctx, inStream)
			if err != nil {
				t.Fatalf("AcceptPeer() unexpected error: %v", err)
			}
			if (node != nil) != tc.wantNode {
				t.Errorf("AcceptPeer() node non-nil = %v; want %v", node != nil, tc.wantNode)
			}
			if node != nil {
				im.UnregisterPeer(node.ID())
			}
		})
	}
}

func TestAcceptPeerReturnsNode(t *testing.T) {
	im := newTestInboundManager(t, 1)
	inStream := newMockBidiStream()
	defer inStream.close()

	node, err := im.AcceptPeer(inboundCtx(t.Context(), 3), inStream)
	if err != nil {
		t.Fatalf("AcceptPeer() unexpected error: %v", err)
	}
	if node == nil {
		t.Fatal("AcceptPeer() node = nil; want non-nil for known peer")
	}
	checkIDs(t, im.InboundConfig(), []uint32{1, 3}, "after AcceptPeer")

	im.UnregisterPeer(node.ID())
	checkIDs(t, im.InboundConfig(), []uint32{1}, "after UnregisterPeer")

	// Calling UnregisterPeer again must be a no-op (idempotent detachStream).
	im.UnregisterPeer(node.ID())
	checkIDs(t, im.InboundConfig(), []uint32{1}, "after second UnregisterPeer")
}

// waitForConfig polls im.InboundConfig until its NodeIDs equal wantIDs or the
// timeout elapses. It reports an error (not fatal) on timeout so the caller can
// inspect the final state.
func waitForConfig(t *testing.T, im *InboundManager, wantIDs []uint32) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if slices.Equal(im.InboundConfig().NodeIDs(), wantIDs) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Errorf("timeout waiting for config %v; got %v", wantIDs, im.InboundConfig().NodeIDs())
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
// with WithInboundManager; NodeStream calls AcceptPeer; the peer appears in
// InboundConfig alongside the self-node.
func TestInboundManagerPeerConnects(t *testing.T) {
	im, err := NewInboundManager(1, WithNodes(map[uint32]inboundTestNode{
		1: {addr: "127.0.0.1:9001"},
		2: {addr: "127.0.0.1:9002"},
	}))
	if err != nil {
		t.Fatalf("NewInboundManager() error: %v", err)
	}
	addrs := TestServers(t, 1, func(_ int) ServerIface {
		return NewServer(WithInboundManager(im))
	})

	checkIDs(t, im.InboundConfig(), []uint32{1}, "before connect")

	connectAsPeer(t, 2, addrs)

	waitForConfig(t, im, []uint32{1, 2})
	checkIDs(t, im.InboundConfig(), []uint32{1, 2}, "after connect")
}

// TestInboundManagerPeerDisconnects verifies that when a peer closes its
// connection the cleanup deferred in NodeStream fires and removes the peer
// from InboundConfig.
func TestInboundManagerPeerDisconnects(t *testing.T) {
	im, err := NewInboundManager(1, WithNodes(map[uint32]inboundTestNode{
		1: {addr: "127.0.0.1:9001"},
		2: {addr: "127.0.0.1:9002"},
	}))
	if err != nil {
		t.Fatalf("NewInboundManager() error: %v", err)
	}
	addrs := TestServers(t, 1, func(_ int) ServerIface {
		return NewServer(WithInboundManager(im))
	})

	mgr := connectAsPeer(t, 2, addrs)
	waitForConfig(t, im, []uint32{1, 2})

	// Close the peer manager to trigger disconnect; Close is idempotent so
	// t.Cleanup (registered by connectAsPeer via TestManager) is harmless.
	if err := mgr.Close(); err != nil {
		t.Fatalf("mgr.Close() error: %v", err)
	}
	waitForConfig(t, im, []uint32{1})
	checkIDs(t, im.InboundConfig(), []uint32{1}, "after disconnect")
}

// TestInboundManagerUnknownPeerIgnored verifies that a client sending an
// unknown or zero node ID does not affect InboundConfig.
func TestInboundManagerUnknownPeerIgnored(t *testing.T) {
	im, err := NewInboundManager(1, WithNodes(map[uint32]inboundTestNode{
		1: {addr: "127.0.0.1:9001"},
		2: {addr: "127.0.0.1:9002"},
	}))
	if err != nil {
		t.Fatalf("NewInboundManager() error: %v", err)
	}
	addrs := TestServers(t, 1, func(_ int) ServerIface {
		return NewServer(WithInboundManager(im))
	})

	// Connect without metadata (external client) and with an unknown ID.
	external := TestManager(t)
	_, err = NewConfiguration(external, WithNodeList(addrs))
	if err != nil {
		t.Fatalf("NewConfiguration() error: %v", err)
	}

	connectAsPeer(t, 99, addrs) // ID 99 not in known set

	// Give the server time to process both connections.
	time.Sleep(50 * time.Millisecond)
	checkIDs(t, im.InboundConfig(), []uint32{1}, "external and unknown peers must not appear")
}
