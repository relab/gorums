package conn

import (
	"cmp"
	"context"
	"fmt"
	"net"
	"sync"
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

// Node encapsulates the state of a node on which a remote procedure call
// can be performed.
type Node struct {
	// Only assigned at creation.
	id   uint32
	addr string
	mgr  *outboundManager // owning manager for this node

	// transport is fixed at construction, like id and addr; a node's channel
	// changes only behind the transport's channel reference.
	transport *stream.Transport

	// inboundMu guards liveChannels and the active-channel handoff in
	// attachStream. A peer may briefly have more than one live inbound stream
	// during connection churn; liveChannels is the set of them, so the node can
	// fail the active channel over to a surviving stream rather than go dark
	// when one stream ends. Lazily initialized on the first attach.
	inboundMu    sync.Mutex
	liveChannels map[*stream.Channel]struct{}
}

// newNode creates a Node with stable identity fields and its transport.
func newNode(id uint32, addr string, mgr *outboundManager, transport *stream.Transport) *Node {
	return &Node{id: id, addr: addr, mgr: mgr, transport: transport}
}

// loadTransport returns the node's transport; it is safe on a nil node and
// returns nil for a zero-value node.
func (n *Node) loadTransport() *stream.Transport {
	if n == nil {
		return nil
	}
	return n.transport
}

// NodeTransport returns the node's transport, giving the call engine access to
// the send path ([stream.Transport.NextMsgID], [stream.Transport.Enqueue])
// without exposing those operations as methods on the public [Node] type. It is
// safe on a nil node. This is the seam between the connectivity layer and the
// call engine in the runtime.
func NodeTransport(n *Node) *stream.Transport {
	return n.loadTransport()
}

// Context creates a new NodeContext from the given parent context
// and this node.
//
// Example:
//
//	nodeCtx := node.Context(context.Background())
//	resp, err := storage.ReadRPC(nodeCtx, req)
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
	EagerReconnect bool                     // re-establish a lost stream proactively; set for symmetric (WithPeers) nodes
	StreamState    func(id uint32, up bool) // optional; invoked on outbound stream transitions
	Manager        *outboundManager         // owning manager
}

// newOutboundNode creates a new node using the provided options. It establishes
// the connection (lazy dial) and initializes the outbound channel.
func newOutboundNode(addr string, opts nodeOptions) (*Node, error) {
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return nil, err
	}

	router := stream.NewMessageRouter(opts.RequestHandler)
	transport := stream.NewTransport(opts.ID, opts.MsgIDGen, router)
	n := newNode(opts.ID, tcpAddr.String(), opts.Manager, transport)

	// Create gRPC connection to the node without connecting (lazy dial).
	conn, err := grpc.NewClient(n.addr, opts.DialOpts...)
	if err != nil {
		return nil, NodeError{nodeID: n.id, cause: err}
	}

	// Create outgoing context with metadata for this node's stream.
	md := opts.Metadata.Copy()
	ctx := metadata.NewOutgoingContext(context.Background(), md)

	var onStreamChange func(up bool)
	if cb := opts.StreamState; cb != nil {
		id := opts.ID
		onStreamChange = func(up bool) { cb(id, up) }
	}
	// Create new outbound channel and establish gRPC node stream
	transport.StoreChannel(stream.NewOutboundChannel(ctx, n.id, opts.SendBufferSize, conn, router, opts.EagerReconnect, onStreamChange))
	return n, nil
}

// newInboundNode creates a Node for a known peer or self without an active
// channel. Used by inboundManager at construction time for all configured
// peers; the channel is attached when the peer's stream arrives.
// The handler, if non-nil, is stored in the router and used to dispatch
// client-initiated requests received on the inbound stream.
func newInboundNode(id uint32, addr string, msgIDGen func() uint64, handler stream.RequestHandler) *Node {
	router := stream.NewMessageRouter(handler)
	return newNode(id, addr, nil, stream.NewTransport(id, msgIDGen, router))
}

// newLocalNode creates a Node that dispatches calls in-process, bypassing the
// network. It is used for the self-node when a server calls its own peers,
// which include itself. The provided handler serves requests directly without
// a gRPC round-trip.
func newLocalNode(id uint32, addr string, msgIDGen func() uint64, handler stream.RequestHandler, mgr *outboundManager) *Node {
	router := stream.NewMessageRouter(handler)
	transport := stream.NewTransport(id, msgIDGen, router)
	n := newNode(id, addr, mgr, transport)
	transport.StoreChannel(stream.NewLocalChannel(id, router))
	return n
}

// IsInbound returns true if the node has an active inbound channel.
func (n *Node) IsInbound() bool {
	ch := n.activeChannel()
	return ch != nil && ch.IsInbound()
}

// IsOutbound returns true if the node has an active outbound client channel.
func (n *Node) IsOutbound() bool {
	ch := n.activeChannel()
	return ch != nil && ch.IsOutbound()
}

// PendingCount returns the number of pending calls currently registered in the router.
func (n *Node) PendingCount() int {
	router := n.messageRouter()
	if router == nil {
		return 0
	}
	return router.PendingCount()
}

// DroppedReplies returns the number of replies silently dropped on this
// node's active channel: back-channel or inbound replies with no response
// channel to report on, dropped because the send queue was full or the
// channel closed. Two-way requests are never counted, since their caller
// already observes the failure directly. Returns 0 if the node has no active
// channel.
func (n *Node) DroppedReplies() int64 {
	ch := n.activeChannel()
	if ch == nil {
		return 0
	}
	return ch.DroppedReplies()
}

// isUp reports whether the node's transport can currently carry calls: a
// local in-process channel, an attached inbound stream, or an established
// outbound stream. It only reads atomics, so it is safe to call while
// holding locks.
func (n *Node) isUp() bool {
	ch := n.activeChannel()
	return ch != nil && ch.StreamUp()
}

// activeChannel returns the current transport's channel, or nil if the node
// has no transport or no attached channel.
func (n *Node) activeChannel() *stream.Channel {
	return n.loadTransport().LoadChannel()
}

// messageRouter returns the current transport's router, or nil if the node
// has no transport.
func (n *Node) messageRouter() *stream.MessageRouter {
	return n.loadTransport().Router()
}

// attachStream attaches a new inbound channel to the node when a peer connects
// and returns a detach function to call when that stream ends.
//
// A peer may briefly have more than one live inbound stream: gRPC can open a
// second NodeStream over one connection during connection churn, and the server
// may register the streams in an order that does not match the client's
// creation order. attachStream tracks the set of live streams' channels and
// installs the newly attached one as the node's active channel; each channel is
// closed only when its own stream ends. When the active stream ends, the node
// fails over to another live stream's channel — any survivor satisfies the
// invariant that the active channel is non-nil while a stream lives, so no
// ordering among survivors is needed — so the active channel is nil only once
// no live stream remains. Failing over instead of clearing the slot keeps a
// surviving stream usable — otherwise it would keep receiving requests while
// its replies were dropped on a nil channel.
//
// detach is idempotent and returns true only when it removed the node's last
// live channel (the peer left the configuration), so the caller can rebuild the
// configuration; it returns false when another live stream remains.
//
// It also returns the channel created for this stream. Server replies for
// requests received on this stream must ride this channel — not the node's
// current active channel — so that during the multi-live overlap a reply is not
// queued on a different peer stream (see [peerNode]).
func (n *Node) attachStream(streamCtx context.Context, inboundStream stream.BidiStream, sendBufferSize uint) (newCh *stream.Channel, detach func() bool) {
	transport := n.loadTransport()
	newCh = stream.NewInboundChannel(streamCtx, n.id, sendBufferSize, inboundStream, transport.Router())
	n.inboundMu.Lock()
	if n.liveChannels == nil {
		n.liveChannels = make(map[*stream.Channel]struct{})
	}
	n.liveChannels[newCh] = struct{}{}
	transport.StoreChannel(newCh)
	n.inboundMu.Unlock()
	return newCh, func() bool {
		n.inboundMu.Lock()
		defer n.inboundMu.Unlock()
		if _, ok := n.liveChannels[newCh]; !ok {
			return false // already detached
		}
		delete(n.liveChannels, newCh)
		newCh.Close()
		if transport.LoadChannel() != newCh {
			return false // a different stream is active; membership unchanged
		}
		for survivor := range n.liveChannels {
			// Fail the active channel over to an arbitrary surviving stream;
			// the peer stays in the configuration.
			transport.StoreChannel(survivor)
			return false
		}
		transport.StoreChannel(nil)
		return true // no live stream remains; peer left the configuration
	}
}

// routeInbound delivers a response to a pending call or dispatches a
// client-initiated request to the registered handler. The release
// function is always called. It is exposed to the stream package through the
// [peerNode] adapter.
func (n *Node) routeInbound(ctx context.Context, msg *stream.Message, release func(), send func(*stream.Message)) {
	router := n.messageRouter()
	if router == nil {
		release()
		return
	}
	router.RouteInboundMessage(ctx, n.id, msg, release, send)
}

// trySend enqueues a request to this node's channel without ever blocking the
// caller; see [stream.Channel.TrySend]. A request for a node without a channel
// is silently dropped. It is exposed to the stream package through the
// [peerNode] adapter.
func (n *Node) trySend(req stream.Request) {
	n.loadTransport().TrySend(req)
}

// peerNode adapts a [Node] to the unexported transport interface expected by
// the stream package ([stream.PeerNode]), keeping Node's own public API free of
// the transport hooks. It wraps a *Node with no per-call allocation beyond the
// one-time construction in [InboundManager.AcceptPeer].
//
// ch is the inbound channel created for this registration's stream. Replies are
// sent on ch rather than the node's current active channel, so a request
// received on one stream has its reply ride that same stream even while another
// stream for the same peer is live and active (see [Node.attachStream]). ch is
// nil only for peerNodes built without a registered stream (some tests), where
// TrySend falls back to the node's active channel.
type peerNode struct {
	n  *Node
	ch *stream.Channel
}

// TrySend implements [stream.PeerNode] by delivering the reply on this
// registration's own stream channel, so a reply dispatched from the
// server-side inbound receive loop (see [stream.Server.NodeStream]) rides the
// stream that received the request and can never block that loop. With no bound
// channel it forwards to the node's active channel.
func (p peerNode) TrySend(req stream.Request) {
	if p.ch != nil {
		p.ch.TrySend(req)
		return
	}
	p.n.trySend(req)
}

// RouteInbound implements [stream.PeerNode] by forwarding to the node's routeInbound.
func (p peerNode) RouteInbound(ctx context.Context, msg *stream.Message, release func(), send func(*stream.Message)) {
	p.n.routeInbound(ctx, msg, release, send)
}

// close this node.
func (n *Node) close() error {
	if n == nil {
		return nil
	}
	return n.loadTransport().Close()
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

// LastErr returns the last error encountered (if any) for this node: a stream
// that could not be established, a failed send, or a broken receive.
//
// It reports current node health, not the outcome of any particular call: it is
// last-write-wins across concurrent requests and reverts to nil once traffic
// flows to the node again. Use the [ByLastError] comparator with [Config.Sort]
// to order nodes by whether they are currently failing.
func (n *Node) LastErr() error {
	if ch := n.activeChannel(); ch != nil {
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
// Use the [ByLatency] comparator with [Config.Sort] to order nodes
// by their current observed latency.
func (n *Node) Latency() time.Duration {
	router := n.messageRouter()
	if router == nil {
		return -1 * time.Second
	}
	return router.Latency()
}

// ByID compares nodes by their identifier in increasing order.
// It is compatible with [slices.SortFunc] and [Config.Sort].
var ByID = func(a, b *Node) int {
	return cmp.Compare(a.id, b.id)
}

// ByLastError compares nodes by their [Node.LastErr] status, sorting the nodes
// with no recorded error first. Since LastErr reverts to nil once traffic flows
// again, this orders nodes by whether they are currently failing.
// It is compatible with [slices.SortFunc] and [Config.Sort].
var ByLastError = func(a, b *Node) int {
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

// ByLatency compares nodes by their current latency estimate in ascending order.
// Nodes with no measurement yet (negative latency value) sort after nodes with a
// measurement. It is compatible with [slices.SortFunc] and [Config.Sort].
var ByLatency = func(a, b *Node) int {
	la, lb := a.Latency(), b.Latency()
	// Note: cmp.Compare alone would sort negative sentinel values first
	// (as the smallest numbers), making unmeasured nodes appear fastest.
	// The switch guards against that by pushing any negative value to the end.
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
var _ stream.PeerNode = peerNode{}
