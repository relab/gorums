package gorums

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strconv"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/relab/gorums/internal/stream"
)

const nilAngleString = "<nil>"

// NodeContext is a context that carries a node for unicast and RPC calls.
// It embeds context.Context and provides access to the Node.
//
// Use [Node.Context] to create a NodeContext from an existing context.
type NodeContext struct {
	context.Context
	node *Node
}

// Node returns the Node associated with this context.
func (c NodeContext) Node() *Node {
	return c.node
}

// enqueue enqueues a request to this node's channel. If the channel is nil,
// e.g., for the self-node, the request is silently dropped.
func (c NodeContext) enqueue(req stream.Request) {
	c.node.Enqueue(req)
}

// nextMsgID returns the next message ID from this client's manager.
func (c NodeContext) nextMsgID() uint64 {
	return c.node.msgIDGen()
}

// Node encapsulates the state of a node on which a remote procedure call
// can be performed.
type Node struct {
	// Only assigned at creation.
	id   uint32
	addr string
	mgr  *outboundManager // only used for backward compatibility to allow Configuration.Manager()

	msgIDGen func() uint64
	router   *stream.MessageRouter
	channel  atomic.Pointer[stream.Channel]
}

// Context creates a new NodeContext from the given parent context
// and this node.
//
// Example:
//
//	nodeCtx := node.Context(context.Background())
//	resp, err := service.GRPCCall(nodeCtx, req)
func (n *Node) Context(parent context.Context) *NodeContext {
	if n == nil {
		panic("gorums: Context called with nil node")
	}
	return &NodeContext{Context: parent, node: n}
}

// nodeOptions contains configuration options for creating a new Node.
type nodeOptions struct {
	ID             uint32
	SendBufferSize uint
	MsgIDGen       func() uint64
	Metadata       metadata.MD
	PerNodeMD      func(uint32) metadata.MD
	DialOpts       []grpc.DialOption
	RequestHandler stream.RequestHandler
	Manager        *outboundManager // only used for backward compatibility to allow Configuration.Manager()
}

// newOutboundNode creates a new node using the provided options. It establishes
// the connection (lazy dial) and initializes the outbound channel.
func newOutboundNode(addr string, opts nodeOptions) (*Node, error) {
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return nil, err
	}

	n := &Node{
		id:       opts.ID,
		addr:     tcpAddr.String(),
		mgr:      opts.Manager,
		msgIDGen: opts.MsgIDGen,
		router:   stream.NewMessageRouter(opts.RequestHandler),
	}

	// Create gRPC connection to the node without connecting (lazy dial).
	conn, err := grpc.NewClient(n.addr, opts.DialOpts...)
	if err != nil {
		return nil, nodeError{nodeID: n.id, cause: err}
	}

	// Create outgoing context with metadata for this node's stream.
	md := opts.Metadata.Copy()
	if opts.PerNodeMD != nil {
		md = metadata.Join(md, opts.PerNodeMD(n.id))
	}
	ctx := metadata.NewOutgoingContext(context.Background(), md)

	// Create new outbound channel and establish gRPC node stream
	n.channel.Store(stream.NewOutboundChannel(ctx, n.id, opts.SendBufferSize, conn, n.router))
	return n, nil
}

// newInboundNode creates a Node for a known peer or self without an active
// channel. Used by inboundManager at construction time for all configured
// peers; the channel is attached when the peer's stream arrives.
// The handler, if non-nil, is stored in the router and used to dispatch
// client-initiated requests received on the inbound stream.
func newInboundNode(id uint32, addr string, msgIDGen func() uint64, handler stream.RequestHandler) *Node {
	return &Node{
		id:       id,
		addr:     addr,
		msgIDGen: msgIDGen,
		router:   stream.NewMessageRouter(handler),
	}
}

// newLocalNode creates a Node that dispatches calls in-process, bypassing the
// network. It is used for the self-node when this process is both client and
// server in a symmetric peer configuration. The provided handler (typically
// the local *Server) serves requests directly without a gRPC round-trip.
func newLocalNode(id uint32, addr string, msgIDGen func() uint64, handler stream.RequestHandler, mgr *outboundManager) *Node {
	router := stream.NewMessageRouter(handler)
	n := &Node{
		id:       id,
		addr:     addr,
		mgr:      mgr,
		msgIDGen: msgIDGen,
		router:   router,
	}
	n.channel.Store(stream.NewLocalChannel(id, router))
	return n
}

// IsInbound returns true if the node has an active inbound channel.
func (n *Node) IsInbound() bool {
	if n == nil {
		return false
	}
	ch := n.channel.Load()
	return ch != nil && ch.IsInbound()
}

// attachStream attaches a new inbound channel to the node when a peer connects.
// If the node already has an active channel (e.g., a stale stream from a previous
// connection), it is atomically replaced and the old channel is closed.
// The returned detach function closes and removes the channel, but only if it
// is still the active one. This prevents stale cleanup from an older stream
// from detaching a replacement channel that has already taken over.
func (n *Node) attachStream(streamCtx context.Context, inboundStream stream.BidiStream, sendBufferSize uint) (detach func() bool) {
	newCh := stream.NewInboundChannel(streamCtx, n.id, sendBufferSize, inboundStream, n.router)
	if old := n.channel.Swap(newCh); old != nil {
		old.Close()
	}
	return func() bool {
		if n.channel.CompareAndSwap(newCh, nil) {
			newCh.Close()
			return true
		}
		return false
	}
}

// RouteInbound delivers a response to a pending call or dispatches a
// client-initiated request to the registered handler. The release
// function is always called.
// This implements the [stream.PeerNode] interface.
func (n *Node) RouteInbound(ctx context.Context, msg *stream.Message, release func(), send func(*stream.Message)) {
	n.router.RouteInboundMessage(ctx, n.id, msg, release, send)
}

// Enqueue enqueues a request to this node's channel.
// For local channels the channel handles in-process dispatch directly.
// If no channel is available, the request is silently dropped.
// This implements the [stream.PeerNode] interface.
func (n *Node) Enqueue(req stream.Request) {
	if ch := n.channel.Load(); ch != nil {
		ch.Enqueue(req)
	}
}

// close this node.
func (n *Node) close() error {
	if ch := n.channel.Load(); ch != nil {
		return ch.Close()
	}
	return nil
}

// ID returns the ID of n.
func (n *Node) ID() uint32 {
	if n != nil {
		return n.id
	}
	return 0
}

// Address returns network address of n.
func (n *Node) Address() string {
	if n != nil {
		return n.addr
	}
	return nilAngleString
}

// Host returns the network host of n.
func (n *Node) Host() string {
	if n == nil {
		return nilAngleString
	}
	host, _, _ := net.SplitHostPort(n.addr)
	return host
}

// Port returns network port of n.
func (n *Node) Port() string {
	if n != nil {
		_, port, _ := net.SplitHostPort(n.addr)
		return port
	}
	return nilAngleString
}

func (n *Node) String() string {
	if n != nil {
		return fmt.Sprintf("addr: %s", n.addr)
	}
	return nilAngleString
}

// FullString returns a more descriptive string representation of n that
// includes id, network address and latency information.
func (n *Node) FullString() string {
	if n != nil {
		return fmt.Sprintf("node %d | addr: %s", n.id, n.addr)
	}
	return nilAngleString
}

// LastErr returns the last error encountered (if any) for this node.
func (n *Node) LastErr() error {
	if ch := n.channel.Load(); ch != nil {
		return ch.LastErr()
	}
	return nil
}

// Latency returns the latency between the client and this node.
func (n *Node) Latency() time.Duration {
	return n.router.Latency()
}

type lessFunc func(n1, n2 *Node) bool

// MultiSorter implements the Sort interface, sorting the nodes within.
type MultiSorter struct {
	nodes []*Node
	less  []lessFunc
}

// Sort sorts the argument slice according to the less functions passed to
// OrderedBy.
func (ms *MultiSorter) Sort(nodes []*Node) {
	ms.nodes = nodes
	sort.Sort(ms)
}

// OrderedBy returns a Sorter that sorts using the less functions, in order.
// Call its Sort method to sort the data.
func OrderedBy(less ...lessFunc) *MultiSorter {
	return &MultiSorter{
		less: less,
	}
}

// Len is part of sort.Interface.
func (ms *MultiSorter) Len() int {
	return len(ms.nodes)
}

// Swap is part of sort.Interface.
func (ms *MultiSorter) Swap(i, j int) {
	ms.nodes[i], ms.nodes[j] = ms.nodes[j], ms.nodes[i]
}

// Less is part of sort.Interface. It is implemented by looping along the
// less functions until it finds a comparison that is either Less or not
// Less. Note that it can call the less functions twice per call. We
// could change the functions to return -1, 0, 1 and reduce the
// number of calls for greater efficiency: an exercise for the reader.
func (ms *MultiSorter) Less(i, j int) bool {
	p, q := ms.nodes[i], ms.nodes[j]
	// Try all but the last comparison.
	var k int
	for k = range len(ms.less) - 1 {
		less := ms.less[k]
		switch {
		case less(p, q):
			// p < q, so we have a decision.
			return true
		case less(q, p):
			// p > q, so we have a decision.
			return false
		}
		// p == q; try the next comparison.
	}
	// All comparisons to here said "equal", so just return whatever
	// the final comparison reports.
	return ms.less[k](p, q)
}

// ID sorts nodes by their identifier in increasing order.
var ID = func(n1, n2 *Node) bool {
	return n1.id < n2.id
}

// Port sorts nodes by their port number in increasing order.
// Warning: This function may be removed in the future.
var Port = func(n1, n2 *Node) bool {
	p1, _ := strconv.Atoi(n1.Port())
	p2, _ := strconv.Atoi(n2.Port())
	return p1 < p2
}

// LastNodeError sorts nodes by their LastErr() status in increasing order. A
// node with LastErr() != nil is larger than a node with LastErr() == nil.
var LastNodeError = func(n1, n2 *Node) bool {
	if n1.LastErr() != nil && n2.LastErr() == nil {
		return false
	}
	return true
}

// compile-time assertion for interface compliance.
var _ stream.PeerNode = (*Node)(nil)
