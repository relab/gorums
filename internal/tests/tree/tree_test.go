package tree_test

import (
	"sync"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/internal/tests/tree"
)

// aggregatorSrv implements tree.TreeAggregatorServer.
// Each instance holds a fixed local value and tracks received broadcasts.
type aggregatorSrv struct {
	value    int32 // local contribution to Aggregate
	wg       *sync.WaitGroup
	mu       sync.Mutex
	received []string // texts received via Broadcast
}

// Broadcast relays the notification to the node's tree children before
// applying local logic. Leaves skip the relay because ctx.TreeChildren()
// returns nil. This implements the downward fan-out half of a tree multicast.
func (s *aggregatorSrv) Broadcast(ctx gorums.ServerCtx, req *tree.BroadcastRequest) {
	if children := ctx.TreeChildren(); len(children) > 0 {
		// Relay to children; errors are not recoverable in a one-way call.
		_ = tree.Broadcast(children.Context(ctx), req)
	}
	s.mu.Lock()
	s.received = append(s.received, req.GetText())
	s.mu.Unlock()
	if s.wg != nil {
		s.wg.Done()
	}
}

// Aggregate sums this node's value with the totals returned by its children.
// Leaves return just their own value. This implements the upward aggregation
// half of a tree quorum call.
func (s *aggregatorSrv) Aggregate(ctx gorums.ServerCtx, req *tree.AggregateRequest) (*tree.AggregateResponse, error) {
	total := s.value
	if children := ctx.TreeChildren(); len(children) > 0 {
		// Release the handler lock before waiting on child responses to allow
		// this server to process other inbound requests concurrently.
		ctx.Release()
		for r := range tree.Aggregate(children.Context(ctx), req).Results() {
			if r.Err == nil {
				total += r.Value.GetTotal()
			}
		}
	}
	return tree.AggregateResponse_builder{Total: total}.Build(), nil
}

// setupTree creates n fully-connected systems, builds a tree from each
// system's own outbound config, and registers the tree and service on each
// server. The caller owns cleanup via the gorums.TestSystems t.Cleanup hook.
//
// The returned tree is built from systems[0]'s connections and is used
// client-side via clientTree.Context(ctx).
func setupTree(t *testing.T, srvs []*aggregatorSrv, opts gorums.TreeOptions) *gorums.TreeConfiguration {
	t.Helper()
	n := len(srvs)
	systems := gorums.TestSystems(t, n)

	for i, sys := range systems {
		ctx := gorums.TestContext(t, 5*time.Second)
		if err := sys.WaitForConfig(ctx, func(cfg gorums.Configuration) bool {
			return cfg.Size() == n
		}); err != nil {
			t.Fatalf("system %d: WaitForConfig: %v", i+1, err)
		}
	}

	clientTree, err := systems[0].OutboundConfig().AsTree(opts)
	if err != nil {
		t.Fatalf("AsTree: %v", err)
	}

	// Each server registers its own tree so ctx.TreeChildren() returns nodes
	// backed by that server's outbound connections, not the client's.
	for i, sys := range systems {
		sysTree, err := sys.OutboundConfig().AsTree(opts)
		if err != nil {
			t.Fatalf("system %d: AsTree: %v", i+1, err)
		}
		srv := srvs[i]
		sys.RegisterService(nil, func(gorumsSrv *gorums.Server) {
			gorumsSrv.RegisterTree(sysTree)
			tree.RegisterTreeAggregatorServer(gorumsSrv, srv)
		})
	}
	return clientTree
}

// TestTreeBroadcast verifies that Broadcast fans out along the tree: the caller
// delivers to root's direct children, each internal node relays to its children,
// and leaves apply local logic only. With bf=2 and depth=2:
//
//	    1 (root)
//	   / \
//	  2   3
//	 / \ / \
//	4  5 6  7
//
// The caller sends to {2, 3} via clientTree.Context(ctx).
// Node 2 relays to {4, 5}; node 3 relays to {6, 7}.
// Six nodes receive the notification; the root does not.
func TestTreeBroadcast(t *testing.T) {
	const n = 7
	opts := gorums.TreeOptions{BranchingFactor: 2, Depth: 2}

	var wg sync.WaitGroup
	wg.Add(n - 1) // all nodes except the root

	srvs := make([]*aggregatorSrv, n)
	for i := range n {
		srvs[i] = &aggregatorSrv{wg: &wg}
	}

	clientTree := setupTree(t, srvs, opts)

	ctx := gorums.TestContext(t, 2*time.Second)
	if err := tree.Broadcast(clientTree.Context(ctx), tree.BroadcastRequest_builder{Text: "hello"}.Build()); err != nil {
		t.Fatalf("Broadcast: %v", err)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout: not all nodes received the broadcast")
	}

	// Root (node 1, srvs[0]) is the logical caller and must not receive the broadcast.
	srvs[0].mu.Lock()
	rootCount := len(srvs[0].received)
	srvs[0].mu.Unlock()
	if rootCount != 0 {
		t.Errorf("root received %d messages, want 0", rootCount)
	}
	// Every other node must have received exactly one message.
	for i := 1; i < n; i++ {
		srvs[i].mu.Lock()
		got := len(srvs[i].received)
		srvs[i].mu.Unlock()
		if got != 1 {
			t.Errorf("node %d received %d messages, want 1", i+1, got)
		}
	}
}

// TestTreeAggregate verifies that Aggregate collects subtree totals up the
// tree. Each node i has value i+1 (its node ID). Internal nodes sum the
// children's sub-totals and add their own value. The caller sums the two
// direct-child subtree results.
//
// With bf=2 and depth=2:
//   - Node 2 returns 2+4+5 = 11
//   - Node 3 returns 3+6+7 = 16
//   - Caller sums: 11+16 = 27  (nodes 2–7; root is not in the call path)
func TestTreeAggregate(t *testing.T) {
	const n = 7
	opts := gorums.TreeOptions{BranchingFactor: 2, Depth: 2}

	srvs := make([]*aggregatorSrv, n)
	for i := range n {
		srvs[i] = &aggregatorSrv{value: int32(i + 1)} // node IDs are 1-based
	}

	clientTree := setupTree(t, srvs, opts)

	ctx := gorums.TestContext(t, 3*time.Second)
	responses := tree.Aggregate(clientTree.Context(ctx), &tree.AggregateRequest{})

	const wantTotal = int32(27) // 2+3+4+5+6+7; root (node 1) is not in the call path
	gotTotal := int32(0)
	for r := range responses.Results() {
		if r.Err != nil {
			t.Fatalf("Aggregate error from node %d: %v", r.NodeID, r.Err)
		}
		gotTotal += r.Value.GetTotal()
	}
	if gotTotal != wantTotal {
		t.Errorf("Aggregate total: got %d, want %d", gotTotal, wantTotal)
	}
}
