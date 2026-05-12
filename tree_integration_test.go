package gorums_test

import (
	"sync"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/internal/testutils/mock"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

// setupTreeSystems creates n fully-connected systems and registers a matching
// TreeConfiguration on each server. Each server's tree is built from that
// server's own outbound config so that ctx.TreeChildren() returns nodes with
// live connections owned by that server.
//
// The returned tree is built from systems[0]'s outbound config and can be used
// as the client-side tree (e.g. to call tree.Context(ctx)).
//
// awaitSystemReady is called before returning so all systems are ready for use.
func setupTreeSystems(t *testing.T, n int, opts gorums.TreeOptions) ([]*gorums.System, *gorums.TreeConfiguration) {
	t.Helper()
	systems := gorums.TestSystems(t, n)
	awaitSystemReady(t, systems)

	clientTree, err := systems[0].OutboundConfig().AsTree(opts)
	if err != nil {
		t.Fatalf("AsTree: %v", err)
	}

	// Each server needs its own tree so ctx.TreeChildren() returns nodes
	// backed by that server's own outbound connections.
	for _, sys := range systems {
		sysTree, err := sys.OutboundConfig().AsTree(opts)
		if err != nil {
			t.Fatalf("AsTree: %v", err)
		}
		sys.RegisterService(nil, func(srv *gorums.Server) {
			srv.RegisterTree(sysTree)
		})
	}
	return systems, clientTree
}

// TestTreeMulticast verifies that Multicast fans out along the tree: the caller
// delivers to the root's direct children, each internal node relays to its own
// children, and leaves apply local logic. With bf=2, depth=2:
//
//	    1 (root)
//	   / \
//	  2   3
//	 / \ / \
//	4  5 6  7
//
// The caller sends to nodes {2, 3} via tree.Context(ctx).
// Nodes 2 and 3 relay to {4, 5} and {6, 7} respectively.
// Total handler invocations: 6 (all nodes except the logical caller root).
func TestTreeMulticast(t *testing.T) {
	opts := gorums.TreeOptions{BranchingFactor: 2, Depth: 2}
	systems, tree := setupTreeSystems(t, 7, opts)

	var wg sync.WaitGroup
	wg.Add(6) // nodes 2–7; root (node 1) is not in the call path

	for _, sys := range systems {
		sys.RegisterService(nil, func(srv *gorums.Server) {
			srv.RegisterHandler(mock.Stream, func(ctx gorums.ServerCtx, in *gorums.Message) (*gorums.Message, error) {
				// Relay to children before running local logic.
				// Internal nodes fan out; leaves skip this branch.
				if children := ctx.TreeChildren(); len(children) > 0 {
					req := gorums.AsProto[*pb.StringValue](in)
					if err := gorums.Multicast(children.Context(ctx), req, mock.Stream); err != nil {
						t.Errorf("relay multicast error: %v", err)
					}
				}
				wg.Done()
				return nil, nil
			})
		})
	}

	ctx := gorums.TestContext(t, 2*time.Second)
	// tree.Context(ctx) addresses root's children {2, 3} — the caller acts as
	// the external root; relay to further descendants is handled by the handlers.
	if err := gorums.Multicast(tree.Context(ctx), pb.String("ping"), mock.Stream); err != nil {
		t.Fatalf("multicast error: %v", err)
	}

	waitWithTimeout(t, &wg)
}

// TestTreeQuorumCall verifies that a QuorumCall aggregates responses up the tree.
// Each node i contributes its node ID as its local value. Internal nodes sum
// their children's aggregated replies and add their own contribution before
// returning. The caller sums the two subtree totals.
//
// With bf=2, depth=2:
//
//	    1 (root)
//	   / \
//	  2   3
//	 / \ / \
//	4  5 6  7
//
// Node 2 returns 2+4+5 = 11; node 3 returns 3+6+7 = 16.
// The caller sums both subtree results: 11+16 = 27.
func TestTreeQuorumCall(t *testing.T) {
	opts := gorums.TreeOptions{BranchingFactor: 2, Depth: 2}
	systems, tree := setupTreeSystems(t, 7, opts)

	for i, sys := range systems {
		myVal := int32(i + 1) // node ID: system[0] → 1, system[1] → 2, …
		sys.RegisterService(nil, func(srv *gorums.Server) {
			srv.RegisterHandler(mock.GetValueMethod, func(ctx gorums.ServerCtx, in *gorums.Message) (*gorums.Message, error) {
				total := myVal
				if children := ctx.TreeChildren(); len(children) > 0 {
					// Release before blocking on child responses; allows the
					// server to process other incoming requests concurrently.
					ctx.Release()
					childResp := gorums.QuorumCall[*pb.Int32Value, *pb.Int32Value](
						children.Context(ctx),
						gorums.AsProto[*pb.Int32Value](in),
						mock.GetValueMethod,
					)
					for r := range childResp.Results() {
						if r.Err == nil {
							total += r.Value.GetValue()
						}
					}
				}
				return gorums.NewResponseMessage(in, pb.Int32(total)), nil
			})
		})
	}

	ctx := gorums.TestContext(t, 3*time.Second)
	responses := gorums.QuorumCall[*pb.Int32Value, *pb.Int32Value](
		tree.Context(ctx),
		pb.Int32(0),
		mock.GetValueMethod,
	)

	const wantTotal = int32(27) // 2+3+4+5+6+7; root (node 1) is not in the call path
	gotTotal := int32(0)
	for r := range responses.Results() {
		if r.Err != nil {
			t.Fatalf("quorum call error from node %d: %v", r.NodeID, r.Err)
		}
		gotTotal += r.Value.GetValue()
	}
	if gotTotal != wantTotal {
		t.Errorf("tree aggregate sum: got %d, want %d", gotTotal, wantTotal)
	}
}
