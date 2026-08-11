package conn

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/relab/gorums/internal/stream"
	"github.com/relab/gorums/internal/testutils/mock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
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

func (*mockBidiStream) Send(*stream.Message) error { return nil }
func (m *mockBidiStream) Recv() (*stream.Message, error) {
	msg, ok := <-m.ch
	if !ok {
		return nil, io.EOF
	}
	return msg, nil
}

// recordingBidiStream is a [stream.BidiStream] that records every message
// passed to Send, so a test can observe which of several overlapping inbound
// streams actually carried a reply.
type recordingBidiStream struct {
	sent chan *stream.Message
	recv chan *stream.Message
}

func newRecordingBidiStream() *recordingBidiStream {
	return &recordingBidiStream{
		sent: make(chan *stream.Message, 16),
		recv: make(chan *stream.Message, 16),
	}
}

func (s *recordingBidiStream) Send(msg *stream.Message) error {
	s.sent <- msg
	return nil
}

func (s *recordingBidiStream) Recv() (*stream.Message, error) {
	msg, ok := <-s.recv
	if !ok {
		return nil, io.EOF
	}
	return msg, nil
}

func (s *recordingBidiStream) close() { close(s.recv) }

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

// testNode is a minimal NodeSource for use in tests.
type testNode struct {
	addr string
}

func (n testNode) Addr() string { return n.addr }

// Compile-time assertions: both node providers satisfy NodeSource.
var _ NodeSource = nodeMap[testNode](nil)

// newTestInboundManager creates an InboundManager with myID and three known peers.
func newTestInboundManager(t *testing.T, myID uint32) *InboundManager {
	t.Helper()
	im := NewInboundManager(myID, WithNodes(map[uint32]testNode{
		1: {"127.0.0.1:9081"},
		2: {"127.0.0.1:9082"},
		3: {"127.0.0.1:9083"},
	}), 0, nil, nil)
	return im
}

func TestNewInboundManager(t *testing.T) {
	tests := []struct {
		name       string
		opt        NodeSource
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
					NewInboundManager(1, tc.opt, 0, nil, nil)
				})
				return
			}
			im := NewInboundManager(1, tc.opt, 0, nil, nil)
			nodes := im.Nodes()
			if len(nodes) != len(tc.wantIDs) {
				t.Fatalf("len(im.Nodes()) = %d; want %d", len(nodes), len(tc.wantIDs))
			}
			for i, node := range nodes {
				if node.ID() != tc.wantIDs[i] {
					t.Errorf("Node %d ID = %d; want %d", i, node.ID(), tc.wantIDs[i])
				}
			}
			if got := im.ConnectedPeers().NodeIDs(); !slices.Equal(got, tc.wantCfgIDs) {
				t.Errorf("Config().NodeIDs() = %v; want %v", got, tc.wantCfgIDs)
			}
		})
	}
}

func TestInboundManagerKeepsHighKnownPeerIDs(t *testing.T) {
	im := NewInboundManager(ClientIDStart, WithNodes(map[uint32]testNode{
		ClientIDStart: {"127.0.0.1:9081"},
	}), 0, nil, nil)

	checkIDs(t, im.ConnectedPeers(), []uint32{ClientIDStart}, "known peers")
	checkIDs(t, im.ConnectedClients(), []uint32{}, "dynamic clients")
}

func TestInboundManagerDynamicClientIDSkipsKnownPeer(t *testing.T) {
	im := NewInboundManager(1, WithNodes(map[uint32]testNode{
		1:             {"127.0.0.1:9081"},
		ClientIDStart: {"127.0.0.1:9082"},
	}), 0, nil, nil)
	clientStream := newMockBidiStream()
	t.Cleanup(clientStream.close)

	_, cleanup, err := im.AcceptPeer(inboundCtx(t.Context(), 0), clientStream)
	if err != nil {
		t.Fatalf("AcceptPeer: %v", err)
	}
	t.Cleanup(cleanup)

	checkIDs(t, im.ConnectedClients(), []uint32{ClientIDStart + 1}, "dynamic clients")
	if _, ok := im.knownNodes[ClientIDStart]; !ok {
		t.Fatalf("known peer %d was overwritten by dynamic client", ClientIDStart)
	}
}

func TestInboundManagerDynamicClientIDExhaustion(t *testing.T) {
	maxID := ^uint32(0)
	tests := []struct {
		name        string
		knownNodes  map[uint32]*Node
		clientNodes map[uint32]*Node
		wantID      uint32
		wantErr     bool
	}{
		{
			name:        "MaximumIDAvailable",
			knownNodes:  make(map[uint32]*Node),
			clientNodes: make(map[uint32]*Node),
			wantID:      maxID,
		},
		{
			name:        "MaximumIDUsedByKnownPeer",
			knownNodes:  map[uint32]*Node{maxID: nil},
			clientNodes: make(map[uint32]*Node),
			wantErr:     true,
		},
		{
			name:        "MaximumIDUsedByClient",
			knownNodes:  make(map[uint32]*Node),
			clientNodes: map[uint32]*Node{maxID: nil},
			wantErr:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			im := &InboundManager{
				knownNodes:   tt.knownNodes,
				clientNodes:  tt.clientNodes,
				nextClientID: uint64(maxID),
			}
			im.mu.Lock()
			got, err := im.nextAvailableClientID()
			im.mu.Unlock()
			if (err != nil) != tt.wantErr {
				t.Fatalf("nextAvailableClientID() error = %v, wantErr %t", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.wantID {
				t.Fatalf("nextAvailableClientID() = %d, want %d", got, tt.wantID)
			}
		})
	}
}

// inboundCtx returns a context carrying nodeID metadata, rooted at parent.
func inboundCtx(parent context.Context, id uint32) context.Context {
	return metadata.NewIncomingContext(parent, MetadataWithNodeID(id))
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
func checkIDs(t *testing.T, cfg Config, wantIDs []uint32, label string) {
	t.Helper()
	if got := cfg.NodeIDs(); !slices.Equal(got, wantIDs) {
		t.Errorf("%s: config IDs = %v; want %v", label, got, wantIDs)
	}
}

// TestAcceptPeerUpdatesConfig checks that the Config is correctly
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
			checkIDs(t, im.ConnectedPeers(), []uint32{1}, "initial")

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
				checkIDs(t, im.ConnectedPeers(), s.wantIDs, fmt.Sprintf("step %d (%s id=%d)", i, s.op, s.id))
			}
		})
	}
}

func TestAcceptPeer(t *testing.T) {
	im := newTestInboundManager(t, 1)

	typePeerNode := reflect.TypeFor[peerNode]()
	typeNilPeer := reflect.TypeFor[*nilPeerNode]()

	tests := []struct {
		name     string
		ctx      context.Context
		wantType reflect.Type
		wantErr  bool
	}{
		{
			name:     "UntrackedClientNoMetadata",
			ctx:      t.Context(), // no gorums-node-id metadata: regular client, not tracked in ConnectedClients
			wantType: typeNilPeer,
		},
		{
			name:     "PeerClientAccepted",
			ctx:      inboundCtx(t.Context(), 0), // gorums-node-id: 0 => back-channel to peer client
			wantType: typePeerNode,
		},
		{
			name:     "UnknownPeerID",
			ctx:      inboundCtx(t.Context(), 99), // not in configured set
			wantType: typeNilPeer,
		},
		{
			name:    "SelfNodeIDRejected",
			ctx:     inboundCtx(t.Context(), 1), // claims this server's own ID: reject so the RPC terminates
			wantErr: true,
		},
		{
			name:     "KnownPeer",
			ctx:      inboundCtx(t.Context(), 2),
			wantType: typePeerNode,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inStream := newMockBidiStream()
			defer inStream.close()
			node, cleanup, err := im.AcceptPeer(tc.ctx, inStream)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("AcceptPeer() error = nil; want a rejection error")
				}
				if status.Code(err) != codes.InvalidArgument {
					t.Errorf("AcceptPeer() error code = %v; want %v", status.Code(err), codes.InvalidArgument)
				}
				return
			}
			if err != nil {
				t.Fatalf("AcceptPeer() unexpected error: %v", err)
			}
			if got := reflect.TypeOf(node); got != tc.wantType {
				t.Errorf("AcceptPeer() type = %v; want %v", got, tc.wantType)
			}
			cleanup()
		})
	}
}

// TestAcceptPeerInstallsNewActiveOnReconnect verifies that calling AcceptPeer
// for a peer that already has a live stream installs the new channel as the
// node's active channel. The prior channel stays live until its own stream ends
// (see the stale-cleanup and overlapping-failover tests); this test covers only
// that the newest registration becomes active and the node has a channel.
func TestAcceptPeerInstallsNewActiveOnReconnect(t *testing.T) {
	im := newTestInboundManager(t, 1)

	// First connection for peer 3.
	first := newMockBidiStream()
	t.Cleanup(first.close)
	im.AcceptPeer(inboundCtx(t.Context(), 3), first)
	checkIDs(t, im.ConnectedPeers(), []uint32{1, 3}, "after first connect")

	// Peer 3 reconnects — the second AcceptPeer installs a new active channel.
	second := newMockBidiStream()
	t.Cleanup(second.close)
	im.AcceptPeer(inboundCtx(t.Context(), 3), second)

	checkIDs(t, im.ConnectedPeers(), []uint32{1, 3}, "after reconnect")
	node := im.knownNodes[3]
	if ch := node.activeChannel(); ch == nil {
		t.Fatal("channel should not be nil after reconnect")
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
	checkIDs(t, im.ConnectedPeers(), []uint32{1, 2}, "after first connect")

	second := newMockBidiStream()
	t.Cleanup(second.close)
	_, cleanupSecond, err := im.AcceptPeer(inboundCtx(t.Context(), 2), second)
	if err != nil {
		t.Fatalf("AcceptPeer(second) error: %v", err)
	}
	checkIDs(t, im.ConnectedPeers(), []uint32{1, 2}, "after replacement")

	// Stale cleanup from the first connection must not detach the replacement.
	cleanupFirst()
	checkIDs(t, im.ConnectedPeers(), []uint32{1, 2}, "after stale cleanup")
	if im.knownNodes[2].activeChannel() == nil {
		t.Fatal("stale cleanup detached the replacement channel")
	}

	// Current cleanup should detach the active channel.
	cleanupSecond()
	checkIDs(t, im.ConnectedPeers(), []uint32{1}, "after current cleanup")
	if im.knownNodes[2].activeChannel() != nil {
		t.Fatal("current cleanup should detach the active channel")
	}
}

// TestAcceptPeerOverlappingStreamsFailover verifies that when two inbound
// streams for the same peer overlap, tearing down the stream that registered
// last does not drop the peer: the earlier-registered stream is still live, so
// its channel must remain active and the peer must stay in the configuration.
//
// This is the inverse of TestAcceptPeerStaleCleanupDoesNotDetachReplacement.
// gRPC can open a second NodeStream for a peer over one connection during
// connection churn, and server-side registration order (serialized by the
// manager lock) can invert the client's stream-creation order: the stream the
// client keeps may register first, while the stream it is canceling registers
// second. When that second registration's stream then ends, the peer must fail
// over to the still-live first stream rather than lose its channel — otherwise
// the surviving stream keeps receiving requests but replies are dropped on a
// nil channel and the peer's caller stalls to its deadline.
func TestAcceptPeerOverlappingStreamsFailover(t *testing.T) {
	im := newTestInboundManager(t, 1)

	// The surviving stream registers first.
	survivor := newMockBidiStream()
	t.Cleanup(survivor.close)
	_, cleanupSurvivor, err := im.AcceptPeer(inboundCtx(t.Context(), 2), survivor)
	if err != nil {
		t.Fatalf("AcceptPeer(survivor) error: %v", err)
	}
	survivorCh := im.knownNodes[2].activeChannel()
	if survivorCh == nil {
		t.Fatal("survivor channel is nil after first registration")
	}

	// The doomed (soon-canceled) stream registers second.
	doomed := newMockBidiStream()
	t.Cleanup(doomed.close)
	_, cleanupDoomed, err := im.AcceptPeer(inboundCtx(t.Context(), 2), doomed)
	if err != nil {
		t.Fatalf("AcceptPeer(doomed) error: %v", err)
	}
	checkIDs(t, im.ConnectedPeers(), []uint32{1, 2}, "both streams live")

	// The doomed stream ends first. Peer 2 must stay, failing over to the
	// still-live survivor stream's channel.
	cleanupDoomed()
	checkIDs(t, im.ConnectedPeers(), []uint32{1, 2}, "after doomed stream ends")
	if got := im.knownNodes[2].activeChannel(); got != survivorCh {
		t.Fatalf("active channel = %p after failover; want survivor %p", got, survivorCh)
	}

	// Only once the survivor stream also ends does the peer leave the config.
	cleanupSurvivor()
	checkIDs(t, im.ConnectedPeers(), []uint32{1}, "after survivor stream ends")
	if im.knownNodes[2].activeChannel() != nil {
		t.Fatal("active channel should be nil after all streams end")
	}
}

// TestAcceptPeerReplyRidesReceivingStream verifies that during the multi-live
// overlap window a reply for a request received on one inbound stream leaves on
// that same stream, not on whichever stream happens to be the node's active
// channel. The survivor registers first; a second stream registers and becomes
// active; a reply issued through the survivor's PeerNode must still ride the
// survivor's stream.
func TestAcceptPeerReplyRidesReceivingStream(t *testing.T) {
	im := NewInboundManager(1, WithNodes(map[uint32]testNode{
		1: {"127.0.0.1:9081"},
		2: {"127.0.0.1:9082"},
	}), 4, nil, nil)

	survivor := newRecordingBidiStream()
	t.Cleanup(survivor.close)
	survivorPeer, cleanupSurvivor, err := im.AcceptPeer(inboundCtx(t.Context(), 2), survivor)
	if err != nil {
		t.Fatalf("AcceptPeer(survivor) error: %v", err)
	}
	t.Cleanup(cleanupSurvivor)

	// A second stream registers and becomes the node's active channel.
	active := newRecordingBidiStream()
	t.Cleanup(active.close)
	_, cleanupActive, err := im.AcceptPeer(inboundCtx(t.Context(), 2), active)
	if err != nil {
		t.Fatalf("AcceptPeer(active) error: %v", err)
	}
	t.Cleanup(cleanupActive)

	// Deliver a reply through the survivor's PeerNode, as NodeStream's drain
	// goroutine does for a request received on that stream.
	const replySeqNo = 42
	reply := stream.Message_builder{MessageSeqNo: replySeqNo, Method: mock.TestMethod}.Build()
	survivorPeer.TrySend(stream.Request{Ctx: t.Context(), Msg: reply})

	select {
	case got := <-survivor.sent:
		if got.GetMessageSeqNo() != replySeqNo {
			t.Errorf("survivor stream carried message %d; want %d", got.GetMessageSeqNo(), replySeqNo)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("reply for a request received on the survivor stream never rode that stream")
	}
	select {
	case got := <-active.sent:
		t.Errorf("reply rode the active stream (message %d) instead of the receiving stream", got.GetMessageSeqNo())
	case <-time.After(100 * time.Millisecond):
		// Expected: the active stream carried nothing.
	}
}

// TestOnConfigChangeCallbackFiringOnConstruction verifies that the onChange
// callback fires once during InboundManager construction, with only the
// self-node present in the initial configuration.
func TestOnConfigChangeCallbackFiringOnConstruction(t *testing.T) {
	var calls [][]uint32
	NewInboundManager(1, WithNodes(map[uint32]testNode{
		1: {"127.0.0.1:9081"},
		2: {"127.0.0.1:9082"},
		3: {"127.0.0.1:9083"},
	}), 0, func(cfg Config) {
		calls = append(calls, slices.Clone(cfg.NodeIDs()))
	}, nil)

	if len(calls) != 1 {
		t.Fatalf("onChange fired %d times during construction; want 1", len(calls))
	}
	if got := calls[0]; !slices.Equal(got, []uint32{1}) {
		t.Errorf("construction config IDs = %v; want [1]", got)
	}
}

// TestOnConfigChangeCallbackPeerConnectDisconnect verifies that the onChange
// callback fires with the updated configuration when a known peer connects and
// later disconnects.
func TestOnConfigChangeCallbackPeerConnectDisconnect(t *testing.T) {
	var snapshots [][]uint32
	im := NewInboundManager(1, WithNodes(map[uint32]testNode{
		1: {"127.0.0.1:9081"},
		2: {"127.0.0.1:9082"},
		3: {"127.0.0.1:9083"},
	}), 0, func(cfg Config) {
		snapshots = append(snapshots, slices.Clone(cfg.NodeIDs()))
	}, nil)

	snapshots = nil // discard the construction snapshot

	stream2 := newMockBidiStream()
	t.Cleanup(stream2.close)
	_, cleanup2, _ := im.AcceptPeer(inboundCtx(t.Context(), 2), stream2)

	if len(snapshots) != 1 {
		t.Fatalf("after connect: onChange fired %d time(s); want 1", len(snapshots))
	}
	if got := snapshots[0]; !slices.Equal(got, []uint32{1, 2}) {
		t.Errorf("after connect: config IDs = %v; want [1, 2]", got)
	}

	cleanup2()

	if len(snapshots) != 2 {
		t.Fatalf("after disconnect: onChange fired %d time(s) total; want 2", len(snapshots))
	}
	if got := snapshots[1]; !slices.Equal(got, []uint32{1}) {
		t.Errorf("after disconnect: config IDs = %v; want [1]", got)
	}
}

// TestOnConfigChangeCallbackMultiplePeers verifies that the onChange callback
// fires in sorted ID order as multiple peers connect and disconnect.
func TestOnConfigChangeCallbackMultiplePeers(t *testing.T) {
	var snapshots [][]uint32
	im := NewInboundManager(1, WithNodes(map[uint32]testNode{
		1: {"127.0.0.1:9081"},
		2: {"127.0.0.1:9082"},
		3: {"127.0.0.1:9083"},
	}), 0, func(cfg Config) {
		snapshots = append(snapshots, slices.Clone(cfg.NodeIDs()))
	}, nil)

	snapshots = nil // discard the construction snapshot

	// Peers connect in reverse order; configs must always be sorted.
	stream3 := newMockBidiStream()
	t.Cleanup(stream3.close)
	_, cleanup3, _ := im.AcceptPeer(inboundCtx(t.Context(), 3), stream3)

	stream2 := newMockBidiStream()
	t.Cleanup(stream2.close)
	_, cleanup2, _ := im.AcceptPeer(inboundCtx(t.Context(), 2), stream2)

	cleanup3()
	cleanup2()

	want := [][]uint32{
		{1, 3},    // after peer 3 connects
		{1, 2, 3}, // after peer 2 connects
		{1, 2},    // after peer 3 disconnects
		{1},       // after peer 2 disconnects
	}
	if len(snapshots) != len(want) {
		t.Fatalf("onChange fired %d time(s); want %d", len(snapshots), len(want))
	}
	for i, w := range want {
		if got := snapshots[i]; !slices.Equal(got, w) {
			t.Errorf("snapshot[%d]: got %v; want %v", i, got, w)
		}
	}
}

// TestOnConfigChangeCallbackIdempotentCleanup verifies that calling the cleanup
// function twice does not fire the callback a second time on the same disconnect.
func TestOnConfigChangeCallbackIdempotentCleanup(t *testing.T) {
	var callCount int
	im := NewInboundManager(1, WithNodes(map[uint32]testNode{
		1: {"127.0.0.1:9081"},
		2: {"127.0.0.1:9082"},
	}), 0, func(_ Config) {
		callCount++
	}, nil)

	callCount = 0 // discard the construction call

	stream2 := newMockBidiStream()
	t.Cleanup(stream2.close)
	_, cleanup, _ := im.AcceptPeer(inboundCtx(t.Context(), 2), stream2)

	if callCount != 1 {
		t.Fatalf("after connect: onChange called %d time(s); want 1", callCount)
	}

	cleanup() // first: active detach → rebuildConfig fires
	cleanup() // second: no-op, must not fire again

	if callCount != 2 {
		t.Fatalf("after double cleanup: onChange called %d time(s); want 2", callCount)
	}
}
