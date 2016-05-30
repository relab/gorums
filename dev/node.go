package dev

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"google.golang.org/grpc"
)

// Node encapsulates the state of a node on which a remote procedure call
// can be made.
type Node struct {
	// Only assigned at creation.
	id   int
	gid  uint32
	self bool
	addr string
	conn *grpc.ClientConn

	sync.Mutex
	lastErr error
	latency time.Duration
}

func (n *Node) connect(opts ...grpc.DialOption) error {
	conn, err := grpc.Dial(n.addr, opts...)
	if err != nil {
		return fmt.Errorf("dialing node failed: %v", err)
	}
	n.conn = conn
	return nil
}

// ID returns the local ID of m.
func (n *Node) ID() int {
	return n.id
}

// GlobalID returns the global id of m.
func (n *Node) GlobalID() uint32 {
	return n.gid
}

// Address returns network address of m.
func (n *Node) Address() string {
	return n.addr
}

// ConnState returns the state of the underlying gRPC client connection.
func (n *Node) ConnState() (grpc.ConnectivityState, error) {
	return n.conn.State()
}

func (n *Node) String() string {
	n.Lock()
	defer n.Unlock()
	var connState string
	if n.conn == nil {
		connState = "nil"
	} else {
		cstate, err := n.conn.State()
		if err != nil {
			connState = err.Error()
		} else {
			connState = cstate.String()
		}
	}
	return fmt.Sprintf(
		"node %d | gid: %d | addr: %s | latency: %v | connstate: %v",
		n.id,
		n.gid,
		n.addr,
		n.latency,
		connState,
	)
}

func (n *Node) setLastErr(err error) {
	n.Lock()
	defer n.Unlock()
	n.lastErr = err
}

// LastErr returns the last error encountered (if any) when invoking a remote
// procedure call on this node.
func (n *Node) LastErr() error {
	n.Lock()
	defer n.Unlock()
	return n.lastErr
}

func (n *Node) setLatency(lat time.Duration) {
	n.Lock()
	defer n.Unlock()
	n.latency = lat
}

// Latency returns the latency of the last successful remote procedure call
// made to this node.
func (n *Node) Latency() time.Duration {
	n.Lock()
	defer n.Unlock()
	return n.latency
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

// ID sorts nodes by their local identifier in increasing order.
var ID = func(n1, n2 *Node) bool {
	return n1.id < n2.id
}

// GlobalID sorts nodes by their global identifier in increasing order.
var GlobalID = func(n1, n2 *Node) bool {
	return n1.gid < n2.gid
}

// Latency sorts nodes by latency in increasing order. Latencies less then
// zero (sentinel value) are considered greater than any positive latency.
var Latency = func(n1, n2 *Node) bool {
	if n1.latency < 0 {
		return false
	}
	return n1.latency < n2.latency

}

// Error sorts nodes by their LastErr() status in increasing order. A
// node with LastErr() != nil is larger than a node with LastErr() == nil.
var Error = func(n1, n2 *Node) bool {
	if n1.lastErr != nil && n2.lastErr == nil {
		return false
	}
	return true
}

// Connectivity sorts nodes by "best"/highest connectivity in increasing
// order. The (gRPC) connectivity status is ranked as follows:
// * Ready
// * Connecting
// * Idle
// * TransientFailure
// * Shutdown
var Connectivity = func(n1, n2 *Node) bool {
	n1State, n1Err := n1.conn.State()
	n2State, n2Err := n2.conn.State()
	switch {
	case n1Err != nil && n2Err == nil:
		return false
	case n1Err != nil && n2Err != nil:
		return true
	case n1Err == nil && n2Err != nil:
		return true
	case n1State <= grpc.Ready && n2State <= grpc.Ready:
		// Both are idle/connecting/ready.
		return n1State < n2State
	case n1State > grpc.Ready && n2State > grpc.Ready:
		// Both are transient/shutdown.
		return n1State > n2State
	case n1State > grpc.Ready && n2State < grpc.Ready:
		// n1 is transient/shutdown and n2 is idle/connecting/ready.
		return true
	default:
		// n2 is transient/shutdown and n1 is idle/connecting/ready.
		return false
	}
}

// Temporary to suppress varcheck warning.
var _ = Connectivity
