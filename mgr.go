package gorums

import (
	"fmt"
	"log"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
)

// Manager manages a pool of node configurations on which quorum remote
// procedure calls can be made.
type Manager struct {
	mu        sync.Mutex
	nodes     []*Node
	lookup    map[uint32]*Node
	closeOnce sync.Once
	logger    *log.Logger
	opts      managerOptions

	*receiveQueue
}

// NewManager attempts to connect to the given set of node addresses and if
// successful returns a new Manager containing connections to those nodes.
// This function is meant for internal Gorums use. You should use the `NewManager`
// function in the generated code instead.
func NewManager(opts ...ManagerOption) (*Manager, error) {
	m := &Manager{
		lookup:       make(map[uint32]*Node),
		receiveQueue: newReceiveQueue(),
		opts:         newManagerOptions(),
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
	if len(m.opts.addrsList) == 0 && len(m.opts.idMapping) == 0 {
		return nil, fmt.Errorf("could not create manager: no nodes provided")
	}
	if m.opts.backoff != backoff.DefaultConfig {
		m.opts.grpcDialOpts = append(m.opts.grpcDialOpts, grpc.WithConnectParams(
			grpc.ConnectParams{Backoff: m.opts.backoff},
		))
	}

	if m.opts.idMapping != nil {
		for naddr, id := range m.opts.idMapping {
			err := m.AddNode(naddr, id)
			if err != nil {
				return nil, ManagerCreationError(err)
			}
		}
		// Sort nodes since map iteration is non-deterministic.
		OrderedBy(ID).Sort(m.nodes)
	} else if m.opts.addrsList != nil {
		for _, naddr := range m.opts.addrsList {
			err := m.AddNode(naddr, 0)
			if err != nil {
				return nil, ManagerCreationError(err)
			}
		}
	}

	if m.logger != nil {
		m.logger.Printf("ready")
	}
	return m, nil
}

func (m *Manager) closeNodeConns() {
	for _, node := range m.nodes {
		err := node.close()
		if err != nil && m.logger != nil {
			m.logger.Printf("node %d: error closing: %v", node.id, err)
		}
	}
}

// Close closes all node connections and any client streams.
func (m *Manager) Close() {
	m.closeOnce.Do(func() {
		if m.logger != nil {
			m.logger.Printf("closing")
		}
		m.closeNodeConns()
	})
}

// NodeIDs returns the identifier of each available node. IDs are returned in
// the same order as they were provided in the creation of the Manager.
func (m *Manager) NodeIDs() []uint32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]uint32, 0, len(m.nodes))
	for _, node := range m.nodes {
		ids = append(ids, node.ID())
	}
	return ids
}

// Node returns the node with the given identifier if present.
func (m *Manager) Node(id uint32) (node *Node, found bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	node, found = m.lookup[id]
	return node, found
}

// Nodes returns a slice of each available node. IDs are returned in the same
// order as they were provided in the creation of the Manager.
func (m *Manager) Nodes() []*Node {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.nodes
}

// Size returns the number of nodes in the Manager.
func (m *Manager) Size() (nodes int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.nodes)
}

// AddNode adds the node to the manager's node pool
// and establishes a connection to the node.
func (m *Manager) AddNode(addr string, id uint32) error {
	if _, found := m.Node(id); found {
		// Node IDs must be unique
		return fmt.Errorf("node ID %d already exists (%s)", id, addr)
	}
	node, err := NewNode(addr, id)
	if err != nil {
		return err
	}
	if m.logger != nil {
		m.logger.Printf("connecting to %s", node)
	}
	if err = node.connect(m.receiveQueue, m.opts); err != nil {
		return fmt.Errorf("connection failed for %s: %w", node, err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.lookup[node.id] = node
	m.nodes = append(m.nodes, node)
	return nil
}
