package conn

import (
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
)

// outboundManager maintains a connection pool of nodes on
// which quorum calls can be performed.
type outboundManager struct {
	mu        sync.Mutex
	nodes     []*Node
	lookup    map[uint32]*Node
	closeOnce sync.Once
	logger    *log.Logger
	opts      DialOptions
	nextMsgID atomic.Uint64
}

// newOutboundManager returns a new outboundManager for managing connection to
// nodes added to the manager.
func newOutboundManager(opts ...DialOption) *outboundManager {
	m := &outboundManager{
		lookup: make(map[uint32]*Node),
		opts:   NewDialOptions(),
	}
	for _, opt := range opts {
		opt(&m.opts)
	}
	if m.opts.Logger != nil {
		m.logger = m.opts.Logger
	}
	if m.opts.Backoff != backoff.DefaultConfig {
		m.opts.GRPCDialOpts = append(m.opts.GRPCDialOpts, grpc.WithConnectParams(
			grpc.ConnectParams{Backoff: m.opts.Backoff},
		))
	}
	if m.logger != nil {
		m.logger.Printf("ready")
	}
	return m
}

// Close closes all node connections and any client streams.
func (m *outboundManager) Close() error {
	var err error
	m.closeOnce.Do(func() {
		for _, node := range m.nodes {
			err = errors.Join(err, node.close())
		}
	})
	return err
}

// Node returns the node with the given identifier if present.
func (m *outboundManager) Node(id uint32) (node *Node, found bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	node, found = m.lookup[id]
	return node, found
}

// Nodes returns a slice of each available node. IDs are returned in the same
// order as they were provided when the outboundManager was created.
func (m *outboundManager) Nodes() []*Node {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.nodes
}

func (m *outboundManager) addNode(node *Node) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lookup[node.id] = node
	m.nodes = append(m.nodes, node)
}

func (m *outboundManager) newNode(id uint32, addr string) (*Node, error) {
	if _, found := m.Node(id); found {
		return nil, fmt.Errorf("gorums: node %d already exists", id)
	}
	if id == m.opts.LocalNodeID && m.opts.Handler != nil {
		// Use a local (in-process) node when this ID is our own node and a handler
		// is configured, so this server calls itself without a network round-trip.
		n := newLocalNode(id, addr, m.getMsgID, m.opts.Handler, m)
		m.addNode(n)
		return n, nil
	}
	if m.opts.StreamDedup && m.opts.InboundMgr != nil && id < m.opts.LocalNodeID {
		// A lower-ID peer dials this node, so rather than dial back, this node
		// reuses that peer's inbound connection. It works once the peer connects
		// and fails with ErrStreamDown until then; [Server.WaitForAll] waits for
		// the peer.
		//
		// A dedup node routes its calls onto the borrowed peer's shared channel,
		// so the borrowed peer must be the same process this node addresses: an
		// ID that maps to no known peer, or to a peer at a different address,
		// would silently carry calls to the wrong process. WithPeers derives the
		// peer and outbound sets from one NodeSource, but Config.Extend can add
		// outbound nodes from a different source, so validate the borrow here.
		// Both addresses are already normalized by the node builder.
		peer := m.opts.InboundMgr.knownPeer(id)
		if peer == nil {
			return nil, fmt.Errorf("gorums: stream dedup outbound node %d (%s) is not a configured peer", id, addr)
		}
		if peer.addr != addr {
			return nil, fmt.Errorf("gorums: stream dedup outbound node %d address %s does not match peer address %s", id, addr, peer.addr)
		}
		n := newSharedNode(peer, addr, m)
		m.addNode(n)
		return n, nil
	}
	opts := nodeOptions{
		ID:             id,
		SendBufferSize: m.opts.SendBuffer,
		MsgIDGen:       m.getMsgID,
		Metadata:       m.opts.Metadata,
		DialOpts:       m.opts.GRPCDialOpts,
		RequestHandler: m.opts.Handler,
		// When this node belongs to a server that calls its peers, the peer may
		// reuse this connection for its own calls and cannot re-dial it. If it
		// drops while this node has nothing to send, the peer would stall waiting
		// for the next local send, so re-establish it eagerly. Plain clients
		// reconnect on the next send.
		EagerReconnect: m.opts.InboundMgr != nil,
		Manager:        m,
	}
	if im := m.opts.InboundMgr; im != nil && im.isKnown(id) {
		// Stream-state changes on a dialed peer feed the server's
		// connected-peer view.
		opts.StreamState = im.peerStreamChanged
	}
	n, err := newOutboundNode(addr, opts)
	if err != nil {
		return nil, err
	}
	m.addNode(n)
	return n, nil
}

// validateStreamDedup reports whether the server is correctly configured
// for stream deduplication. It requires a nonzero local node ID that is
// one of the server's own peers.
func (m *outboundManager) validateStreamDedup() error {
	if !m.opts.StreamDedup || m.opts.InboundMgr == nil {
		return nil
	}
	localID := m.opts.LocalNodeID
	if localID == 0 {
		return errors.New("gorums: stream dedup requires a nonzero local node ID")
	}
	if !m.opts.InboundMgr.isKnown(localID) {
		return fmt.Errorf("gorums: stream dedup server peer configuration does not contain local node %d", localID)
	}
	return nil
}

// getMsgID returns a unique message ID for a new RPC from this client's manager.
// Client-initiated IDs never have the high bit set in practice: reaching 2^63
// requires approximately 292,000 years at one million calls per second.
func (m *outboundManager) getMsgID() uint64 {
	return m.nextMsgID.Add(1)
}

// compile-time assertion for interface compliance.
var _ nodeRegistry = (*outboundManager)(nil)
