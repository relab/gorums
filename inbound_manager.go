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
// InboundManager is safe for concurrent use.
type InboundManager struct {
	mu             sync.RWMutex
	myID           uint32              // this server's own NodeID; always present in inboundCfg
	nodes          map[uint32]*Node    // pre-created for all known peers; channel is nil until connected
	inboundCfg     Configuration       // auto-updated slice, sorted by ID
	nextMsgID      atomic.Uint64       // counter for server-initiated message IDs
	sendBufferSize uint                // send buffer size for inbound channels
	onConfigChange func(Configuration) // optional; called after each config change
}

// NewInboundManager creates an InboundManager for this server whose NodeID is myID,
// configured with the given NodeListOption and optional InboundManagerOptions.
// If myID is present in the NodeListOption it is immediately included in the
// InboundConfig as the self-node, so that quorum thresholds account for the local
// replica from the moment of construction. The optional InboundManagerOptions
// (e.g. WithInboundSendBufferSize) are applied before nodes are created, so they
// take effect for all pre-created nodes.
func NewInboundManager(myID uint32, opt NodeListOption, opts ...InboundManagerOption) (*InboundManager, error) {
	im := &InboundManager{
		myID:  myID,
		nodes: make(map[uint32]*Node),
	}
	for _, o := range opts {
		o(im)
	}
	if err := opt.newServerConfig(im); err != nil {
		return nil, err
	}
	im.rebuildConfig()
	return im, nil
}

// KnownIDs returns the sorted list of NodeIDs the manager was configured with.
func (im *InboundManager) KnownIDs() []uint32 {
	im.mu.RLock()
	defer im.mu.RUnlock()
	return slices.Sorted(maps.Keys(im.nodes))
}

// InboundConfig returns a Configuration of all currently connected peers.
// The returned slice is replaced atomically on each connect/disconnect;
// retaining a reference to an old value is safe.
func (im *InboundManager) InboundConfig() Configuration {
	im.mu.RLock()
	defer im.mu.RUnlock()
	return im.inboundCfg
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

// isKnown reports whether the given NodeID is in the configured peer set.
func (im *InboundManager) isKnown(id uint32) bool {
	im.mu.RLock()
	defer im.mu.RUnlock()
	_, ok := im.nodes[id]
	return ok
}

// AcceptPeer identifies a connecting peer by its NodeID metadata, registers
// it if recognized, and returns the peer's Node along with a cleanup function
// that should be deferred to unregister the peer when the stream ends.
// Returns (nil, nil, nil) for external (non-replica) clients or unknown peers.
// The error return is reserved for future use (e.g., credential validation);
// it is currently always nil.
func (im *InboundManager) AcceptPeer(ctx context.Context, inboundStream stream.BidiStream) (stream.PeerNode, func(), error) {
	id := nodeID(ctx)
	if id == 0 {
		return nil, nil, nil // External client (non-replica).
	}
	if !im.isKnown(id) {
		return nil, nil, nil // Unknown peer.
	}
	im.registerPeer(id, inboundStream, ctx)
	return im.nodes[id], func() { im.unregisterPeer(id) }, nil
}

// registerPeer attaches an inbound channel to the pre-created Node for the
// given peer and updates the live configuration. If the node already has an
// active channel (e.g., a stale stream from a previous connection), attachStream
// atomically replaces it.
func (im *InboundManager) registerPeer(id uint32, inboundStream stream.BidiStream, streamCtx context.Context) {
	im.mu.Lock()
	defer im.mu.Unlock()
	node := im.nodes[id]
	node.attachStream(inboundStream, streamCtx, im.sendBufferSize)
	im.rebuildConfig()
}

// unregisterPeer detaches the inbound channel for the given peer and updates
// the live configuration. It is safe to call for unknown or already-detached
// peers (the call is a no-op in those cases).
func (im *InboundManager) unregisterPeer(id uint32) {
	im.mu.Lock()
	defer im.mu.Unlock()
	node, ok := im.nodes[id]
	if !ok {
		return
	}
	node.detachStream()
	im.rebuildConfig()
}

// rebuildConfig rebuilds inbound configuration from the current nodes map.
// A node is included if it has an active channel (peer connected) or if it
// is the self-node (myID), which is always present so that quorum thresholds
// account for the local replica. Callers must hold the lock.
func (im *InboundManager) rebuildConfig() {
	cfg := make(Configuration, 0, len(im.nodes))
	for _, node := range im.nodes {
		if node.id == im.myID || node.channel.Load() != nil {
			cfg = append(cfg, node)
		}
	}
	OrderedBy(ID).Sort(cfg)
	im.inboundCfg = cfg
	if im.onConfigChange != nil {
		im.onConfigChange(cfg)
	}
}

// compile-time assertion that InboundManager implements the PeerAcceptor interface.
var _ stream.PeerAcceptor = (*InboundManager)(nil)
