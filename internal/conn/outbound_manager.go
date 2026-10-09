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
	closed    bool // guarded by mu; set once by Close
	closeOnce sync.Once
	logger    *log.Logger
	opts      DialOptions
	msgIDs    atomic.Uint64
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
		m.opts.GRPCDialOptions = append(m.opts.GRPCDialOptions, grpc.WithConnectParams(
			grpc.ConnectParams{Backoff: m.opts.Backoff},
		))
	}
	if m.logger != nil {
		m.logger.Printf("ready")
	}
	return m
}

// errConfigClosed reports an attempt to extend a closed connection pool.
var errConfigClosed = errors.New("gorums: configuration is closed")

// Close closes all node connections and any client streams. Once Close
// starts, the manager rejects new nodes. Close is idempotent and safe for
// concurrent use; every call returns after the nodes are closed.
func (m *outboundManager) Close() error {
	var err error
	m.closeOnce.Do(func() {
		m.mu.Lock()
		m.closed = true
		nodes := m.nodes
		m.mu.Unlock()
		// Close the nodes outside mu: closing a node waits for its
		// goroutines, and the closed flag stops addNode from adding more.
		for _, node := range nodes {
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

// addNode registers node with the manager. If the manager is closed, addNode
// closes node instead and returns an error, so that no live node is left
// outside the set that Close releases.
func (m *outboundManager) addNode(node *Node) error {
	m.mu.Lock()
	closed := m.closed
	if !closed {
		m.lookup[node.id] = node
		m.nodes = append(m.nodes, node)
	}
	m.mu.Unlock()
	if closed {
		_ = node.close()
		return errConfigClosed
	}
	return nil
}

// isClosed reports whether Close has started.
func (m *outboundManager) isClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}

func (m *outboundManager) newNode(id uint32, addr string) (*Node, error) {
	if _, found := m.Node(id); found {
		return nil, fmt.Errorf("gorums: node %d already exists", id)
	}
	if id == m.opts.LocalNodeID && m.opts.Handler != nil {
		// Use a local (in-process) node when this ID is our own node and a handler
		// is configured, so this server calls itself without a network round-trip.
		n := newLocalNode(id, addr, m.nextMsgID, m.opts.Handler, m)
		if err := m.addNode(n); err != nil {
			return nil, err
		}
		return n, nil
	}
	if m.opts.StreamDedup && m.opts.InboundManager != nil && id < m.opts.LocalNodeID {
		// A lower-ID peer dials this node, and this node sends its calls over
		// that peer's inbound connection. Its calls fail with
		// [stream.ErrStreamDown] until the peer connects; the server's
		// WaitForAll waits for the peer.
		//
		// The borrowed peer must be the process this node addresses, so the
		// borrow requires a known peer with the same address. [Config.Extend]
		// can add outbound nodes from a source other than the peer set. Both
		// addresses are already normalized by the node builder.
		peer := m.opts.InboundManager.knownPeer(id)
		if peer == nil {
			return nil, fmt.Errorf("gorums: stream dedup outbound node %d (%s) is not a configured peer", id, addr)
		}
		if peer.addr != addr {
			return nil, fmt.Errorf("gorums: stream dedup outbound node %d address %s does not match peer address %s", id, addr, peer.addr)
		}
		n := newSharedNode(peer, addr, m)
		if err := m.addNode(n); err != nil {
			return nil, err
		}
		return n, nil
	}
	opts := nodeOptions{
		ID:              id,
		SendBufferSize:  m.opts.SendBufferSize,
		MsgIDGen:        m.nextMsgID,
		Metadata:        m.opts.Metadata,
		GRPCDialOptions: m.opts.GRPCDialOptions,
		Handler:         m.opts.Handler,
		// A configuration that carries a server, as a peer or as a back-channel
		// client, reconnects eagerly: the remote side keeps this node in its
		// connected configuration, or sends its own calls on this stream under
		// stream deduplication, only while the stream is up.
		EagerReconnect: m.opts.InboundManager != nil,
		Manager:        m,
	}
	if im := m.opts.InboundManager; im != nil && im.isKnown(id) {
		// Stream-state changes on a dialed peer feed the server's
		// connected-peer view.
		opts.OnStreamChange = im.onStreamChange
	}
	n, err := newOutboundNode(addr, opts)
	if err != nil {
		return nil, err
	}
	if err := m.addNode(n); err != nil {
		return nil, err
	}
	return n, nil
}

// nextMsgID returns a unique message ID for a new RPC from this client's manager.
// Client-initiated IDs never have the high bit set in practice: reaching 2^63
// requires approximately 292,000 years at one million calls per second.
func (m *outboundManager) nextMsgID() uint64 {
	return m.msgIDs.Add(1)
}

// compile-time assertion for interface compliance.
var _ nodeRegistry = (*outboundManager)(nil)
