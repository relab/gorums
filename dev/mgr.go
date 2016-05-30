package dev

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"net"
	"time"
)

// NewManager attempts to connect to the given set of node addresses and if
// successful returns a new Manager containing connections to those nodes.
func NewManager(nodeAddrs []string, opts ...ManagerOption) (*Manager, error) {
	if len(nodeAddrs) == 0 {
		return nil, fmt.Errorf("could not create manager: no nodes provided")
	}

	m := new(Manager)
	m.nodeGidToID = make(map[uint32]int)
	m.configGidToID = make(map[uint32]int)

	for _, opt := range opts {
		opt(&m.opts)
	}

	selfAddrIndex, selfGid, err := m.parseSelfOptions(nodeAddrs)
	if err != nil {
		return nil, ManagerCreationError(err)
	}

	gidSeen := false
	for i, naddr := range nodeAddrs {
		node, err2 := m.createNode(naddr)
		if err2 != nil {
			return nil, ManagerCreationError(err)
		}
		m.nodes = append(m.nodes, node)
		if i == selfAddrIndex {
			node.self = true
			continue
		}
		if node.gid == selfGid {
			node.self = true
			gidSeen = true
		}
	}
	if selfGid != 0 && !gidSeen {
		return nil, ManagerCreationError(
			fmt.Errorf("WithSelfGid provided, but no node with gid %d found", selfGid),
		)
	}

	OrderedBy(GlobalID).Sort(m.nodes)

	for i, node := range m.nodes {
		node.id = i
		m.nodeGidToID[node.gid] = node.id
	}

	err = m.connectAll()
	if err != nil {
		return nil, ManagerCreationError(err)
	}

	err = m.createStreamClients()
	if err != nil {
		return nil, ManagerCreationError(err)
	}

	if m.opts.logger != nil {
		m.logger = m.opts.logger
	}

	m.setDefaultQuorumFuncs()

	return m, nil
}

func (m *Manager) parseSelfOptions(addrs []string) (int, uint32, error) {
	if m.opts.selfAddr != "" && m.opts.selfGid != 0 {
		return 0, 0, fmt.Errorf("both WithSelfAddr and WithSelfGid provided")
	}
	if m.opts.selfGid != 0 {
		return -1, m.opts.selfGid, nil
	}
	if m.opts.selfAddr == "" {
		return -1, 0, nil
	}

	seen, index := contains(m.opts.selfAddr, addrs)
	if !seen {
		return 0, 0, fmt.Errorf(
			"option WithSelfAddr provided, but address %q was not present in address list",
			m.opts.selfAddr)
	}

	return index, 0, nil
}

func (m *Manager) createNode(addr string) (*Node, error) {
	m.Lock()
	defer m.Unlock()

	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("create node %s error: %v", addr, err)
	}

	h := fnv.New32a()
	_, _ = h.Write([]byte(tcpAddr.String()))
	gid := h.Sum32()

	if _, nodeExists := m.nodeGidToID[gid]; nodeExists {
		return nil, fmt.Errorf("create node %s error: node already exists", addr)
	}

	node := &Node{
		gid:     gid,
		addr:    tcpAddr.String(),
		latency: -1 * time.Second,
	}

	m.nodeGidToID[gid] = -1

	return node, nil
}

func (m *Manager) connectAll() error {
	if m.opts.noConnect {
		return nil
	}
	for _, node := range m.nodes {
		if node.self {
			continue
		}
		err := node.connect(m.opts.grpcDialOpts...)
		if err != nil {
			return fmt.Errorf("connect node %s error: %v", node.addr, err)
		}
	}
	return nil
}

func (m *Manager) closeNodeConns() {
	for _, node := range m.nodes {
		if node.self {
			continue
		}
		err := node.conn.Close()
		if err == nil {
			continue
		}
		if m.logger != nil {
			m.logger.Printf("node %d: error closing connection: %v", node.id, err)
		}
	}
}

// Close closes all node connections and any client streams.
func (m *Manager) Close() {
	m.closeOnce.Do(func() {
		m.closeStreamClients()
		m.closeNodeConns()
	})
}

// NodeIDs returns the identifier of each available node.
func (m *Manager) NodeIDs() []int {
	m.RLock()
	defer m.RUnlock()
	ids := make([]int, len(m.nodes))
	for i := range m.nodes {
		ids[i] = i
	}
	return ids
}

// NodeGlobalIDs returns the global identifier of each available node.
func (m *Manager) NodeGlobalIDs() []uint32 {
	m.RLock()
	defer m.RUnlock()
	gids := make([]uint32, len(m.nodeGidToID))
	for gid, id := range m.nodeGidToID {
		gids[id] = gid
	}
	return gids
}

// Node returns the node with the given local identifier if present.
func (m *Manager) Node(id int) (node *Node, found bool) {
	m.RLock()
	defer m.RUnlock()
	if id < 0 || id >= len(m.nodes) {
		return nil, false
	}
	node = m.nodes[id]
	if node == nil {
		return nil, false
	}
	return node, true
}

// NodeFromGlobalID returns the node with the given global identifier if
// present.
func (m *Manager) NodeFromGlobalID(gid uint32) (node *Node, found bool) {
	m.RLock()
	defer m.RUnlock()
	localID, found := m.nodeGidToID[gid]
	if !found {
		return nil, false
	}
	return m.Node(localID)
}

// Nodes returns a slice of each available node.
func (m *Manager) Nodes(excludeSelf bool) []*Node {
	m.RLock()
	defer m.RUnlock()
	var nodes []*Node
	for _, node := range m.nodes {
		if excludeSelf && node.self {
			continue
		}
		nodes = append(nodes, node)
	}
	return nodes
}

// ConfigurationIDs returns the identifier of each available configuration.
func (m *Manager) ConfigurationIDs() []int {
	m.RLock()
	defer m.RUnlock()
	ids := make([]int, len(m.configs))
	for i := range m.configs {
		ids[i] = i
	}
	return ids
}

// ConfigurationGlobalIDs returns the global identifier of each available
// configuration.
func (m *Manager) ConfigurationGlobalIDs() []uint32 {
	m.RLock()
	defer m.RUnlock()
	gids := make([]uint32, len(m.configGidToID))
	for gid, id := range m.configGidToID {
		gids[id] = gid
	}
	return gids
}

// Configuration returns the configuration with the given identifier if
// present.
func (m *Manager) Configuration(id int) (config *Configuration, found bool) {
	m.RLock()
	defer m.RUnlock()
	if id < 0 || id >= len(m.configs) {
		return nil, false
	}
	config = m.configs[id]
	if config == nil {
		return nil, false
	}
	return config, true
}

// ConfigurationFromGlobalID returns the configuration with the given global
// identifier if present.
func (m *Manager) ConfigurationFromGlobalID(gid uint32) (config *Configuration, found bool) {
	m.RLock()
	defer m.RUnlock()
	localID, found := m.configGidToID[gid]
	if !found {
		return nil, false
	}
	return m.Configuration(localID)
}

// Configurations returns a slice of each available configuration.
func (m *Manager) Configurations() []*Configuration {
	m.RLock()
	defer m.RUnlock()
	cos := make([]*Configuration, len(m.configs))
	for i := range m.configs {
		cos[i] = m.configs[i]
	}
	return cos
}

// Size returns the number of nodes and configurations in the Manager.
func (m *Manager) Size() (nodes, configs int) {
	m.RLock()
	defer m.RUnlock()
	return len(m.nodes), len(m.configs)
}

// AddNode attempts to dial to the provide node address. The node is
// added to the Manager's pool of nodes if a connection was established.
func (m *Manager) AddNode(addr string) error {
	panic("not implemented")
}

// NewConfiguration returns a new configuration given a set of node ids and
// a quorum size. Any given gRPC call options will be used for every RPC
// invocation on the configuration.
func (m *Manager) NewConfiguration(ids []int, quorumSize int, timeout time.Duration) (*Configuration, error) {
	m.Lock()
	defer m.Unlock()

	if len(ids) == 0 {
		return nil, IllegalConfigError("need at least one node")
	}
	if quorumSize > len(ids) || quorumSize < 1 {
		return nil, IllegalConfigError("invalid quourm size")
	}
	if timeout <= 0 {
		return nil, IllegalConfigError("timeout must be positive")
	}

	var cnodes []*Node
	for _, nid := range ids {
		if nid < 0 || nid >= len(m.nodes) {
			return nil, NodeNotFoundError(nid)
		}
		node := m.nodes[nid]
		if node == nil {
			return nil, NodeNotFoundError(nid)
		}
		if node.self && m.selfSpecified() {
			return nil, IllegalConfigError(
				fmt.Sprintf("self (%d) can't be part of a configuration when a self-option is provided", nid),
			)
		}
		cnodes = append(cnodes, node)
	}

	// Node ids are sorted by global id to
	// ensure a globally consistent configuration id.
	OrderedBy(GlobalID).Sort(cnodes)

	h := fnv.New32a()
	binary.Write(h, binary.LittleEndian, int64(quorumSize))
	binary.Write(h, binary.LittleEndian, timeout)
	for _, node := range cnodes {
		binary.Write(h, binary.LittleEndian, node.gid)
	}
	gcid := h.Sum32()

	cid, found := m.configGidToID[gcid]
	if found {
		if m.configs[cid] == nil {
			panic(fmt.Sprintf("config with gcid %d and cid %d was nil", gcid, cid))
		}
		return m.configs[cid], nil
	}
	cid = len(m.configs)

	c := &Configuration{
		id:      cid,
		gid:     gcid,
		nodes:   ids,
		mgr:     m,
		quorum:  quorumSize,
		timeout: timeout,
	}
	m.configs = append(m.configs, c)

	return c, nil
}

func (m *Manager) selfSpecified() bool {
	return m.opts.selfAddr != "" || m.opts.selfGid != 0
}
