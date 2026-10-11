package gorums_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/gorumstest"
	"github.com/relab/gorums/internal/testutils/mock"
	gorumsimpl "github.com/relab/gorums/runtime/gorumsimpl"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

// The tests in this file show one way to use a [gorums.TreeConfig] with
// symmetric peers: every server builds the same tree over its own peer
// configuration, and relays to its own children from inside its handler.
// The server whose node is the root takes part in the protocol and starts the
// calls itself. The tree is plain topology; the server needs no tree
// registration. A handler finds its position with its server's NodeID.
//
// With bf=2 and depth=2 over seven servers:
//
//	    1 (root)
//	   / \
//	  2   3
//	 / \ / \
//	4  5 6  7

// setupTreeServers starts n connected peer servers and returns them together
// with one tree per server, each built over that server's own peer
// configuration, which is in node-ID order.
func setupTreeServers(t *testing.T, n int, dedup bool, opts gorums.TreeOptions) ([]*gorums.Server, []*gorums.TreeConfig) {
	t.Helper()
	var srvOpts []gorums.ServerOption
	if dedup {
		srvOpts = append(srvOpts, gorums.WithStreamDedup())
	}
	servers := gorumstest.LocalServers(t, n, srvOpts...)
	gorumstest.WaitForPeers(t, servers)
	trees := make([]*gorums.TreeConfig, n)
	for i, srv := range servers {
		tree, err := gorums.NewTree(srv.PeerConfig(), opts)
		if err != nil {
			t.Fatalf("server %d: NewTree: %v", i+1, err)
		}
		trees[i] = tree
	}
	return servers, trees
}

// TestTreeMulticast verifies a hop-by-hop multicast: the root multicasts to
// its children only, and each internal node relays to its own children.
// Every node except the root receives the message exactly once; the root is
// the sender and handles it locally.
func TestTreeMulticast(t *testing.T) {
	for _, dedup := range []bool{false, true} {
		t.Run(fmt.Sprintf("Dedup=%t", dedup), func(t *testing.T) {
			const n = 7
			servers, trees := setupTreeServers(t, n, dedup, gorums.TreeOptions{BranchingFactor: 2, Depth: 2})

			var mu sync.Mutex
			received := make(map[gorums.ID]int)
			var wg sync.WaitGroup
			wg.Add(n - 1)

			for i, srv := range servers {
				tree := trees[i]
				srv.RegisterHandler(mock.TestMethod, func(ctx gorums.ServerContext, in *gorums.Message) (*gorums.Message, error) {
					ctx.Release()
					if children := tree.ChildrenOf(srv.NodeID()); len(children) > 0 {
						req := gorums.AsProto[*pb.StringValue](in)
						if err := gorumsimpl.Multicast(children.Context(ctx), req, mock.TestMethod).Send(); err != nil {
							t.Errorf("node %d: relay: %v", srv.NodeID(), err)
						}
					}
					mu.Lock()
					received[srv.NodeID()]++
					mu.Unlock()
					wg.Done()
					return nil, nil
				})
			}

			ctx := gorumstest.Context(t, 5*time.Second)
			root := trees[0].Root().ID()
			if err := gorumsimpl.Multicast(trees[0].ChildrenOf(root).Context(ctx), pb.String("ping"), mock.TestMethod).Send(); err != nil {
				t.Fatalf("multicast: %v", err)
			}
			waitWithTimeout(t, &wg)

			mu.Lock()
			defer mu.Unlock()
			for id := gorums.ID(2); id <= n; id++ {
				if received[id] != 1 {
					t.Errorf("node %d received %d messages, want 1", id, received[id])
				}
			}
			if received[root] != 0 {
				t.Errorf("root received %d messages, want 0", received[root])
			}
		})
	}
}

// TestTreeQuorumCall verifies hop-by-hop aggregation with a blocking relay:
// each internal node calls its children from inside its handler, after
// Release, and adds its own value, which is its node ID. The root is a
// participating replica: it calls its children and adds its own value, so
// the result is the sum over all seven nodes. Node 2 returns 2+4+5 = 11 and
// node 3 returns 3+6+7 = 16.
func TestTreeQuorumCall(t *testing.T) {
	for _, dedup := range []bool{false, true} {
		t.Run(fmt.Sprintf("Dedup=%t", dedup), func(t *testing.T) {
			const n = 7
			servers, trees := setupTreeServers(t, n, dedup, gorums.TreeOptions{BranchingFactor: 2, Depth: 2})

			// aggregate returns the sum of the node's own value and the
			// totals of its children.
			aggregate := func(ctx gorums.ServerContext, tree *gorums.TreeConfig, me gorums.ID) (int32, error) {
				total := int32(me)
				children := tree.ChildrenOf(me)
				if len(children) == 0 {
					return total, nil
				}
				for r := range gorumsimpl.QuorumCall[*pb.Int32Value, *pb.Int32Value](children.Context(ctx), pb.Int32(0), mock.GetValueMethod).Results() {
					if r.Err != nil {
						return 0, fmt.Errorf("child %d: %w", r.NodeID, r.Err)
					}
					total += r.Value.GetValue()
				}
				return total, nil
			}

			for i, srv := range servers {
				tree := trees[i]
				srv.RegisterHandler(mock.GetValueMethod, func(ctx gorums.ServerContext, in *gorums.Message) (*gorums.Message, error) {
					// Release before blocking on the children, so that the
					// next request on this stream can start meanwhile.
					ctx.Release()
					total, err := aggregate(ctx, tree, srv.NodeID())
					if err != nil {
						return nil, err
					}
					return gorums.NewResponseMessage(in, pb.Int32(total)), nil
				})
			}

			// The root runs the same aggregation as every other node, with
			// the test context in place of a request context.
			ctx := gorumstest.Context(t, 5*time.Second)
			total := int32(trees[0].Root().ID())
			for r := range gorumsimpl.QuorumCall[*pb.Int32Value, *pb.Int32Value](
				trees[0].ChildrenOf(trees[0].Root().ID()).Context(ctx), pb.Int32(0), mock.GetValueMethod,
			).Results() {
				if r.Err != nil {
					t.Fatalf("quorum call, node %d: %v", r.NodeID, r.Err)
				}
				total += r.Value.GetValue()
			}
			if want := int32(28); total != want { // 1+2+...+7
				t.Errorf("tree aggregate = %d, want %d", total, want)
			}
		})
	}
}

// TestTreeExternalCaller verifies that a caller outside the root starts a tree
// call by sending to the root: the root does its own work and relays, so the
// result covers all seven nodes, and the root's work is not skipped.
func TestTreeExternalCaller(t *testing.T) {
	const n = 7
	servers, trees := setupTreeServers(t, n, false, gorums.TreeOptions{BranchingFactor: 2, Depth: 2})

	for i, srv := range servers {
		tree := trees[i]
		srv.RegisterHandler(mock.GetValueMethod, func(ctx gorums.ServerContext, in *gorums.Message) (*gorums.Message, error) {
			ctx.Release()
			total := int32(srv.NodeID())
			if children := tree.ChildrenOf(srv.NodeID()); len(children) > 0 {
				for r := range gorumsimpl.QuorumCall[*pb.Int32Value, *pb.Int32Value](children.Context(ctx), pb.Int32(0), mock.GetValueMethod).Results() {
					if r.Err != nil {
						return nil, fmt.Errorf("child %d: %w", r.NodeID, r.Err)
					}
					total += r.Value.GetValue()
				}
			}
			return gorums.NewResponseMessage(in, pb.Int32(total)), nil
		})
	}

	// The caller is server 4, which uses its own view of the tree to find the root.
	ctx := gorumstest.Context(t, 5*time.Second)
	rootOnly := gorums.Config{trees[3].Root()}
	resp, err := gorumsimpl.QuorumCall[*pb.Int32Value, *pb.Int32Value](rootOnly.Context(ctx), pb.Int32(0), mock.GetValueMethod).First()
	if err != nil {
		t.Fatalf("quorum call to the root: %v", err)
	}
	if want := int32(28); resp.GetValue() != want { // 1+2+...+7, including the root
		t.Errorf("tree aggregate = %d, want %d", resp.GetValue(), want)
	}
}
