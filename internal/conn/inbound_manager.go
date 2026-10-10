package conn

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/relab/gorums/internal/stream"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// gorumsNodeIDKey carries the peer-asserted Gorums node ID in gRPC metadata.
const gorumsNodeIDKey = "gorums-node-id"

// errSelfNodeIDStream is returned by [InboundManager.AcceptPeer] when an
// inbound stream presents this server's own node ID. It terminates the RPC, so
// the misconfigured peer observes the rejection. The InvalidArgument code
// marks it as a client configuration error that a retry does not fix.
var errSelfNodeIDStream = status.Error(codes.InvalidArgument, "gorums: inbound stream claims the server's own node ID")

// nodeID extracts the NodeID from the gorums-node-id metadata key in ctx.
// It returns 0 if the key is absent, empty, or not a valid ID greater than zero.
func nodeID(ctx context.Context) ID {
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
	return ID(id)
}

// hasPeerMetadata reports whether ctx contains the gorums-node-id metadata key,
// regardless of its value. A client that sends this key (even with value "0")
// has declared itself capable of receiving back-channel calls. Regular clients
// (those dialing without a back-channel server) never send this key.
func hasPeerMetadata(ctx context.Context) bool {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return false
	}
	return len(md.Get(gorumsNodeIDKey)) > 0
}

// MetadataWithNodeID returns a metadata.MD containing the gorums-node-id key with the given id value.
func MetadataWithNodeID(id ID) metadata.MD {
	return metadata.Pairs(gorumsNodeIDKey, strconv.FormatUint(uint64(id), 10))
}

// InboundManager manages server-side awareness of connected peers. It is
// configured at construction time with a fixed set of known peers, registers
// them as they connect, and maintains an auto-updated [Config] that
// can be used for server-initiated quorum calls, multicast, and other call types.
//
// Clients that specify node ID 0 in their metadata are assumed to be capable
// of receiving back-channel calls from the server. These clients are
// accepted with auto-generated IDs and included in the client configuration.
// Client nodes are removed from clientNodes when they disconnect, while
// known peer nodes persist in knownNodes to allow for reconnection.
//
// InboundManager is safe for concurrent use.
type InboundManager struct {
	mu               sync.RWMutex
	myID             ID                    // this server's own NodeID; always present in inboundPeers
	knownNodes       map[ID]*Node          // pre-created configured peers, including self when configured
	clientNodes      map[ID]*Node          // dynamically assigned peer-capable clients
	peerConfig       Config                // the server's peer Config; set once by SetPeerConfig after NewConfig builds it
	connectedPeers   Config                // auto-updated connectivity-filtered subset of peerConfig, sorted by ID
	inboundPeers     Config                // auto-updated slice of known peers with an inbound stream, sorted by ID
	connectedClients Config                // auto-updated slice of client peers, sorted by ID
	msgIDs           atomic.Uint64         // counter for server-initiated message IDs
	sendBufferSize   uint                  // send buffer size for inbound channels
	dispatchSize     uint                  // request dispatch queue size for inbound channels
	handler          stream.RequestHandler // handler for dispatching incoming requests on all inbound nodes
	onConfigChange   func(Config)          // optional; called after each connected-peer config change
	nextClientID     uint64                // next candidate ID for a client peer; uint64 represents exhaustion
	configCh         chan struct{}         // closed and replaced on each connectedPeers/connectedClients change; protected by mu
	stopCh           chan struct{}         // closed on shutdown to unblock waiters; never replaced
	stopOnce         sync.Once             // ensures stopCh is closed exactly once
}

// ClientIDStart is the starting ID for dynamically assigned client peers.
// Chosen to keep dynamically assigned IDs away from typical known-peer IDs.
// The available ID space is [ClientIDStart, math.MaxUint32], giving approximately
// 4.3 billion candidate IDs before exhaustion. Configured peers may use IDs in
// this range; the allocator skips every occupied known-peer or client ID.
const ClientIDStart = 1 << 20

// NewInboundManager creates an InboundManager for this server whose NodeID is myID.
// If peerNodes is non-nil, the InboundManager is configured with the given NodeSource
// defining the set of known peers. If myID is present in the NodeSource it is
// immediately included in the Config as the self-node, so that quorum thresholds
// account for the local replica from the moment of construction. The handler is
// installed on the self-node (if present) to enable in-process dispatch without
// a network round-trip. Inbound channels use sendBuffer and dispatchSize as
// their send queue and request dispatch capacities; a dispatchSize of 0
// selects the default. It returns an error if peerNodes is invalid, for
// example because of an invalid address or a duplicate node.
func NewInboundManager(myID ID, peerNodes NodeSource, sendBufferSize, dispatchSize uint, onConfigChange func(Config), handler stream.RequestHandler) (*InboundManager, error) {
	im := &InboundManager{
		myID:           myID,
		knownNodes:     make(map[ID]*Node),
		clientNodes:    make(map[ID]*Node),
		sendBufferSize: sendBufferSize,
		dispatchSize:   dispatchSize,
		handler:        handler,
		onConfigChange: onConfigChange,
		nextClientID:   ClientIDStart,
		configCh:       make(chan struct{}),
		stopCh:         make(chan struct{}),
	}
	if peerNodes != nil {
		if _, err := peerNodes.newConfig(im); err != nil {
			return nil, err
		}
	}
	im.rebuildConfig()
	return im, nil
}

// Nodes returns a slice of known peer nodes in order of their IDs.
// Nodes with no active channel (disconnected peers) are still included
// since they are still part of the configuration and may reconnect.
// Client peer nodes are removed when they disconnect.
func (im *InboundManager) Nodes() []*Node {
	im.mu.RLock()
	defer im.mu.RUnlock()
	return slices.SortedFunc(maps.Values(im.knownNodes), func(a, b *Node) int {
		return cmp.Compare(a.ID(), b.ID())
	})
}

// ConnectedPeers returns the current connected-peer [Config]; see
// [github.com/relab/gorums.Server.ConnectedPeers]. Before [InboundManager.SetPeerConfig]
// installs a peer configuration, it returns the inbound view.
func (im *InboundManager) ConnectedPeers() Config {
	if im == nil {
		return nil
	}
	im.mu.RLock()
	defer im.mu.RUnlock()
	return im.connectedPeers
}

// SetPeerConfig installs the server's peer [Config], from which the
// connected-peer view is derived. It is called once by [github.com/relab/gorums.NewServer] after the
// peer configuration is built; stream-state changes observed before that are
// picked up by the rebuild here.
func (im *InboundManager) SetPeerConfig(cfg Config) {
	im.mu.Lock()
	defer im.mu.Unlock()
	im.peerConfig = cfg
	im.rebuildConfig()
}

// onStreamChange rebuilds the connected-peer view after a dialed peer's
// outbound stream came up or went down. It is registered as the
// stream-change callback for the server's outbound peer nodes; the new state
// is read directly from the nodes during the rebuild.
func (im *InboundManager) onStreamChange() {
	im.mu.Lock()
	defer im.mu.Unlock()
	im.rebuildConfig()
}

// ConnectedClients returns the current connected-client [Config]; see
// [github.com/relab/gorums.Server.ConnectedClients].
func (im *InboundManager) ConnectedClients() Config {
	if im == nil {
		return nil
	}
	im.mu.RLock()
	defer im.mu.RUnlock()
	return im.connectedClients
}

// NodeID returns this server's own nodeID.
func (im *InboundManager) NodeID() ID {
	if im == nil {
		return 0
	}
	return im.myID
}

// nextMsgID returns the next unique message ID for server-initiated calls.
// The high bit is always set to avoid collision with client-initiated IDs.
// Exhausting the remaining 63-bit counter space requires approximately
// 292,000 years at one million calls per second.
func (im *InboundManager) nextMsgID() uint64 {
	return stream.ServerSequenceNumber(im.msgIDs.Add(1))
}

// newNode creates a peer node for the given id and normalized addr and
// registers it in the manager's node map. This must be called during
// construction before any peers connect, so no locking is needed.
// If id equals myID, a local (in-process) node is created instead of an
// inbound node, enabling direct handler invocation without a network round-trip.
func (im *InboundManager) newNode(id ID, addr string) (*Node, error) {
	var node *Node
	if id == im.myID && im.handler != nil {
		node = newLocalNode(id, addr, im.nextMsgID, im.handler, nil)
	} else {
		node = newInboundNode(id, addr, im.nextMsgID)
	}
	im.knownNodes[id] = node
	return node, nil
}

// knownPeer returns the pre-created inbound node for a known peer, or nil if
// id is not part of the server's peer configuration. The node is returned
// regardless of whether the peer is currently connected: known-peer nodes
// exist from construction, so a dedup outbound configuration can borrow a
// peer's channel slot before the peer first connects. The returned node's
// channel reference is shared, so channel attachments and replacements remain
// visible to holders.
func (im *InboundManager) knownPeer(id ID) *Node {
	if im == nil {
		return nil
	}
	im.mu.RLock()
	defer im.mu.RUnlock()
	return im.knownNodes[id]
}

// isKnown returns true if the given NodeID is a known peer.
// Returns false for id == 0 (external clients) or unknown IDs.
func (im *InboundManager) isKnown(id ID) bool {
	if id == 0 {
		return false
	}
	im.mu.RLock()
	defer im.mu.RUnlock()
	_, ok := im.knownNodes[id]
	return ok
}

// AcceptPeer accepts an inbound stream and returns its channel and a cleanup
// function that closes the channel. It returns [errSelfNodeIDStream] for a
// stream that presents this server's own node ID.
//
// If the stream identifies a known peer, AcceptPeer attaches the channel to
// that peer's node. If the stream includes peer metadata with node ID 0,
// AcceptPeer creates a client peer node with an assigned ID. Otherwise, the
// channel serves the connection without being tracked in any configuration.
func (im *InboundManager) AcceptPeer(streamCtx context.Context, inboundStream stream.BidiStream) (*stream.InboundChannel, func(), error) {
	noop := func() {}
	if im == nil {
		return untrackedChannel(streamCtx, inboundStream, stream.InboundOptions{SendBufferSize: DefaultSendBufferSize})
	}
	id := nodeID(streamCtx)
	if im.myID != 0 && id == im.myID {
		// A stream presenting this server's own node ID is rejected, so the
		// self-node keeps its in-process channel (see newLocalNode). The error
		// terminates the RPC in [stream.Server.NodeStream] before any
		// connection callback or application handler runs, which reports the
		// misconfigured peer at the connection boundary. The self-node stays
		// in the configuration.
		return nil, noop, errSelfNodeIDStream
	}
	if im.isKnown(id) {
		// Known peer — register on pre-created node.
		return im.registerPeer(streamCtx, inboundStream, id)
	}
	if id != 0 {
		// Unknown positive ID: misconfigured or unrecognized peer — serve it untracked.
		return untrackedChannel(streamCtx, inboundStream, im.inboundOptions())
	}
	if !hasPeerMetadata(streamCtx) {
		// Regular client (no gorums-node-id key): accept the connection but do not
		// track it in ConnectedClients — the client cannot receive back-channel calls.
		return untrackedChannel(streamCtx, inboundStream, im.inboundOptions())
	}
	// Peer-capable anonymous client (gorums-node-id: 0) — create new node with auto-assigned ID.
	return im.acceptClient(streamCtx, inboundStream)
}

// registerPeer attaches an inbound channel to the pre-created Node for the
// given peer and updates the live configuration. If the node already has a live
// stream (e.g., during connection churn), attachStream installs the new channel
// as active while keeping the prior one live until its own stream ends, so the
// node never goes dark mid-handover; see [Node.attachStream]. The returned
// cleanup function detaches and closes this registration's channel.
func (im *InboundManager) registerPeer(streamCtx context.Context, inboundStream stream.BidiStream, id ID) (*stream.InboundChannel, func(), error) {
	im.mu.Lock()
	defer im.mu.Unlock()
	node := im.knownNodes[id]
	newCh, detach := node.attachStream(streamCtx, inboundStream, im.inboundOptions())
	im.rebuildConfig()

	return newCh, func() {
		im.mu.Lock()
		if _, ok := im.knownNodes[id]; ok && detach() {
			im.rebuildConfig()
		}
		im.mu.Unlock()
		// Closing waits for the channel's send loop, which may wait on the
		// peer, so it runs without holding im.mu.
		_ = newCh.Close()
	}, nil
}

// acceptClient creates a new node with an auto-assigned ID for an unknown
// connecting client. The node is added to clientNodes and the configuration
// is rebuilt. The returned cleanup function removes the client node entirely
// when the stream ends (unlike known peers which persist for reconnection).
func (im *InboundManager) acceptClient(streamCtx context.Context, inboundStream stream.BidiStream) (*stream.InboundChannel, func(), error) {
	im.mu.Lock()
	defer im.mu.Unlock()
	id, err := im.nextAvailableClientID()
	if err != nil {
		return nil, func() {}, err
	}
	node := newInboundNode(id, "client", im.nextMsgID)
	newCh, detach := node.attachStream(streamCtx, inboundStream, im.inboundOptions())
	im.clientNodes[id] = node
	im.rebuildConfig()

	return newCh, func() {
		im.mu.Lock()
		if _, ok := im.clientNodes[id]; ok && detach() {
			delete(im.clientNodes, id)
			im.rebuildConfig()
		}
		im.mu.Unlock()
		// Closing waits for the channel's send loop, which may wait on the
		// peer, so it runs without holding im.mu.
		_ = newCh.Close()
	}, nil
}

// nextAvailableClientID returns the next unoccupied dynamic client ID.
// The caller must hold im.mu.
func (im *InboundManager) nextAvailableClientID() (ID, error) {
	const maxNodeID = uint64(1<<32 - 1)
	for im.nextClientID <= maxNodeID {
		id := ID(im.nextClientID)
		im.nextClientID++
		if _, exists := im.knownNodes[id]; exists {
			continue
		}
		if _, exists := im.clientNodes[id]; exists {
			continue
		}
		return id, nil
	}
	return 0, fmt.Errorf("gorums: dynamic client ID space exhausted")
}

// rebuildConfig rebuilds the inbound, client, and connected-peer
// configurations from their sources. A known peer is in the inbound Config if
// it has an active channel (the peer opened a stream to this server) or if it
// is myID; a client is in the client Config while its stream lives. The
// connected-peer Config is the subset of the installed peer configuration
// whose nodes can currently carry calls; before a peer configuration is
// installed it falls back to the inbound view. Callers must hold the lock.
func (im *InboundManager) rebuildConfig() {
	inboundCfg := make(Config, 0, len(im.knownNodes))
	for id, node := range im.knownNodes {
		if id == im.myID || node.activeChannel() != nil {
			inboundCfg = append(inboundCfg, node)
		}
	}
	clientCfg := make(Config, 0, len(im.clientNodes))
	for _, node := range im.clientNodes {
		if node.activeChannel() != nil {
			clientCfg = append(clientCfg, node)
		}
	}
	slices.SortFunc(inboundCfg, ByID)
	slices.SortFunc(clientCfg, ByID)
	im.inboundPeers = inboundCfg
	im.connectedClients = clientCfg

	cfg := inboundCfg
	if im.peerConfig != nil {
		cfg = make(Config, 0, len(im.peerConfig))
		for _, node := range im.peerConfig {
			if node.ID() == im.myID || node.isUp() {
				cfg = append(cfg, node)
			}
		}
		slices.SortFunc(cfg, ByID)
	}
	cfgChanged := !slices.Equal(im.connectedPeers, cfg)
	im.connectedPeers = cfg
	if cfgChanged && im.onConfigChange != nil {
		im.onConfigChange(cfg)
	}
	// Broadcast config change to all waiters.
	close(im.configCh)
	im.configCh = make(chan struct{})
}

// checkConfig checks the condition under the read lock and returns the
// current broadcast channel if the condition is not yet met.
func (im *InboundManager) checkConfig(cond func() bool) (met bool, ch <-chan struct{}) {
	im.mu.RLock()
	defer im.mu.RUnlock()
	if cond() {
		return true, nil
	}
	return false, im.configCh
}

// waitForConfig blocks until cond returns true or until ctx is cancelled
// or the InboundManager is closed. The cond function is called while the
// read lock is held, so it must not acquire any additional locks.
func (im *InboundManager) waitForConfig(ctx context.Context, cond func() bool) error {
	for {
		met, ch := im.checkConfig(cond)
		if met {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-im.stopCh:
			return ErrStopped
		case <-ch:
		}
	}
}

// WaitForPeers blocks until cond returns true for the current connected-peer
// [Config]; see [github.com/relab/gorums.Server.WaitForPeers]. cond runs under im.mu, so it must not
// call back into im or otherwise acquire additional locks.
func (im *InboundManager) WaitForPeers(ctx context.Context, cond func(Config) bool) error {
	return im.waitForConfig(ctx, func() bool {
		return cond(im.connectedPeers)
	})
}

// WaitForClients blocks until cond returns true for the current
// connected-client [Config]; see [github.com/relab/gorums.Server.WaitForClients]. cond runs under
// im.mu, so it must not call back into im or otherwise acquire additional
// locks.
func (im *InboundManager) WaitForClients(ctx context.Context, cond func(Config) bool) error {
	return im.waitForConfig(ctx, func() bool {
		return cond(im.connectedClients)
	})
}

// InboundPeers returns the known peers that currently have an inbound stream,
// plus the local node, sorted by ID. This is the inbound-stream view that
// white-box server tests observe directly; it is distinct from the
// connectivity-filtered [InboundManager.ConnectedPeers] view.
func (im *InboundManager) InboundPeers() Config {
	im.mu.RLock()
	defer im.mu.RUnlock()
	return im.inboundPeers
}

// WaitForInbound blocks until cond returns true for the current inbound peer
// view (the same view as [InboundManager.InboundPeers]), or until ctx is
// cancelled or the manager is closed. cond runs under im.mu, so it must not
// call back into im or otherwise acquire additional locks.
func (im *InboundManager) WaitForInbound(ctx context.Context, cond func(Config) bool) error {
	return im.waitForConfig(ctx, func() bool {
		return cond(im.inboundPeers)
	})
}

// Close signals all waiters to stop and prevents new waits from blocking.
// Called from [github.com/relab/gorums.Server.Stop] and [github.com/relab/gorums.Server.GracefulStop].
func (im *InboundManager) Close() {
	im.stopOnce.Do(func() { close(im.stopCh) })
}

// inboundOptions returns the options for the manager's inbound channels.
func (im *InboundManager) inboundOptions() stream.InboundOptions {
	return stream.InboundOptions{
		SendBufferSize: im.sendBufferSize,
		DispatchSize:   im.dispatchSize,
		Handler:        im.handler,
	}
}

// untrackedChannel returns a channel for a stream that is not tracked in any
// configuration, and a cleanup function that closes it. Replies on it wait for
// send queue space.
func untrackedChannel(streamCtx context.Context, inboundStream stream.BidiStream, opts stream.InboundOptions) (*stream.InboundChannel, func(), error) {
	opts.WaitingReplies = true
	ch := stream.NewInboundChannel(streamCtx, 0, inboundStream, opts)
	return ch, func() { _ = ch.Close() }, nil
}

// compile-time assertion for interface compliance.
var (
	_ stream.PeerAcceptor = (*InboundManager)(nil)
	_ nodeRegistry        = (*InboundManager)(nil)
)
