package gorums

import (
	"cmp"
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

// RawNode encapsulates the state of a node on which a remote procedure call
// can be performed.
//
// This struct is intended to be used by generated code.
// You should use the generated `Node` struct instead.
type RawNode[idType cmp.Ordered] struct {
	// Only assigned at creation.
	id     idType
	addr   string
	conn   *grpc.ClientConn
	cancel func()
	mgr    *RawManager[idType]

	// the default channel
	channel *channel[idType]
}

// NewRawNode returns a new node for the provided address.
func NewRawNode[idType cmp.Ordered](addr string) (*RawNode[idType], error) {
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return nil, err
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(tcpAddr.String()))
	var id idType
	return &RawNode[idType]{
		id:   id, // h.Sum32()
		addr: tcpAddr.String(),
	}, nil
}

// NewRawNodeWithID returns a new node for the provided address and id.
func NewRawNodeWithID[idType cmp.Ordered](addr string, id idType) (*RawNode[idType], error) {
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return nil, err
	}
	return &RawNode[idType]{
		id:   id,
		addr: tcpAddr.String(),
	}, nil
}

// connect to this node and associate it with the manager.
func (n *RawNode[idType]) connect(mgr *RawManager[idType]) error {
	n.mgr = mgr
	if n.mgr.opts.noConnect {
		return nil
	}
	n.channel = newChannel(n)
	if err := n.channel.connect(); err != nil {
		return nodeError[idType]{nodeID: n.id, cause: err}
	}
	return nil
}

// dial the node and close the current connection.
func (n *RawNode[idType]) dial() error {
	if n.conn != nil {
		// close the current connection before dialing again.
		n.conn.Close()
	}
	var err error
	n.conn, err = grpc.NewClient(n.addr, n.mgr.opts.grpcDialOpts...)
	return err
}

// newContext returns a new context for this node's channel.
// This context is used by the channel implementation to stop
// all goroutines and the NodeStream, when the context is canceled.
//
// This method must be called for each connection to ensure
// fresh contexts. Reusing contexts could result in reusing
// a cancelled context.
func (n *RawNode[idType]) newContext() context.Context {
	md := n.mgr.opts.metadata.Copy()
	if n.mgr.opts.perNodeMD != nil {
		md = metadata.Join(md, n.mgr.opts.perNodeMD(n.id))
	}
	var ctx context.Context
	ctx, n.cancel = context.WithCancel(context.Background())
	return metadata.NewOutgoingContext(ctx, md)
}

// close this node.
func (n *RawNode[idType]) close() error {
	// important to cancel first to stop goroutines
	n.cancel()
	if n.conn == nil {
		return nil
	}
	if err := n.conn.Close(); err != nil {
		return nodeError[idType]{nodeID: n.id, cause: err}
	}
	return nil
}

// ID returns the ID of n.
func (n *RawNode[idType]) ID() idType {
	if n != nil {
		return n.id
	}
	var zero idType
	return zero
}

// Address returns network address of n.
func (n *RawNode[idType]) Address() string {
	if n != nil {
		return n.addr
	}
	return nilAngleString
}

// Host returns the network host of n.
func (n *RawNode[idType]) Host() string {
	if n == nil {
		return nilAngleString
	}
	host, _, _ := net.SplitHostPort(n.addr)
	return host
}

// Port returns network port of n.
func (n *RawNode[idType]) Port() string {
	if n != nil {
		_, port, _ := net.SplitHostPort(n.addr)
		return port
	}
	return nilAngleString
}

func (n *RawNode[idType]) String() string {
	if n != nil {
		return fmt.Sprintf("addr: %s", n.addr)
	}
	return nilAngleString
}

// FullString returns a more descriptive string representation of n that
// includes id, network address and latency information.
func (n *RawNode[idType]) FullString() string {
	if n != nil {
		return fmt.Sprintf("node %v | addr: %s", n.id, n.addr)
	}
	return nilAngleString
}

// LastErr returns the last error encountered (if any) for this node.
func (n *RawNode[idType]) LastErr() error {
	return n.channel.lastErr()
}

// Latency returns the latency between the client and this node.
func (n *RawNode[idType]) Latency() time.Duration {
	return n.channel.channelLatency()
}

type lessFunc[idType cmp.Ordered] func(n1, n2 *RawNode[idType]) bool

// MultiSorter implements the Sort interface, sorting the nodes within.
type MultiSorter[idType cmp.Ordered] struct {
	nodes []*RawNode[idType]
	less  []lessFunc[idType]
}

// Sort sorts the argument slice according to the less functions passed to
// OrderedBy.
func (ms *MultiSorter[idType]) Sort(nodes []*RawNode[idType]) {
	ms.nodes = nodes
	sort.Sort(ms)
}

// OrderedBy returns a Sorter that sorts using the less functions, in order.
// Call its Sort method to sort the data.
func OrderedBy[idType cmp.Ordered](less ...lessFunc[idType]) *MultiSorter[idType] {
	return &MultiSorter[idType]{
		less: less,
	}
}

// Len is part of sort.Interface.
func (ms *MultiSorter[idType]) Len() int {
	return len(ms.nodes)
}

// Swap is part of sort.Interface.
func (ms *MultiSorter[idType]) Swap(i, j int) {
	ms.nodes[i], ms.nodes[j] = ms.nodes[j], ms.nodes[i]
}

// Less is part of sort.Interface. It is implemented by looping along the
// less functions until it finds a comparison that is either Less or not
// Less. Note that it can call the less functions twice per call. We
// could change the functions to return -1, 0, 1 and reduce the
// number of calls for greater efficiency: an exercise for the reader.
func (ms *MultiSorter[idType]) Less(i, j int) bool {
	p, q := ms.nodes[i], ms.nodes[j]
	// Try all but the last comparison.
	var k int
	for k = 0; k < len(ms.less)-1; k++ {
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
func ID[idType cmp.Ordered](n1, n2 *RawNode[idType]) bool {
	return n1.id < n2.id
}

// Port sorts nodes by their port number in increasing order.
// Warning: This function may be removed in the future.
func Port[idType cmp.Ordered](n1, n2 *RawNode[idType]) bool {
	p1, _ := strconv.Atoi(n1.Port())
	p2, _ := strconv.Atoi(n2.Port())
	return p1 < p2
}

// LastNodeError sorts nodes by their LastErr() status in increasing order. A
// node with LastErr() != nil is larger than a node with LastErr() == nil.
func LastNodeError[idType cmp.Ordered](n1, n2 *RawNode[idType]) bool {
	if n1.channel.lastErr() != nil && n2.channel.lastErr() == nil {
		return false
	}
	return true
}
