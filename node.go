package gorums

import (
	"context"
	"fmt"
	"hash/fnv"
	"net"
	"sort"
	"strconv"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const nilAngleString = "<nil>"

// NodeContext is a context that carries a node for unicast and RPC calls.
// It embeds context.Context and provides access to the Node.
//
// Use [WithNodeContext] to create a NodeContext from an existing context.
type NodeContext struct {
	context.Context
	node *Node
}

// WithNodeContext creates a new NodeContext from the given parent context
// and node. The node must not be nil.
//
// Example:
//
//	ctx := gorums.WithNodeContext(context.Background(), node)
//	resp, err := service.GRPCCall(ctx, req)
func WithNodeContext(parent context.Context, node *Node) *NodeContext {
	if node == nil {
		panic("gorums: WithNodeContext called with nil node")
	}
	return &NodeContext{Context: parent, node: node}
}

// Node returns the Node associated with this context.
func (c NodeContext) Node() *Node {
	return c.node
}

// enqueue enqueues a request to this node's channel.
func (c NodeContext) enqueue(req request) {
	c.node.channel.enqueue(req)
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
	mgr  *Manager // only used for backward compatibility to allow Configuration.Manager()

	msgIDGen func() uint64
	channel  *channel
}

// nodeOptions contains configuration options for creating a new Node.
type nodeOptions struct {
	ID             uint32
	SendBufferSize uint
	MsgIDGen       func() uint64
	Metadata       metadata.MD
	PerNodeMD      func(uint32) metadata.MD
	DialOpts       []grpc.DialOption
	Manager        *Manager // only used for backward compatibility to allow Configuration.Manager()
}

// newNode creates a new node using the provided options.
// It establishes the connection (lazy dial) and initializes the channel.
func newNode(addr string, opts nodeOptions) (*Node, error) {
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return nil, err
	}

	n := &Node{
		id:       opts.ID,
		addr:     tcpAddr.String(),
		mgr:      opts.Manager,
		msgIDGen: opts.MsgIDGen,
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

	// Create channel and establish gRPC node stream
	n.channel = newChannel(ctx, conn, n.id, opts.SendBufferSize)
	return n, nil
}

// nodeID returns the ID for the provided address.
// It resolves the address to ensure the ID is consistent.
func nodeID(addr string) (uint32, error) {
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return 0, err
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(tcpAddr.String()))
	return h.Sum32(), nil
}

// close this node.
func (n *Node) close() error {
	if n.channel != nil {
		return n.channel.close()
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
	return n.channel.lastErr()
}

// Latency returns the latency between the client and this node.
func (n *Node) Latency() time.Duration {
	return n.channel.channelLatency()
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
	if n1.channel.lastErr() != nil && n2.channel.lastErr() == nil {
		return false
	}
	return true
}
