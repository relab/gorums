package gorums

import (
	"fmt"
	"log"
	"strings"
	"sync"

	"golang.org/x/net/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
)

// Manager manages a pool of node configurations on which quorum remote
// procedure calls can be made.
type Manager struct {
	mu       sync.Mutex
	nodes    []*Node
	lookup   map[uint32]*Node
	eventLog trace.EventLog

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

	var nodeAddrs []string
	if m.opts.idMapping != nil {
		for naddr, id := range m.opts.idMapping {
			err := m.AddNode(naddr, id)
			if err != nil {
				return nil, ManagerCreationError(err)
			}
			nodeAddrs = append(nodeAddrs, naddr)
		}
		// Sort nodes since map iteration is non-deterministic.
		OrderedBy(ID).Sort(m.nodes)
	} else if m.opts.addrsList != nil {
		nodeAddrs = m.opts.addrsList
		for _, naddr := range m.opts.addrsList {
			err := m.AddNode(naddr, 0)
			if err != nil {
				return nil, ManagerCreationError(err)
			}
		}
	}

	if m.opts.trace {
		title := strings.Join(nodeAddrs, ",")
		m.eventLog = trace.NewEventLog("gorums.Manager", title)
	}
	if err := m.connectAll(); err != nil {
		return nil, ManagerCreationError(err)
	}
	if m.opts.logger != nil {
		m.logger = m.opts.logger
	}
	if m.eventLog != nil {
		m.eventLog.Printf("ready")
	}

	return m, nil
}

func (m *Manager) connectAll() error {
	if m.opts.noConnect {
		return nil
	}

	if m.eventLog != nil {
		m.eventLog.Printf("connecting")
	}

	for _, node := range m.nodes {
		err := node.connect(m.receiveQueue, m.opts)
		if err != nil {
			if m.eventLog != nil {
				m.eventLog.Errorf("connect failed, error connecting to node %s, error: %v", node.addr, err)
			}
			return fmt.Errorf("connect node %s error: %v", node.addr, err)
		}
	}
	return nil
}

func (m *Manager) closeNodeConns() {
	for _, node := range m.nodes {
		err := node.close()
		if err == nil {
			continue
		}
		if m.logger != nil {
			m.logger.Printf("node %d: error closing: %v", node.id, err)
		}
	}
}

// Close closes all node connections and any client streams.
func (m *Manager) Close() {
	m.closeOnce.Do(func() {
		if m.eventLog != nil {
			m.eventLog.Printf("closing")
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

// AddNode adds the node to the manager's node pool.
// No connections are established here.
func (m *Manager) AddNode(addr string, id uint32) error {
	if _, found := m.Node(id); found {
		// Node IDs must be unique
		return fmt.Errorf("node ID %d already exists (%s)", id, addr)
	}
	node, err := NewNode(addr, id)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.lookup[node.id] = node
	m.nodes = append(m.nodes, node)
	return nil
}
