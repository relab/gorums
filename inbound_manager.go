package gorums

import (
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

// metadataWithNodeID returns a metadata.MD containing the gorums-node-id key with the given id value.
func metadataWithNodeID(id uint32) metadata.MD {
	return metadata.Pairs(gorumsNodeIDKey, strconv.Format(id, 10))
}

// InboundManager manages server-side awareness of connected peers.
// It is configured at construction time with a fixed set of known peers,
// registers them as they connect, and maintains an auto-updated Configuration
// that can be used for server-initiated quorum calls, multicast, and other
// call types.
//
// When clientPeers is enabled, unknown clients (those without a recognized
// NodeID) are accepted and assigned auto-generated IDs. Client nodes are
// removed from the nodes map when they disconnect.
//
// InboundManager is safe for concurrent use.
type InboundManager struct {
	mu              sync.RWMutex
	myID            uint32              // this server's own NodeID; always present in inboundCfg
	nodes           map[uint32]*Node    // pre-created for known peers; client peers added on connect
	config          Configuration       // auto-updated slice of known peers, sorted by ID
	clientConfig    Configuration       // auto-updated slice of client peers, sorted by ID
	nextMsgID       atomic.Uint64       // counter for server-initiated message IDs
	sendBufferSize  uint                // send buffer size for inbound channels
	onConfigChange  func(Configuration) // optional; called after each known-peer config change
	onClientsChange func(Configuration) // optional; called after each client-peer config change
	clientPeers     bool                // accept unknown clients with auto-assigned IDs
	nextClientID    uint32              // next ID to assign to a client peer
}

// clientIDStart is the starting ID for dynamically assigned client peers.
// Chosen to be high enough to avoid collisions with typical known-peer IDs.
// The available ID space is [clientIDStart, math.MaxUint32], giving approximately
// 4.3 billion unique IDs before exhaustion. acceptClient rejects new peers if the
// counter reaches math.MaxUint32 to prevent silent wraparound.
const clientIDStart = 1 << 20

// newInboundManager creates an InboundManager for this server whose NodeID is myID.
// If opt is non-nil, the InboundManager is configured with the given NodeListOption
// defining the set of known peers. If myID is present in the NodeListOption it is
// immediately included in the Config as the self-node, so that quorum thresholds
// account for the local replica from the moment of construction. If clientPeers is
// true, unknown clients are accepted with auto-assigned IDs. Panics on configuration
// errors (invalid addresses, duplicate nodes, etc.)
func newInboundManager(myID uint32, opt NodeListOption, sendBuffer uint, onConfigChange, onClientsChange func(Configuration), clientPeers bool) *InboundManager {
	im := &InboundManager{
		myID:            myID,
		nodes:           make(map[uint32]*Node),
		sendBufferSize:  sendBuffer,
		onConfigChange:  onConfigChange,
		onClientsChange: onClientsChange,
		clientPeers:     clientPeers,
		nextClientID:    clientIDStart,
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

// Config returns a [Configuration] of all connected known peers, plus this node.
// The returned slice is replaced atomically on each connect/disconnect;
// retaining a reference to an old configuration is safe.
// Returns nil if no peer tracking is configured.
func (im *InboundManager) Config() Configuration {
	im.mu.RLock()
	defer im.mu.RUnlock()
	return im.config
}

// ClientConfig returns a [Configuration] of all connected client peers.
// The returned slice is replaced atomically on each connect/disconnect;
// retaining a reference to an old value is safe.
// Returns nil if no peer tracking is configured.
func (im *InboundManager) ClientConfig() Configuration {
	im.mu.RLock()
	defer im.mu.RUnlock()
	return im.clientConfig
}

// serverIDMask is the high bit of a uint64, used to partition message ID spaces.
// Server-initiated call IDs always have this bit set; client-initiated IDs never do.
// This prevents message ID collisions when requests flow in both directions over the
// same bidirectional gRPC stream (e.g., server handler calling back to a client peer).
const serverIDMask = uint64(1) << 63

// getMsgID returns the next unique message ID for server-initiated calls.
// The high bit is always set to avoid collision with client-initiated IDs.
// Exhausting the remaining 63-bit counter space requires approximately
// 292,000 years at one million calls per second.
func (im *InboundManager) getMsgID() uint64 {
	return im.nextMsgID.Add(1) | serverIDMask
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
//
// For known peers, the pre-created Node is returned. For unknown peers when
// clientPeers is enabled, a new Node is created with an auto-assigned ID.
// For a nil receiver, external (non-replica) clients when client mode is
// disabled, it returns (nil, noop, nil) where noop is a no-op cleanup function.
// The error return is reserved for future use (e.g., credential validation);
// it is currently always nil.
func (im *InboundManager) AcceptPeer(streamCtx context.Context, inboundStream stream.BidiStream) (stream.PeerNode, func(), error) {
	noop := func() {}
	if im == nil {
		return nil, noop, nil
	}
	id := nodeID(streamCtx)
	if im.isKnown(id) {
		// Known peer — register on pre-created node.
		return im.registerPeer(streamCtx, inboundStream, id)
	}
	if !im.clientPeers {
		// External client or unknown peer.
		return nil, noop, nil
	}
	// Client peer — create new node with auto-assigned ID.
	return im.acceptClient(streamCtx, inboundStream)
}

// registerPeer attaches an inbound channel to the pre-created Node for the
// given peer and updates the live configuration. If the node already has an
// active channel (e.g., a stale stream from a previous connection), attachStream
// atomically replaces it. The returned cleanup function detaches the channel.
func (im *InboundManager) registerPeer(streamCtx context.Context, inboundStream stream.BidiStream, id uint32) (stream.PeerNode, func(), error) {
	im.mu.Lock()
	defer im.mu.Unlock()
	node := im.nodes[id]
	node.attachStream(streamCtx, inboundStream, im.sendBufferSize)
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

// acceptClient creates a new node with an auto-assigned ID for an unknown
// connecting client. The node is added to the nodes map and the configuration
// is rebuilt. The returned cleanup function removes the client node entirely
// when the stream ends (unlike known peers which persist for reconnection).
func (im *InboundManager) acceptClient(streamCtx context.Context, inboundStream stream.BidiStream) (stream.PeerNode, func(), error) {
	im.mu.Lock()
	defer im.mu.Unlock()
	if im.nextClientID == ^uint32(0) {
		return nil, func() {}, errors.New("gorums: dynamic client ID space exhausted")
	}
	id := im.nextClientID
	im.nextClientID++
	node := newPeerNode(id, "client", im.getMsgID)
	node.attachStream(streamCtx, inboundStream, im.sendBufferSize)
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

// rebuildConfig rebuilds inbound and client configurations from the current nodes map.
// A node is included in the known Config if it has an active channel (peer connected)
// or if it is myID. A node is included in ClientConfig if it is a connected client peer.
// Callers must hold the lock.
func (im *InboundManager) rebuildConfig() {
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
	if im.onClientsChange != nil {
		im.onClientsChange(clientCfg)
	}
}

// compile-time assertion that InboundManager implements the PeerAcceptor interface.
var _ stream.PeerAcceptor = (*InboundManager)(nil)
