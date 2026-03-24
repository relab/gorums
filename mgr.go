package gorums

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
	opts      dialOptions
	nextMsgID uint64
}

// Deprecated: Manager is an alias for outboundManager and will be removed in a
// future release. Use [Configuration] instead.
type Manager = outboundManager

// newOutboundManager returns a new outboundManager for managing connection to
// nodes added to the manager.
func newOutboundManager(opts ...DialOption) *outboundManager {
	m := &outboundManager{
		lookup: make(map[uint32]*Node),
		opts:   newDialOptions(),
	}
	for _, opt := range opts {
		opt(&m.opts)
	}
	if m.opts.logger != nil {
		m.logger = m.opts.logger
	}
	if m.opts.backoff != backoff.DefaultConfig {
		m.opts.grpcDialOpts = append(m.opts.grpcDialOpts, grpc.WithConnectParams(
			grpc.ConnectParams{Backoff: m.opts.backoff},
		))
	}
	if m.logger != nil {
		m.logger.Printf("ready")
	}
	return m
}

// Deprecated: Use [NewConfig] instead.
func NewManager(opts ...DialOption) *Manager {
	return newOutboundManager(opts...)
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
// order as they were provided in the creation of the Manager.
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
		return nil, fmt.Errorf("node %d already exists", id)
	}
	// Use a local (in-process) node when this ID is our own local node
	// and a handler is configured (symmetric peer configuration).
	if id == m.opts.localNodeID && m.opts.handler != nil {
		n := newLocalNode(id, addr, m.getMsgID, m.opts.handler, m)
		m.addNode(n)
		return n, nil
	}
	opts := nodeOptions{
		ID:             id,
		SendBufferSize: m.opts.sendBuffer,
		MsgIDGen:       m.getMsgID,
		Metadata:       m.opts.metadata,
		PerNodeMD:      m.opts.perNodeMD,
		DialOpts:       m.opts.grpcDialOpts,
		RequestHandler: m.opts.handler,
		Manager:        m,
	}
	n, err := newOutboundNode(addr, opts)
	if err != nil {
		return nil, err
	}
	m.addNode(n)
	return n, nil
}

// getMsgID returns a unique message ID for a new RPC from this client's manager.
// Client-initiated IDs never have the high bit set in practice: reaching 2^63
// requires approximately 292,000 years at one million calls per second.
func (m *outboundManager) getMsgID() uint64 {
	return atomic.AddUint64(&m.nextMsgID, 1)
}

// compile-time assertion for interface compliance.
var _ nodeRegistry = (*outboundManager)(nil)
