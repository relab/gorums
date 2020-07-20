package dev

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"log"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

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
	configs  map[uint32]*Configuration
	eventLog trace.EventLog

	closeOnce sync.Once
	logger    *log.Logger
	opts      managerOptions

	*receiveQueue
}

// NewManager attempts to connect to the given set of node addresses and if
// successful returns a new Manager containing connections to those nodes.
func NewManager(opts ...ManagerOption) (*Manager, error) {

	m := &Manager{
		lookup:       make(map[uint32]*Node),
		configs:      make(map[uint32]*Configuration),
		receiveQueue: newReceiveQueue(),
		opts:         newManagerOptions(),
	}

	for _, opt := range opts {
		opt(&m.opts)
	}

	m.opts.grpcDialOpts = append(m.opts.grpcDialOpts, grpc.WithDefaultCallOptions(
		grpc.CallContentSubtype(gorumsContentType),
		grpc.ForceCodec(newGorumsCodec()),
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
			if m.lookup[id] != nil {
				err := fmt.Errorf("Two node ids are identical(id %d). Node ids have to be unique", id)
				return nil, ManagerCreationError(err)
			}
			nodeAddrs = append(nodeAddrs, naddr)
			node, err := m.createNode(naddr, id)
			if err != nil {
				return nil, ManagerCreationError(err)
			}
			m.lookup[node.id] = node
			m.nodes = append(m.nodes, node)
		}

		// Sort nodes since map iteration is non-deterministic.
		OrderedBy(ID).Sort(m.nodes)

	} else if m.opts.addrsList != nil {
		nodeAddrs = m.opts.addrsList
		for _, naddr := range m.opts.addrsList {
			node, err := m.createNode(naddr, 0)
			if err != nil {
				return nil, ManagerCreationError(err)
			}
			m.lookup[node.id] = node
			m.nodes = append(m.nodes, node)
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

func (m *Manager) createNode(addr string, id uint32) (*Node, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("create node %s error: %v", addr, err)
	}

	if id == 0 {
		h := fnv.New32a()
		_, _ = h.Write([]byte(tcpAddr.String()))
		id = h.Sum32()
	}

	if _, found := m.lookup[id]; found {
		return nil, fmt.Errorf("create node %s error: node already exists", addr)
	}

	node := &Node{
		id:      id,
		addr:    tcpAddr.String(),
		latency: -1 * time.Second,
	}
	node.createOrderedStream(m.receiveQueue, m.opts)

	return node, nil
}

func (m *Manager) connectAll() error {
	if m.opts.noConnect {
		return nil
	}

	if m.eventLog != nil {
		m.eventLog.Printf("connecting")
	}

	for _, node := range m.nodes {
		err := node.connect(m.opts)
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

// ConfigurationIDs returns the identifier of each available
// configuration.
func (m *Manager) ConfigurationIDs() []uint32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]uint32, 0, len(m.configs))
	for id := range m.configs {
		ids = append(ids, id)
	}
	return ids
}

// Configuration returns the configuration with the given global
// identifier if present.
func (m *Manager) Configuration(id uint32) (config *Configuration, found bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	config, found = m.configs[id]
	return config, found
}

// Configurations returns a slice of each available configuration.
func (m *Manager) Configurations() []*Configuration {
	m.mu.Lock()
	defer m.mu.Unlock()
	configs := make([]*Configuration, 0, len(m.configs))
	for _, conf := range m.configs {
		configs = append(configs, conf)
	}
	return configs
}

// Size returns the number of nodes and configurations in the Manager.
func (m *Manager) Size() (nodes, configs int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.nodes), len(m.configs)
}

// AddNode attempts to dial to the provide node address. The node is
// added to the Manager's pool of nodes if a connection was established.
func (m *Manager) AddNode(addr string) error {
	panic("not implemented")
}

// NewConfiguration returns a new configuration given quorum specification and
// a timeout.
func (m *Manager) NewConfiguration(ids []uint32, qspec QuorumSpec) (*Configuration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(ids) == 0 {
		return nil, IllegalConfigError("need at least one node")
	}

	var nodes []*Node
	unique := make(map[uint32]struct{})
	var uniqueIDs []uint32
	for _, nid := range ids {
		// ensure that identical IDs are only counted once
		if _, duplicate := unique[nid]; duplicate {
			continue
		}
		unique[nid] = struct{}{}
		uniqueIDs = append(uniqueIDs, nid)

		node, found := m.lookup[nid]
		if !found {
			return nil, NodeNotFoundError(nid)
		}
		nodes = append(nodes, node)
	}

	// node IDs are sorted to ensure a globally consistent configuration ID
	sort.Sort(idSlice(uniqueIDs))

	h := fnv.New32a()
	for _, id := range uniqueIDs {
		_ = binary.Write(h, binary.LittleEndian, id)
	}
	cid := h.Sum32()

	conf, found := m.configs[cid]
	if found {
		return conf, nil
	}

	c := &Configuration{
		id:    cid,
		nodes: nodes,
		n:     len(nodes),
		mgr:   m,
		qspec: qspec,
	}
	m.configs[cid] = c

	return c, nil
}

type idSlice []uint32

func (p idSlice) Len() int           { return len(p) }
func (p idSlice) Less(i, j int) bool { return p[i] < p[j] }
func (p idSlice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
