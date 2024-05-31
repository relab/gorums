package gorums

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/relab/gorums/broadcast"
	"github.com/relab/gorums/logging"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
)

// RawManager maintains a connection pool of nodes on
// which quorum calls can be performed.
//
// This struct is intended to be used by generated code.
// You should use the generated `Manager` struct instead.
type RawManager struct {
	mu        sync.Mutex
	nodes     []*RawNode
	lookup    map[uint32]*RawNode
	closeOnce sync.Once
	logger    *slog.Logger
	opts      managerOptions
	nextMsgID uint64
	snowflake Snowflake
}

// NewRawManager returns a new RawManager for managing connection to nodes added
// to the manager. This function accepts manager options used to configure
// various aspects of the manager. This function is meant for internal use.
// You should use the `NewManager` function in the generated code instead.
func NewRawManager(opts ...ManagerOption) *RawManager {
	m := &RawManager{
		lookup: make(map[uint32]*RawNode),
		opts:   newManagerOptions(),
	}
	for _, opt := range opts {
		opt(&m.opts)
	}
	if m.opts.logger != nil {
		m.logger = m.opts.logger.With(slog.Uint64("MachineID", m.opts.machineID))
	}
	m.opts.grpcDialOpts = append(m.opts.grpcDialOpts, grpc.WithDefaultCallOptions(
		grpc.CallContentSubtype(ContentSubtype),
	))
	if m.opts.backoff != backoff.DefaultConfig {
		m.opts.grpcDialOpts = append(m.opts.grpcDialOpts, grpc.WithConnectParams(
			grpc.ConnectParams{Backoff: m.opts.backoff},
		))
	}
	m.log("manager: ready", nil)
	m.snowflake = broadcast.NewSnowflake(m.opts.machineID)
	return m
}

func (m *RawManager) closeNodeConns() {
	for _, node := range m.nodes {
		err := node.close()
		if err != nil {
			m.log("manager: error closing node", err, logging.NodeID(node.ID()))
		}
	}
}

// Close closes all node connections and any client streams.
func (m *RawManager) Close() {
	m.closeOnce.Do(func() {
		m.log("manager: closing", nil)
		m.closeNodeConns()
	})
}

// NodeIDs returns the identifier of each available node. IDs are returned in
// the same order as they were provided in the creation of the Manager.
func (m *RawManager) NodeIDs() []uint32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]uint32, 0, len(m.nodes))
	for _, node := range m.nodes {
		ids = append(ids, node.ID())
	}
	return ids
}

// Node returns the node with the given identifier if present.
func (m *RawManager) Node(id uint32) (node *RawNode, found bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	node, found = m.lookup[id]
	return node, found
}

// Nodes returns a slice of each available node. IDs are returned in the same
// order as they were provided in the creation of the Manager.
func (m *RawManager) Nodes() []*RawNode {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.nodes
}

// Size returns the number of nodes in the Manager.
func (m *RawManager) Size() (nodes int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.nodes)
}

// AddNode adds the node to the manager's node pool
// and establishes a connection to the node.
func (m *RawManager) AddNode(node *RawNode) error {
	if _, found := m.Node(node.ID()); found {
		// Node IDs must be unique
		return fmt.Errorf("config: node %d (%s) already exists", node.ID(), node.Address())
	}
	m.log("manager: connecting to node", nil, logging.NodeID(node.ID()), logging.NodeAddr(node.Address()))
	if err := node.connect(m); err != nil {
		m.log("manager: failed to connect to node (retrying later)", err, logging.NodeID(node.ID()), logging.NodeAddr(node.Address()))
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lookup[node.id] = node
	m.nodes = append(m.nodes, node)
	return nil
}

func (m *RawManager) Snowflake() Snowflake {
	return m.snowflake
}

// getMsgID returns a unique message ID.
func (m *RawManager) getMsgID() uint64 {
	return atomic.AddUint64(&m.nextMsgID, 1)
}

func (m *RawManager) log(msg string, err error, args ...slog.Attr) {
	if m.logger != nil {
		args = append(args, logging.Err(err), logging.Type("manager"))
		level := slog.LevelError
		if err != nil {
			level = slog.LevelError
		}
		m.logger.LogAttrs(context.Background(), level, msg, args...)
	}
}
