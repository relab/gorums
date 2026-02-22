package gorums

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/relab/gorums/internal/stream"
	"google.golang.org/grpc/metadata"
)

// inboundTestNode is a minimal NodeAddress for use in InboundManager tests.
type inboundTestNode struct {
	addr string
}

func (n inboundTestNode) Addr() string { return n.addr }

// Compile-time assertions: both node providers satisfy NodeListOption.
var _ NodeListOption = nodeMap[inboundTestNode](nil)
var _ NodeListOption = nodeList(nil)

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

// newTestInboundManager creates an InboundManager with myID and three known
// peers (IDs 1–3). It uses the public WithNodes API to mirror what an external
// user would write.
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
			wantCfgIDs: []uint32{1}, // only self-node (myID=1) until peers connect
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
// Using t.Context() as parent ensures channel goroutines are cleaned up when
// the test ends.
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
// by reconnect, and idempotent / stale unregister calls.
//
// The self-node (myID=1) is always present from construction; it is included in
// every wantIDs slice.
func TestInboundManagerRegisterUnregister(t *testing.T) {
	type configStep struct {
		op      string   // "register" or "unregister"
		id      uint32   // peer ID for "register"; ignored for "unregister"
		slot    string   // key to store/retrieve the unregister func
		wantIDs []uint32 // expected config IDs after this step
	}

	tests := []struct {
		name  string
		steps []configStep
	}{
		{
			name: "RegisterAndUnregister",
			steps: []configStep{
				{op: "register", id: 2, slot: "2a", wantIDs: []uint32{1, 2}},
				{op: "unregister", slot: "2a", wantIDs: []uint32{1}},
			},
		},
		{
			name: "RegisterAllPeers",
			steps: []configStep{
				{op: "register", id: 2, slot: "2a", wantIDs: []uint32{1, 2}},
				{op: "register", id: 3, slot: "3a", wantIDs: []uint32{1, 2, 3}},
				{op: "unregister", slot: "2a", wantIDs: []uint32{1, 3}},
				{op: "unregister", slot: "3a", wantIDs: []uint32{1}},
			},
		},
		{
			// Peers connect in reverse order; config must always be sorted.
			name: "RegisterOutOfOrderSorted",
			steps: []configStep{
				{op: "register", id: 3, slot: "3a", wantIDs: []uint32{1, 3}},
				{op: "register", id: 2, slot: "2a", wantIDs: []uint32{1, 2, 3}},
				{op: "unregister", slot: "2a", wantIDs: []uint32{1, 3}},
				{op: "unregister", slot: "3a", wantIDs: []uint32{1}},
			},
		},
		{
			// Simulates a stream breaking and the peer reconnecting.
			name: "StreamBreakageAndReconnect",
			steps: []configStep{
				{op: "register", id: 2, slot: "2a", wantIDs: []uint32{1, 2}},
				{op: "unregister", slot: "2a", wantIDs: []uint32{1}},         // stream broken
				{op: "register", id: 2, slot: "2b", wantIDs: []uint32{1, 2}}, // peer reconnects
				{op: "unregister", slot: "2b", wantIDs: []uint32{1}},
			},
		},
		{
			// A stale cleanup from a previous registration must not evict a peer
			// that has since reconnected. sync.Once in registerPeer prevents this.
			name: "StaleUnregisterAfterReconnect",
			steps: []configStep{
				{op: "register", id: 2, slot: "2a", wantIDs: []uint32{1, 2}},
				{op: "unregister", slot: "2a", wantIDs: []uint32{1}},         // first call: removes peer
				{op: "register", id: 2, slot: "2b", wantIDs: []uint32{1, 2}}, // peer reconnects
				{op: "unregister", slot: "2a", wantIDs: []uint32{1, 2}},      // stale: must be no-op
				{op: "unregister", slot: "2b", wantIDs: []uint32{1}},
			},
		},
		{
			// Calling the same unregister function multiple times must be a no-op
			// after the first invocation.
			name: "IdempotentUnregister",
			steps: []configStep{
				{op: "register", id: 3, slot: "3a", wantIDs: []uint32{1, 3}},
				{op: "unregister", slot: "3a", wantIDs: []uint32{1}},
				{op: "unregister", slot: "3a", wantIDs: []uint32{1}}, // second call: no-op
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			im := newTestInboundManager(t, 1)
			checkIDs(t, im.InboundConfig(), []uint32{1}, "initial")

			unregisters := make(map[string]func())
			for i, s := range tc.steps {
				switch s.op {
				case "register":
					inStream := newMockBidiStream()
					t.Cleanup(inStream.close)
					unregisters[s.slot] = im.registerPeer(s.id, inStream, t.Context())
				case "unregister":
					unregisters[s.slot]()
				default:
					t.Fatalf("unknown op %q in step %d", s.op, i)
				}
				checkIDs(t, im.InboundConfig(), s.wantIDs, fmt.Sprintf("step %d (%s slot=%s)", i, s.op, s.slot))
			}
		})
	}
}

func TestAcceptPeer(t *testing.T) {
	im := newTestInboundManager(t, 1)

	tests := []struct {
		name        string
		ctx         context.Context
		wantCleanup bool
	}{
		{
			name:        "ExternalClientNoMetadata",
			ctx:         t.Context(), // no gorums-node-id metadata
			wantCleanup: false,
		},
		{
			name:        "UnknownPeerID",
			ctx:         inboundCtx(t.Context(), 99), // not in configured set
			wantCleanup: false,
		},
		{
			name:        "KnownPeer",
			ctx:         inboundCtx(t.Context(), 2),
			wantCleanup: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inStream := newMockBidiStream()
			defer inStream.close()
			cleanup, err := im.AcceptPeer(tc.ctx, inStream)
			if err != nil {
				t.Fatalf("AcceptPeer() unexpected error: %v", err)
			}
			if (cleanup != nil) != tc.wantCleanup {
				t.Errorf("AcceptPeer() cleanup non-nil = %v; want %v", cleanup != nil, tc.wantCleanup)
			}
			if cleanup != nil {
				cleanup()
			}
		})
	}
}

func TestAcceptPeerReturnsCleanup(t *testing.T) {
	im := newTestInboundManager(t, 1)
	inStream := newMockBidiStream()
	defer inStream.close()

	cleanup, err := im.AcceptPeer(inboundCtx(t.Context(), 3), inStream)
	if err != nil {
		t.Fatalf("AcceptPeer() unexpected error: %v", err)
	}
	if cleanup == nil {
		t.Fatal("AcceptPeer() cleanup = nil; want non-nil for known peer")
	}
	checkIDs(t, im.InboundConfig(), []uint32{1, 3}, "after AcceptPeer")

	cleanup()
	checkIDs(t, im.InboundConfig(), []uint32{1}, "after cleanup")

	// Calling cleanup again must be a no-op (idempotent).
	cleanup()
	checkIDs(t, im.InboundConfig(), []uint32{1}, "after second cleanup")
}
