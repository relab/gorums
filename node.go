package gorums

import (
	"cmp"
	"context"
	"fmt"
	"net"
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
	mgr  *outboundManager // owning manager for this node

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
		panic("gorums: Context called on nil node")
	}
	return &NodeContext{Context: parent, node: n}
}

// nodeOptions contains configuration options for creating a new Node.
type nodeOptions struct {
	ID             uint32
	SendBufferSize uint
	MsgIDGen       func() uint64
	Metadata       metadata.MD
	DialOpts       []grpc.DialOption
	RequestHandler stream.RequestHandler
	Manager        *outboundManager // owning manager
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

// Latency returns the current round-trip latency estimate for this node,
// computed as an exponentially weighted moving average with a
// smoothing factor of 0.2 (roughly a 5-sample window).
//
// The returned value has several important limits:
//   - It returns -1s until the first successful response is received; treat
//     negative values as "no data" rather than a real measurement.
//   - The estimate is only updated when there is active traffic. On an idle
//     node the value may be arbitrarily stale and will not reflect recent
//     changes in network conditions.
//   - A step-change in latency takes several round trips to settle because
//     each new sample contributes only 20% of the new value.
//
// Use [Latency] as a comparator with [Configuration.SortBy] to order nodes
// by their current observed latency.
func (n *Node) Latency() time.Duration {
	return n.router.Latency()
}

// ID compares nodes by their identifier in increasing order.
// It is compatible with [slices.SortFunc] and [Configuration.SortBy].
var ID = func(a, b *Node) int {
	return cmp.Compare(a.id, b.id)
}

// LastNodeError compares nodes by their LastErr() status.
// Nodes with no error sort before nodes with an error.
// It is compatible with [slices.SortFunc] and [Configuration.SortBy].
var LastNodeError = func(a, b *Node) int {
	aErr := a.LastErr()
	bErr := b.LastErr()
	switch {
	case aErr != nil && bErr == nil:
		return 1
	case aErr == nil && bErr != nil:
		return -1
	default:
		return 0
	}
}

// Latency compares nodes by their current latency estimate in ascending order.
// Nodes with no measurement yet (negative latency value) sort after nodes with a
// measurement. It is compatible with [slices.SortFunc] and [Configuration.SortBy].
var Latency = func(a, b *Node) int {
	la, lb := a.Latency(), b.Latency()
	switch {
	case la < 0 && lb < 0:
		return 0
	case la < 0:
		return 1
	case lb < 0:
		return -1
	}
	return cmp.Compare(la, lb)
}

// compile-time assertion for interface compliance.
var _ stream.PeerNode = (*Node)(nil)
