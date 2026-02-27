package gorums

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"

	"github.com/relab/gorums/internal/stream"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/metadata"
)

// Manager maintains a connection pool of nodes on
// which quorum calls can be performed.
type Manager struct {
	mu         sync.Mutex
	nodes      []*Node
	lookup     map[uint32]*Node
	closeOnce  sync.Once
	logger     *log.Logger
	opts       managerOptions
	nextMsgID  uint64
	inboundMgr *InboundManager // non-nil when stream dedup is active
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
	m.inboundMgr = m.opts.inboundMgr
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
	// Stream dedup: if an InboundManager is present, try to adopt the shared
	// peer node instead of creating a new outbound connection.
	if n, adopted := m.adoptPeerNode(id, addr); adopted {
		return n, nil
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
// When stream dedup is active (inboundMgr != nil), it delegates to the
// InboundManager's counter to avoid message ID collisions in shared routers.
func (m *Manager) getMsgID() uint64 {
	if m.inboundMgr != nil {
		return m.inboundMgr.getMsgID()
	}
	return atomic.AddUint64(&m.nextMsgID, 1)
}

// adoptPeerNode looks up a shared peer node from the InboundManager and adopts
// it into this Manager. It applies the stream dedup ownership rule:
//   - myID < peerID (lower-ID, owner): create outbound channel on the shared node.
//   - myID > peerID (higher-ID, receiver): adopt the shared node as-is (no channel);
//     the channel arrives when the lower-ID peer connects via registerPeer.
//   - myID == peerID (self): do not adopt; fall through to normal outbound creation.
//
// Returns (node, true) if the peer was adopted, or (nil, false) to fall through.
func (m *Manager) adoptPeerNode(id uint32, addr string) (*Node, bool) {
	if m.inboundMgr == nil {
		return nil, false
	}
	myID := m.inboundMgr.myID
	if id == myID {
		// Self-node: fall through to normal outbound creation.
		return nil, false
	}
	peer := m.inboundMgr.PeerNode(id)
	if peer == nil {
		// Not a known peer in InboundManager; fall through.
		return nil, false
	}
	// Set backward-compat Manager pointer and use unified message ID generator.
	peer.mgr = m
	peer.msgIDGen = m.getMsgID

	if myID < id {
		// Lower-ID node owns the outbound stream: create gRPC connection
		// and outbound channel on the shared node.
		conn, err := grpc.NewClient(addr, m.opts.grpcDialOpts...)
		if err != nil {
			// Fall through to normal creation on dial error.
			return nil, false
		}
		md := m.opts.metadata.Copy()
		if m.opts.perNodeMD != nil {
			md = metadata.Join(md, m.opts.perNodeMD(id))
		}
		ctx := metadata.NewOutgoingContext(context.Background(), md)
		ch := stream.NewOutboundChannel(ctx, id, m.opts.sendBuffer, conn, peer.router)
		peer.channel.Store(ch)
	}
	// Higher-ID node: peer.channel remains nil until the lower-ID peer connects
	// and InboundManager.registerPeer attaches an inbound channel.

	m.addNode(peer)
	return peer, true
}
