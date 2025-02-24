package gorums

import (
	"cmp"
	"fmt"
	"log"
	"sync"
	"sync/atomic"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
)

// RawManager maintains a connection pool of nodes on
// which quorum calls can be performed.
//
// This struct is intended to be used by generated code.
// You should use the generated `Manager` struct instead.
type RawManager[idType cmp.Ordered] struct {
	mu        sync.Mutex
	nodes     []*RawNode[idType]
	lookup    map[idType]*RawNode[idType]
	closeOnce sync.Once
	logger    *log.Logger
	opts      managerOptions[idType]
	nextMsgID uint64
}

// NewRawManager returns a new RawManager for managing connection to nodes added
// to the manager. This function accepts manager options used to configure
// various aspects of the manager. This function is meant for internal use.
// You should use the `NewManager` function in the generated code instead.
func NewRawManager[idType cmp.Ordered](opts ...ManagerOption[idType]) *RawManager[idType] {
	m := &RawManager[idType]{
		lookup: make(map[idType]*RawNode[idType]),
		opts:   newManagerOptions[idType](),
	}
	for _, opt := range opts {
		opt(&m.opts)
	}
	if m.opts.logger != nil {
		m.logger = m.opts.logger
	}
	m.opts.grpcDialOpts = append(m.opts.grpcDialOpts, grpc.WithDefaultCallOptions(
		grpc.CallContentSubtype(ContentSubtype),
	))
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

func (m *RawManager[idType]) closeNodeConns() {
	for _, node := range m.nodes {
		err := node.close()
		if err != nil && m.logger != nil {
			m.logger.Printf("error closing: %v", err)
		}
	}
}

// Close closes all node connections and any client streams.
func (m *RawManager[idType]) Close() {
	m.closeOnce.Do(func() {
		if m.logger != nil {
			m.logger.Printf("closing")
		}
		m.closeNodeConns()
	})
}

// NodeIDs returns the identifier of each available node. IDs are returned in
// the same order as they were provided in the creation of the Manager.
func (m *RawManager[idType]) NodeIDs() []idType {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]idType, 0, len(m.nodes))
	for _, node := range m.nodes {
		ids = append(ids, node.ID())
	}
	return ids
}

// Node returns the node with the given identifier if present.
func (m *RawManager[idType]) Node(id idType) (node *RawNode[idType], found bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	node, found = m.lookup[id]
	return node, found
}

// Nodes returns a slice of each available node. IDs are returned in the same
// order as they were provided in the creation of the Manager.
func (m *RawManager[idType]) Nodes() []*RawNode[idType] {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.nodes
}

// Size returns the number of nodes in the Manager.
func (m *RawManager[idType]) Size() (nodes int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.nodes)
}

// AddNode adds the node to the manager's node pool
// and establishes a connection to the node.
func (m *RawManager[idType]) AddNode(node *RawNode[idType]) error {
	if _, found := m.Node(node.ID()); found {
		// Node IDs must be unique
		return fmt.Errorf("config: node %d (%s) already exists", node.ID(), node.Address())
	}
	if m.logger != nil {
		m.logger.Printf("Connecting to %s with id %d\n", node, node.id)
	}
	if err := node.connect(m); err != nil {
		if m.logger != nil {
			m.logger.Printf("Failed to connect to %s: %v (retrying)", node, err)
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.lookup[node.id] = node
	m.nodes = append(m.nodes, node)
	return nil
}

// getMsgID returns a unique message ID.
func (m *RawManager[idType]) getMsgID() uint64 {
	return atomic.AddUint64(&m.nextMsgID, 1)
}
