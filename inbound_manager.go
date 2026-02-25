package gorums

import (
	"context"
	"maps"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"

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
	id, err := strconv.ParseUint(vals[0], 10, 32)
	if err != nil || id == 0 {
		return 0
	}
	return uint32(id)
}

// InboundManager manages server-side awareness of connected peers.
// It is configured at construction time with a fixed set of known peers,
// registers them as they connect, and maintains an auto-updated Configuration
// that can be used for server-initiated quorum calls, multicast, and other
// call types.
//
// When dynamicPeers is enabled, unknown clients (those without a recognized
// NodeID) are accepted and assigned auto-generated IDs. Dynamic nodes are
// removed from the nodes map when they disconnect.
//
// InboundManager is safe for concurrent use.
type InboundManager struct {
	mu             sync.RWMutex
	myID           uint32              // this server's own NodeID; always present in inboundCfg
	nodes          map[uint32]*Node    // pre-created for known peers; dynamic peers added on connect
	config         Configuration       // auto-updated slice of known peers, sorted by ID
	dynamicConfig  Configuration       // auto-updated slice of dynamic peers, sorted by ID
	nextMsgID      atomic.Uint64       // counter for server-initiated message IDs
	sendBufferSize uint                // send buffer size for inbound channels
	onConfigChange func(Configuration) // optional; called after each config change
	dynamicPeers   bool                // accept unknown clients with auto-assigned IDs
	nextDynamicID  uint32              // next ID to assign to a dynamic peer
}

// dynamicIDStart is the starting ID for dynamically assigned peers.
// Chosen to be high enough to avoid collisions with typical known-peer IDs.
const dynamicIDStart = 1 << 20

// newInboundManager creates an InboundManager for this server whose NodeID is myID.
// If opt is non-nil, the InboundManager is configured with the given NodeListOption
// defining the set of known peers. If myID is present in the NodeListOption it is
// immediately included in the Config as the self-node, so that quorum thresholds
// account for the local replica from the moment of construction. If dynamicPeers is
// true, unknown clients are accepted with auto-assigned IDs. Panics on configuration
// errors (invalid addresses, duplicate nodes, etc.)
func newInboundManager(myID uint32, opt NodeListOption, sendBuffer uint, onConfigChange func(Configuration), dynamicPeers bool) *InboundManager {
	im := &InboundManager{
		myID:           myID,
		nodes:          make(map[uint32]*Node),
		sendBufferSize: sendBuffer,
		onConfigChange: onConfigChange,
		dynamicPeers:   dynamicPeers,
		nextDynamicID:  dynamicIDStart,
	}
	if opt != nil {
		if err := opt.newServerConfig(im); err != nil {
			panic("gorums: invalid peer configuration: " + err.Error())
		}
	}
	im.rebuildConfig()
	return im
}

// KnownIDs returns the sorted list of NodeIDs the manager was configured with.
func (im *InboundManager) KnownIDs() []uint32 {
	im.mu.RLock()
	defer im.mu.RUnlock()
	return slices.Sorted(maps.Keys(im.nodes))
}

// Config returns a Configuration of all connected known peers, plus this node.
// The returned slice is replaced atomically on each connect/disconnect;
// retaining a reference to an old configuration is safe.
func (im *InboundManager) Config() Configuration {
	im.mu.RLock()
	defer im.mu.RUnlock()
	return im.config
}

// DynamicConfig returns a Configuration of all connected dynamic peers.
// The returned slice is replaced atomically on each connect/disconnect;
// retaining a reference to an old value is safe.
func (im *InboundManager) DynamicConfig() Configuration {
	im.mu.RLock()
	defer im.mu.RUnlock()
	return im.dynamicConfig
}

// getMsgID returns the next unique message ID for server-initiated calls.
func (im *InboundManager) getMsgID() uint64 {
	return im.nextMsgID.Add(1)
}

// newNode creates a peer node for the given id and normalized addr and
// registers it in the manager's node map. This must be called during
// construction before any peers connect, so no locking is needed.
func (im *InboundManager) newNode(id uint32, addr string) {
	im.nodes[id] = newPeerNode(id, addr, im.getMsgID)
}

// isKnown returns true if the given NodeID is a known peer.
// Returns false for id == 0 (external clients) or unknown IDs.
func (im *InboundManager) isKnown(id uint32) bool {
	if id == 0 {
		return false
	}
	im.mu.RLock()
	defer im.mu.RUnlock()
	_, ok := im.nodes[id]
	return ok
}

// AcceptPeer identifies a connecting peer by its NodeID metadata, registers
// it if recognized, and returns the peer's Node along with a cleanup function
// that should be deferred to unregister the peer when the stream ends.
// The handlers map is propagated to the peer's router so that it can dispatch
// server-initiated requests.
//
// For known peers, the pre-created Node is returned. For unknown peers when
// dynamicPeers is enabled, a new Node is created with an auto-assigned ID.
// For a nil receiver, external (non-replica) clients when dynamic mode is
// disabled, it returns (nil, noop, nil) where noop is a no-op cleanup function.
// The error return is reserved for future use (e.g., credential validation);
// it is currently always nil.
func (im *InboundManager) AcceptPeer(ctx context.Context, inboundStream stream.BidiStream, handlers map[string]stream.Handler) (stream.PeerNode, func(), error) {
	noop := func() {}
	if im == nil {
		return nil, noop, nil
	}
	id := nodeID(ctx)
	if im.isKnown(id) {
		// Known peer — register on pre-created node.
		return im.registerPeer(id, inboundStream, ctx, handlers)
	}
	if !im.dynamicPeers {
		// External client or unknown peer.
		return nil, noop, nil
	}
	// Dynamic peer — create new node with auto-assigned ID.
	return im.acceptDynamic(inboundStream, ctx, handlers)
}

// registerPeer attaches an inbound channel to the pre-created Node for the
// given peer and updates the live configuration. If the node already has an
// active channel (e.g., a stale stream from a previous connection), attachStream
// atomically replaces it. The returned cleanup function detaches the channel.
func (im *InboundManager) registerPeer(id uint32, inboundStream stream.BidiStream, streamCtx context.Context, handlers map[string]stream.Handler) (stream.PeerNode, func(), error) {
	im.mu.Lock()
	defer im.mu.Unlock()
	node := im.nodes[id]
	node.setHandlers(handlers)
	node.attachStream(inboundStream, streamCtx, im.sendBufferSize)
	im.rebuildConfig()

	return node, func() {
		im.mu.Lock()
		defer im.mu.Unlock()
		node, ok := im.nodes[id]
		if !ok {
			return
		}
		node.detachStream()
		im.rebuildConfig()
	}, nil
}

// acceptDynamic creates a new node with an auto-assigned ID for an unknown
// connecting client. The node is added to the nodes map and the configuration
// is rebuilt. The returned cleanup function removes the dynamic node entirely
// when the stream ends (unlike known peers which persist for reconnection).
func (im *InboundManager) acceptDynamic(inboundStream stream.BidiStream, streamCtx context.Context, handlers map[string]stream.Handler) (stream.PeerNode, func(), error) {
	im.mu.Lock()
	defer im.mu.Unlock()
	id := im.nextDynamicID
	im.nextDynamicID++
	node := newPeerNode(id, "dynamic", im.getMsgID)
	node.setHandlers(handlers)
	node.attachStream(inboundStream, streamCtx, im.sendBufferSize)
	im.nodes[id] = node
	im.rebuildConfig()

	return node, func() {
		im.mu.Lock()
		defer im.mu.Unlock()
		node, ok := im.nodes[id]
		if !ok {
			return
		}
		node.detachStream()
		delete(im.nodes, id)
		im.rebuildConfig()
	}, nil
}

// rebuildConfig rebuilds inbound and dynamic configurations from the current nodes map.
// A node is included in the known Config if it has an active channel (peer connected)
// or if it is myID. A node is included in DynamicConfig if it is a connected dynamic peer.
// Callers must hold the lock.
func (im *InboundManager) rebuildConfig() {
	cfg := make(Configuration, 0, len(im.nodes))
	dynamicCfg := make(Configuration, 0)
	for id, node := range im.nodes {
		if id >= dynamicIDStart {
			if node.channel.Load() != nil {
				dynamicCfg = append(dynamicCfg, node)
			}
		} else {
			if id == im.myID || node.channel.Load() != nil {
				cfg = append(cfg, node)
			}
		}
	}
	OrderedBy(ID).Sort(cfg)
	OrderedBy(ID).Sort(dynamicCfg)
	im.config = cfg
	im.dynamicConfig = dynamicCfg
	if im.onConfigChange != nil {
		im.onConfigChange(cfg) // Note: only calling onConfigChange for known peers currently
	}
}

// compile-time assertion that InboundManager implements the PeerAcceptor interface.
var _ stream.PeerAcceptor = (*InboundManager)(nil)
