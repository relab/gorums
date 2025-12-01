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
// It embeds context.Context and provides access to the RawNode.
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

// Node returns the RawNode associated with this context.
func (c *NodeContext) Node() *Node {
	return c.node
}

// Node encapsulates the state of a node on which a remote procedure call
// can be performed.
//
// This struct is intended to be used by generated code.
// You should use the generated `Node` struct instead.
type Node struct {
	// Only assigned at creation.
	id     uint32
	addr   string
	conn   *grpc.ClientConn
	cancel func()
	mgr    *Manager

	// the default channel
	channel *channel
}

// NewNode returns a new node for the provided address.
func NewNode(addr string) (*Node, error) {
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return nil, err
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(tcpAddr.String()))
	return &Node{
		id:   h.Sum32(),
		addr: tcpAddr.String(),
	}, nil
}

// NewNodeWithID returns a new node for the provided address and id.
func NewNodeWithID(addr string, id uint32) (*Node, error) {
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return nil, err
	}
	return &Node{
		id:   id,
		addr: tcpAddr.String(),
	}, nil
}

// connect to this node and associate it with the manager.
func (n *Node) connect(mgr *Manager) (err error) {
	n.mgr = mgr
	if n.mgr.opts.noConnect {
		return nil
	}

	// Create gRPC connection
	n.conn, err = grpc.NewClient(n.addr, n.mgr.opts.grpcDialOpts...)
	if err != nil {
		return nodeError{nodeID: n.id, cause: err}
	}

	// Create channel and establish gRPC node stream
	n.channel = newChannel(n)
	if err := n.channel.ensureStream(); err != nil {
		return nodeError{nodeID: n.id, cause: err}
	}
	return nil
}

// newContext returns a new context for this node's channel.
// This context is used by the channel implementation to stop
// all goroutines and the NodeStream, when the context is canceled.
//
// This method must be called for each connection to ensure
// fresh contexts. Reusing contexts could result in reusing
// a cancelled context.
func (n *Node) newContext() context.Context {
	md := n.mgr.opts.metadata.Copy()
	if n.mgr.opts.perNodeMD != nil {
		md = metadata.Join(md, n.mgr.opts.perNodeMD(n.id))
	}
	var ctx context.Context
	ctx, n.cancel = context.WithCancel(context.Background())
	return metadata.NewOutgoingContext(ctx, md)
}

// close this node.
func (n *Node) close() error {
	// important to cancel first to stop goroutines
	n.cancel()
	if n.conn == nil {
		return nil
	}
	if err := n.conn.Close(); err != nil {
		return nodeError{nodeID: n.id, cause: err}
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
