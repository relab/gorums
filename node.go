package gorums

import (
	"context"
	"fmt"
	"hash/fnv"
	"net"
	"sort"
	"strconv"
	"sync"
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
type RawNode struct {
	// Only assigned at creation.
	id            uint32
	addr          string
	conn          *grpc.ClientConn
	cancel        func()
	mgr           *RawManager
	dialMut       sync.Mutex
	connectionMut sync.Mutex

	// the default channel
	channel *channel
}

// NewRawNode returns a new node for the provided address.
func NewRawNode(addr string) (*RawNode, error) {
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return nil, err
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(tcpAddr.String()))
	return &RawNode{
		id:   h.Sum32(),
		addr: tcpAddr.String(),
	}, nil
}

// NewRawNodeWithID returns a new node for the provided address and id.
func NewRawNodeWithID(addr string, id uint32) (*RawNode, error) {
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return nil, err
	}
	return &RawNode{
		id:   id,
		addr: tcpAddr.String(),
	}, nil
}

// connect to this node and associate it with the manager.
func (n *RawNode) connect(mgr *RawManager) error {
	n.mgr = mgr
	if n.mgr.opts.noConnect {
		return nil
	}
	n.channel = newChannel(n)
	//if err := n.channel.connect(); err != nil {
	//	return nodeError{nodeID: n.id, cause: err}
	//}
	return nil
}

// dial the node and close the current connection.
func (n *RawNode) dial() error {
	n.dialMut.Lock()
	defer n.dialMut.Unlock()
	if n.conn != nil {
		// close the current connection before dialing again.
		n.conn.Close()
	}
	var err error
	ctx, cancel := context.WithTimeout(context.Background(), n.mgr.opts.nodeDialTimeout)
	defer cancel()
	n.conn, err = grpc.DialContext(ctx, n.addr, n.mgr.opts.grpcDialOpts...)
	return err
}

// newContext returns a new context for this node's channel.
// This context is used by the channel implementation to stop
// all goroutines and the NodeStream, when the context is canceled.
//
// This method must be called for each connection to ensure
// fresh contexts. Reusing contexts could result in reusing
// a cancelled context.
func (n *RawNode) newContext() context.Context {
	n.dialMut.Lock()
	defer n.dialMut.Unlock()
	md := n.mgr.opts.metadata.Copy()
	if n.mgr.opts.perNodeMD != nil {
		md = metadata.Join(md, n.mgr.opts.perNodeMD(n.id))
	}
	//md = metadata.Join(md, metadata.Pairs("publicKey", n.mgr.publicKey))
	var ctx context.Context
	ctx, n.cancel = context.WithCancel(context.Background())
	return metadata.NewOutgoingContext(ctx, md)
}

// close this node.
func (n *RawNode) close() error {
	n.dialMut.Lock()
	defer n.dialMut.Unlock()
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
func (n *RawNode) ID() uint32 {
	if n != nil {
		return n.id
	}
	return 0
}

// Address returns network address of n.
func (n *RawNode) Address() string {
	if n != nil {
		return n.addr
	}
	return nilAngleString
}

// Host returns the network host of n.
func (n *RawNode) Host() string {
	if n == nil {
		return nilAngleString
	}
	host, _, _ := net.SplitHostPort(n.addr)
	return host
}

// Port returns network port of n.
func (n *RawNode) Port() string {
	if n != nil {
		_, port, _ := net.SplitHostPort(n.addr)
		return port
	}
	return nilAngleString
}

func (n *RawNode) String() string {
	if n != nil {
		return fmt.Sprintf("addr: %s", n.addr)
	}
	return nilAngleString
}

// FullString returns a more descriptive string representation of n that
// includes id, network address and latency information.
func (n *RawNode) FullString() string {
	if n != nil {
		return fmt.Sprintf("node %d | addr: %s", n.id, n.addr)
	}
	return nilAngleString
}

// LastErr returns the last error encountered (if any) for this node.
func (n *RawNode) LastErr() error {
	return n.channel.lastErr()
}

// Latency returns the latency between the client and this node.
func (n *RawNode) Latency() time.Duration {
	return n.channel.channelLatency()
}

type lessFunc func(n1, n2 *RawNode) bool

// MultiSorter implements the Sort interface, sorting the nodes within.
type MultiSorter struct {
	nodes []*RawNode
	less  []lessFunc
}

// Sort sorts the argument slice according to the less functions passed to
// OrderedBy.
func (ms *MultiSorter) Sort(nodes []*RawNode) {
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
var ID = func(n1, n2 *RawNode) bool {
	return n1.id < n2.id
}

// Port sorts nodes by their port number in increasing order.
// Warning: This function may be removed in the future.
var Port = func(n1, n2 *RawNode) bool {
	p1, _ := strconv.Atoi(n1.Port())
	p2, _ := strconv.Atoi(n2.Port())
	return p1 < p2
}

// LastNodeError sorts nodes by their LastErr() status in increasing order. A
// node with LastErr() != nil is larger than a node with LastErr() == nil.
var LastNodeError = func(n1, n2 *RawNode) bool {
	if n1.channel.lastErr() != nil && n2.channel.lastErr() == nil {
		return false
	}
	return true
}
