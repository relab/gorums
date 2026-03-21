package gorums

import (
	"cmp"
	"context"
	"errors"
	"maps"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/relab/gorums/internal/strconv"

	"github.com/relab/gorums/internal/stream"
	"google.golang.org/grpc/metadata"
)

// gorumsNodeIDKey is the gRPC metadata key used by gorums clients to advertise
// their NodeID to the server.
// See doc/issue-peer-id.md for security considerations and future directions.
const gorumsNodeIDKey = "gorums-node-id"

// nodeID extracts the NodeID from the gorums-node-id metadata key in ctx.
// It returns 0 if the key is absent, empty, or not a valid uint32 greater than zero.
func nodeID(ctx context.Context) uint32 {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return 0
	}
	vals := md.Get(gorumsNodeIDKey)
	if len(vals) == 0 {
		return 0
	}
	id, err := strconv.ParseInteger[uint32](vals[0], 10)
	if err != nil || id == 0 {
		return 0
	}
	return uint32(id)
}

// hasPeerMetadata reports whether ctx contains the gorums-node-id metadata key,
// regardless of its value. A client that sends this key (even with value "0")
// has declared itself capable of receiving back-channel calls. Regular clients
// (those using NewConfig rather than (*Server).NewConfig) never send this key.
func hasPeerMetadata(ctx context.Context) bool {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return false
	}
	return len(md.Get(gorumsNodeIDKey)) > 0
}

// metadataWithNodeID returns a metadata.MD containing the gorums-node-id key with the given id value.
func metadataWithNodeID(id uint32) metadata.MD {
	return metadata.Pairs(gorumsNodeIDKey, strconv.Format(id, 10))
}

// inboundManager manages server-side awareness of connected peers. It is
// configured at construction time with a fixed set of known peers, registers
// them as they connect, and maintains an auto-updated [Configuration] that
// can be used for server-initiated quorum calls, multicast, and other call types.
//
// Clients that specify node ID 0 in their metadata are assumed to be capable
// of receiving reverse-direction calls from the server. These clients are
// accepted with auto-generated IDs and included in the ClientConfig.
// Client nodes are removed from the nodes map when they disconnect, while
// known peer nodes persist in the map to allow for reconnection.
//
// inboundManager is safe for concurrent use.
type inboundManager struct {
	mu             sync.RWMutex
	myID           uint32                // this server's own NodeID; always present in inboundCfg
	nodes          map[uint32]*Node      // pre-created for known peers; client peers added on connect
	config         Configuration         // auto-updated slice of known peer servers, sorted by ID
	clientConfig   Configuration         // auto-updated slice of client peers, sorted by ID
	nextMsgID      atomic.Uint64         // counter for server-initiated message IDs
	sendBufferSize uint                  // send buffer size for inbound channels
	handler        stream.RequestHandler // handler for dispatching incoming requests on all inbound nodes
	onConfigChange func(Configuration)   // optional; called after each known-peer config change
	nextClientID   uint32                // next ID to assign to a client peer
}

// clientIDStart is the starting ID for dynamically assigned client peers.
// Chosen to be high enough to avoid collisions with typical known-peer IDs.
// The available ID space is [clientIDStart, math.MaxUint32], giving approximately
// 4.3 billion unique IDs before exhaustion. acceptClient rejects new peers if the
// counter reaches math.MaxUint32 to prevent silent wraparound.
const clientIDStart = 1 << 20

// newInboundManager creates an inboundManager for this server whose NodeID is myID.
// If opt is non-nil, the inboundManager is configured with the given NodeListOption
// defining the set of known peers. If myID is present in the NodeListOption it is
// immediately included in the Config as the self-node, so that quorum thresholds
// account for the local replica from the moment of construction. The handler is
// installed on the self-node (if present) to enable in-process dispatch without
// a network round-trip. Panics on configuration errors (invalid addresses,
// duplicate nodes, etc.)
func newInboundManager(myID uint32, opt NodeListOption, sendBuffer uint, onConfigChange func(Configuration), handler stream.RequestHandler) *inboundManager {
	im := &inboundManager{
		myID:           myID,
		nodes:          make(map[uint32]*Node),
		sendBufferSize: sendBuffer,
		handler:        handler,
		onConfigChange: onConfigChange,
		nextClientID:   clientIDStart,
	}
	if opt != nil {
		if _, err := opt.newConfig(im); err != nil {
			panic("gorums: invalid peer configuration: " + err.Error())
		}
	}
	im.rebuildConfig()
	return im
}

// Nodes returns a slice of known peer nodes in order of their IDs.
// Nodes with no active channel (disconnected peers) are still included
// since they are still part of the configuration and may reconnect.
// Client peer nodes are removed when they disconnect.
func (im *inboundManager) Nodes() []*Node {
	im.mu.RLock()
	defer im.mu.RUnlock()
	return slices.SortedFunc(maps.Values(im.nodes), func(a, b *Node) int {
		return cmp.Compare(a.ID(), b.ID())
	})
}

// Config returns a [Configuration] of all connected known peer servers, including this node.
// An empty (non-nil) Configuration is returned if no known peers are connected.
// The returned slice is replaced atomically on each connect/disconnect;
// thus, retaining a reference to an old configuration is safe.
func (im *inboundManager) Config() Configuration {
	if im == nil {
		return nil
	}
	im.mu.RLock()
	defer im.mu.RUnlock()
	return im.config
}

// ClientConfig returns a [Configuration] of all connected clients capable of
// receiving reverse-direction calls from the server.
// An empty (non-nil) Configuration is returned if no client peers are connected.
// The returned slice is replaced atomically on each connect/disconnect;
// thus, retaining a reference to an old configuration is safe.
func (im *inboundManager) ClientConfig() Configuration {
	if im == nil {
		return nil
	}
	im.mu.RLock()
	defer im.mu.RUnlock()
	return im.clientConfig
}

// NodeID returns this server's own NodeID.
func (im *inboundManager) NodeID() uint32 {
	if im == nil {
		return 0
	}
	return im.myID
}

// getMsgID returns the next unique message ID for server-initiated calls.
// The high bit is always set to avoid collision with client-initiated IDs.
// Exhausting the remaining 63-bit counter space requires approximately
// 292,000 years at one million calls per second.
func (im *inboundManager) getMsgID() uint64 {
	return stream.ServerSequenceNumber(im.nextMsgID.Add(1))
}

// newNode creates a peer node for the given id and normalized addr and
// registers it in the manager's node map. This must be called during
// construction before any peers connect, so no locking is needed.
// If id equals myID, a local (in-process) node is created instead of an
// inbound node, enabling direct handler invocation without a network round-trip.
func (im *inboundManager) newNode(id uint32, addr string) (*Node, error) {
	var node *Node
	if id == im.myID && im.handler != nil {
		node = newLocalNode(id, addr, im.getMsgID, im.handler, nil)
	} else {
		node = newInboundNode(id, addr, im.getMsgID, im.handler)
	}
	im.nodes[id] = node
	return node, nil
}

// isKnown returns true if the given NodeID is a known peer.
// Returns false for id == 0 (external clients) or unknown IDs.
func (im *inboundManager) isKnown(id uint32) bool {
	if id == 0 {
		return false
	}
	im.mu.RLock()
	defer im.mu.RUnlock()
	_, ok := im.nodes[id]
	return ok
}

// AcceptPeer accepts an inbound stream and returns the associated peer node
// and a cleanup function. It currently never returns an error, but the signature
// supports error handling to allow for authenticated peer connections in the future.
// The returned [stream.PeerNode] is always non-nil, when err is nil.
//
// If the stream identifies a known peer, AcceptPeer registers it and returns
// that peer's node.
// If the stream includes peer metadata with node ID 0, AcceptPeer creates a
// client peer node with an assigned ID.
// Otherwise, AcceptPeer returns a nilPeerNode that accepts the connection
// without tracking it in the configuration.
func (im *inboundManager) AcceptPeer(streamCtx context.Context, inboundStream stream.BidiStream) (stream.PeerNode, func(), error) {
	noop := func() {}
	if im == nil {
		return &nilPeerNode{stream: inboundStream}, noop, nil
	}
	nilNode := &nilPeerNode{stream: inboundStream, handler: im.handler}
	id := nodeID(streamCtx)
	if im.isKnown(id) {
		// Known peer — register on pre-created node.
		return im.registerPeer(streamCtx, inboundStream, id)
	}
	if id != 0 {
		// Unknown positive ID: misconfigured or unrecognized peer — reject quietly.
		return nilNode, noop, nil
	}
	if !hasPeerMetadata(streamCtx) {
		// Regular client (no gorums-node-id key): accept the connection but do not
		// track it in ClientConfig — the client cannot receive back-channel calls.
		return nilNode, noop, nil
	}
	// Peer-capable anonymous client (gorums-node-id: 0) — create new node with auto-assigned ID.
	return im.acceptClient(streamCtx, inboundStream)
}

// registerPeer attaches an inbound channel to the pre-created Node for the
// given peer and updates the live configuration. If the node already has an
// active channel (e.g., a stale stream from a previous connection), attachStream
// atomically replaces it. The returned cleanup function detaches the channel.
func (im *inboundManager) registerPeer(streamCtx context.Context, inboundStream stream.BidiStream, id uint32) (stream.PeerNode, func(), error) {
	im.mu.Lock()
	defer im.mu.Unlock()
	node := im.nodes[id]
	detach := node.attachStream(streamCtx, inboundStream, im.sendBufferSize)
	im.rebuildConfig()

	return node, func() {
		im.mu.Lock()
		defer im.mu.Unlock()
		_, ok := im.nodes[id]
		if !ok {
			return
		}
		if detach() {
			im.rebuildConfig()
		}
	}, nil
}

// acceptClient creates a new node with an auto-assigned ID for an unknown
// connecting client. The node is added to the nodes map and the configuration
// is rebuilt. The returned cleanup function removes the client node entirely
// when the stream ends (unlike known peers which persist for reconnection).
func (im *inboundManager) acceptClient(streamCtx context.Context, inboundStream stream.BidiStream) (stream.PeerNode, func(), error) {
	im.mu.Lock()
	defer im.mu.Unlock()
	if im.nextClientID == ^uint32(0) {
		return nil, func() {}, errors.New("gorums: dynamic client ID space exhausted")
	}
	id := im.nextClientID
	im.nextClientID++
	node := newInboundNode(id, "client", im.getMsgID, im.handler)
	detach := node.attachStream(streamCtx, inboundStream, im.sendBufferSize)
	im.nodes[id] = node
	im.rebuildConfig()

	return node, func() {
		im.mu.Lock()
		defer im.mu.Unlock()
		_, ok := im.nodes[id]
		if !ok {
			return
		}
		if detach() {
			delete(im.nodes, id)
			im.rebuildConfig()
		}
	}, nil
}

// rebuildConfig rebuilds inbound and client configurations from the current nodes map.
// A node is included in the known Config if it has an active channel (peer connected)
// or if it is myID. A node is included in ClientConfig if it is a connected client peer.
// Callers must hold the lock.
func (im *inboundManager) rebuildConfig() {
	cfg := make(Configuration, 0, len(im.nodes))
	clientCfg := make(Configuration, 0)
	for id, node := range im.nodes {
		if id >= clientIDStart {
			if node.channel.Load() != nil {
				clientCfg = append(clientCfg, node)
			}
		} else {
			if id == im.myID || node.channel.Load() != nil {
				cfg = append(cfg, node)
			}
		}
	}
	OrderedBy(ID).Sort(cfg)
	OrderedBy(ID).Sort(clientCfg)
	im.config = cfg
	im.clientConfig = clientCfg
	if im.onConfigChange != nil {
		im.onConfigChange(cfg)
	}
}

// nilPeerNode implements [stream.PeerNode] for regular clients that have no
// back-channel capability.
type nilPeerNode struct {
	stream  stream.BidiStream
	handler stream.RequestHandler
}

// RouteInbound dispatches all messages as client-initiated requests to the
// registered handler (if any).
func (p *nilPeerNode) RouteInbound(ctx context.Context, msg *stream.Message, release func(), send func(*stream.Message)) {
	if p.handler != nil {
		go p.handler.HandleRequest(msg.AppendToIncomingContext(ctx), msg, release, send)
	} else {
		release()
	}
}

// Enqueue sends the message directly on the inbound stream; send errors are
// absorbed because gRPC cancels the stream context on failure, which causes
// NodeStream to exit.
func (p *nilPeerNode) Enqueue(req stream.Request) { _ = p.stream.Send(req.Msg) }

// compile-time assertion for interface compliance.
var (
	_ stream.PeerAcceptor = (*inboundManager)(nil)
	_ nodeRegistry        = (*inboundManager)(nil)
	_ stream.PeerNode     = (*nilPeerNode)(nil)
)
