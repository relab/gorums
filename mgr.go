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

// Manager maintains a connection pool of nodes on
// which quorum calls can be performed.
type Manager struct {
	mu        sync.Mutex
	nodes     []*Node
	lookup    map[uint32]*Node
	closeOnce sync.Once
	logger    *log.Logger
	opts      managerOptions
	nextMsgID uint64
}

// NewManager returns a new Manager for managing connection to nodes added
// to the manager. This function accepts manager options used to configure
// various aspects of the manager.
func NewManager(opts ...ManagerOption) *Manager {
	m := &Manager{
		lookup: make(map[uint32]*Node),
		opts:   newManagerOptions(),
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

// Close closes all node connections and any client streams.
func (m *Manager) Close() error {
	var err error
	m.closeOnce.Do(func() {
		for _, node := range m.nodes {
			err = errors.Join(err, node.close())
		}
	})
	return err
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

func (m *Manager) addNode(node *Node) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lookup[node.id] = node
	m.nodes = append(m.nodes, node)
}

func (m *Manager) newNode(addr string, id uint32) (*Node, error) {
	if _, found := m.Node(id); found {
		return nil, fmt.Errorf("node %d already exists", id)
	}
	opts := nodeOptions{
		ID:             id,
		SendBufferSize: m.opts.sendBuffer,
		MsgIDGen:       m.getMsgID,
		Metadata:       m.opts.metadata,
		PerNodeMD:      m.opts.perNodeMD,
		DialOpts:       m.opts.grpcDialOpts,
		Manager:        m,
	}
	n, err := newNode(addr, opts)
	if err != nil {
		return nil, err
	}
	m.addNode(n)
	return n, nil
}

// getMsgID returns a unique message ID for a new RPC from this client's manager.
func (m *Manager) getMsgID() uint64 {
	return atomic.AddUint64(&m.nextMsgID, 1)
}
