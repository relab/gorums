package gorums

import (
	"context"
	"fmt"
	"hash/fnv"
	"math/rand"
	"net"
	"sort"
	"strconv"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const nilAngleString = "<nil>"

// Node encapsulates the state of a node on which a remote procedure call
// can be performed.
type Node struct {
	// Only assigned at creation.
	id      uint32
	addr    string
	conn    *grpc.ClientConn
	mu      sync.Mutex
	lastErr error
	latency time.Duration

	*receiveQueue
	opts *managerOptions

	channel    *Channel           // the default channel
	chanCtx    context.Context    // a context given to new channels
	chanCancel context.CancelFunc // cancels all channels
}

// NewNode returns a new node for the provided address.
func NewNode(addr string) (*Node, error) {
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("node error: '%s' error: %v", addr, err)
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(tcpAddr.String()))
	return &Node{
		id:      h.Sum32(),
		addr:    tcpAddr.String(),
		latency: -1 * time.Second,
	}, nil
}

// NewNodeWithID returns a new node for the provided address and id.
func NewNodeWithID(addr string, id uint32) (*Node, error) {
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("node error: '%s' error: %v", addr, err)
	}
	return &Node{
		id:      id,
		addr:    tcpAddr.String(),
		latency: -1 * time.Second,
	}, nil
}

// connect to this node to facilitate gRPC calls and optionally client streams.
func (n *Node) connect(mgr *Manager) error {
	n.opts = &mgr.opts
	n.receiveQueue = mgr.receiveQueue

	if n.opts.noConnect {
		return nil
	}
	var err error
	ctx, cancel := context.WithTimeout(context.Background(), n.opts.nodeDialTimeout)
	defer cancel()
	n.conn, err = grpc.DialContext(ctx, n.addr, n.opts.grpcDialOpts...)
	if err != nil {
		return fmt.Errorf("dialing node failed: %w", err)
	}
	md := n.opts.metadata.Copy()
	if n.opts.perNodeMD != nil {
		md = metadata.Join(md, n.opts.perNodeMD(n.id))
	}
	// a context for all of the streams
	ctx, n.chanCancel = context.WithCancel(context.Background())
	n.chanCtx = metadata.NewOutgoingContext(ctx, md)

	if n.channel, err = n.NewChannel(); err != nil {
		return fmt.Errorf("starting stream failed: %w", err)
	}
	return nil
}

// NewChannel creates a new channel for this Node.
func (n *Node) NewChannel() (*Channel, error) {
	ch := &Channel{
		sendQ:   make(chan request, n.opts.sendBuffer),
		node:    n,
		backoff: n.opts.backoff,
		rand:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	err := ch.connect()
	if err != nil {
		return nil, err
	}
	return ch, nil
}

// Channel returns the default channel for this Node.
func (n *Node) Channel() *Channel {
	return n.channel
}

// close this node for further calls and optionally stream.
func (n *Node) close() error {
	if n.conn == nil {
		return nil
	}
	if err := n.conn.Close(); err != nil {
		return fmt.Errorf("%d: conn close error: %w", n.id, err)
	}
	n.chanCancel()
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
		n.mu.Lock()
		defer n.mu.Unlock()
		return fmt.Sprintf(
			"node %d | addr: %s | latency: %v",
			n.id, n.addr, n.latency,
		)
	}
	return nilAngleString
}

func (n *Node) setLastErr(err error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.lastErr = err
}

// LastErr returns the last error encountered (if any) when invoking a remote
// procedure call on this node.
func (n *Node) LastErr() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.lastErr
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
// less functions until it finds a comparison that is either Less or
// !Less. Note that it can call the less functions twice per call. We
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
	if n1.lastErr != nil && n2.lastErr == nil {
		return false
	}
	return true
}
