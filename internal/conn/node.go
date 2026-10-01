package conn

import (
	"cmp"
	"context"
	"fmt"
	"net"
	"slices"
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

// sharedNodeTransport derives a borrower transport from an inbound peer node,
// reusing the peer's channel, latency estimate, and server-space message-ID generator.
// It returns nil if the peer has no transport.
func sharedNodeTransport(peer *Node) *stream.Transport {
	transport := peer.loadTransport()
	if transport == nil {
		return nil
	}
	return stream.NewSharedTransport(transport)
}

// Node encapsulates the state of a node on which a remote procedure call
// can be performed.
type Node struct {
	// Only assigned at creation.
	id   uint32
	addr string
	mgr  *outboundManager // owning manager for this node

	// transport is fixed at construction, like id and addr; a node's channel
	// changes only behind the transport's shared channel reference.
	transport *stream.Transport

	// inboundMu guards liveChannels and the active-channel handoff in
	// attachStream. A peer may briefly have more than one live inbound stream
	// during connection churn; liveChannels holds their channels in attach
	// order, and the last one is the node's active channel.
	inboundMu    sync.Mutex
	liveChannels []*stream.InboundChannel
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
	EagerReconnect bool             // re-establish a lost stream proactively; see [stream.OutboundOptions]
	OnStreamChange func()           // optional; invoked on outbound stream transitions
	Manager        *outboundManager // owning manager
}

// newOutboundNode creates a new node using the provided options. It establishes
// the connection (lazy dial) and initializes the outbound channel.
func newOutboundNode(addr string, opts nodeOptions) (*Node, error) {
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return nil, err
	}

	transport := stream.NewTransport(opts.ID, opts.MsgIDGen)
	n := newNode(opts.ID, tcpAddr.String(), opts.Manager, transport)

	// Create gRPC connection to the node without connecting (lazy dial).
	conn, err := grpc.NewClient(n.addr, opts.DialOpts...)
	if err != nil {
		return nil, NodeError{nodeID: n.id, cause: err}
	}

	// Create outgoing context with metadata for this node's stream.
	md := opts.Metadata.Copy()
	ctx := metadata.NewOutgoingContext(context.Background(), md)

	// Create new outbound channel and establish gRPC node stream
	transport.StoreChannel(stream.NewOutboundChannel(ctx, n.id, conn, stream.OutboundOptions{
		SendBufferSize: opts.SendBufferSize,
		Handler:        opts.RequestHandler,
		Latency:        transport.Latency(),
		EagerReconnect: opts.EagerReconnect,
		OnStreamChange: opts.OnStreamChange,
	}))
	return n, nil
}

// newInboundNode creates a Node for a known peer or client without an active
// channel; the channel is attached when the peer's stream arrives.
func newInboundNode(id uint32, addr string, msgIDGen func() uint64) *Node {
	return newNode(id, addr, nil, stream.NewTransport(id, msgIDGen))
}

// newLocalNode creates a Node that dispatches calls in-process, bypassing the
// network. It is used for the self-node when a server calls its own peers,
// which include itself. The provided handler serves requests directly without
// a gRPC round-trip.
func newLocalNode(id uint32, addr string, msgIDGen func() uint64, handler stream.RequestHandler, mgr *outboundManager) *Node {
	transport := stream.NewTransport(id, msgIDGen)
	n := newNode(id, addr, mgr, transport)
	transport.StoreChannel(stream.NewLocalChannel(id, handler))
	return n
}

// newSharedNode creates a node that reuses the inbound peer node's channel and
// latency estimate. The shared node draws message IDs from the peer node's generator,
// i.e. the server-initiated ID space, so its request IDs cannot collide with
// the remote peer's client-initiated IDs on the same stream. Channel
// replacement on peer reconnect is observed through the shared channel reference.
func newSharedNode(peer *Node, addr string, mgr *outboundManager) *Node {
	return newNode(peer.id, addr, mgr, sharedNodeTransport(peer))
}

// IsInbound returns true if the node has an active inbound channel.
func (n *Node) IsInbound() bool {
	_, ok := n.activeChannel().(*stream.InboundChannel)
	return ok
}

// IsOutbound returns true if the node has an active outbound client channel.
func (n *Node) IsOutbound() bool {
	_, ok := n.activeChannel().(*stream.OutboundChannel)
	return ok
}

// IsShared reports whether the node borrows an inbound peer node's channel,
// as the higher-ID peer of a pair does under stream deduplication. Callers can
// use it to derive their own stream-topology statistics.
func (n *Node) IsShared() bool {
	return n.loadTransport().IsShared()
}

// PendingCount returns the number of calls awaiting responses on the node's
// active channel.
func (n *Node) PendingCount() int {
	ch := n.activeChannel()
	if ch == nil {
		return 0
	}
	return ch.PendingCount()
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
func (n *Node) activeChannel() stream.Channel {
	return n.loadTransport().LoadChannel()
}

// attachStream attaches a new inbound channel to the node when a peer connects
// and returns it, with a detach function to call when that stream ends.
//
// A peer may briefly have more than one live inbound stream: gRPC can open a
// second NodeStream over one connection during connection churn, and the server
// may register the streams in an order that does not match the client's
// creation order. The most recently attached live channel is the node's active
// channel; each channel is closed only when its own stream ends, and when the
// active one ends, the next most recent live channel becomes active. Replies
// to requests received on a stream ride that stream's channel.
//
// detach is idempotent and returns true only when it removed the node's last
// live channel (the peer left the configuration).
func (n *Node) attachStream(streamCtx context.Context, inboundStream stream.BidiStream, opts stream.InboundOptions) (newCh *stream.InboundChannel, detach func() bool) {
	transport := n.loadTransport()
	opts.Latency = transport.Latency()
	newCh = stream.NewInboundChannel(streamCtx, n.id, inboundStream, opts)
	n.inboundMu.Lock()
	n.liveChannels = append(n.liveChannels, newCh)
	transport.StoreChannel(newCh)
	n.inboundMu.Unlock()
	return newCh, func() bool {
		n.inboundMu.Lock()
		defer n.inboundMu.Unlock()
		i := slices.Index(n.liveChannels, newCh)
		if i < 0 {
			return false // already detached
		}
		n.liveChannels = slices.Delete(n.liveChannels, i, i+1)
		_ = newCh.Close()
		if len(n.liveChannels) == 0 {
			transport.StoreChannel(nil)
			return true
		}
		transport.StoreChannel(n.liveChannels[len(n.liveChannels)-1])
		return false
	}
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
	return n.loadTransport().Latency().Load()
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
