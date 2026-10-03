package gorums_test

import (
	"context"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/gorumstest"
	"github.com/relab/gorums/internal/testutils/mock"
	gorumsimpl "github.com/relab/gorums/runtime/gorumsimpl"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

func dispatchNode(t *testing.T, cfg gorums.Config, id uint32) *gorums.Node {
	t.Helper()
	for _, n := range cfg.Nodes() {
		if n.ID() == id {
			return n
		}
	}
	t.Fatalf("node %d not in config", id)
	return nil
}

func dispatchSetup(t *testing.T, dedup bool) []*gorums.Server {
	t.Helper()
	var opts []gorums.ServerOption
	if dedup {
		opts = append(opts, gorums.WithStreamDedup())
	}
	return gorumstest.LocalServers(t, 2, opts...)
}

func dispatchWait(t *testing.T, servers []*gorums.Server) {
	t.Helper()
	ctx := gorumstest.Context(t, 5*time.Second)
	for _, srv := range servers {
		if err := srv.WaitForPeers(ctx, func(c gorums.Config) bool { return c.Size() == len(servers) }); err != nil {
			t.Fatalf("WaitForPeers: %v", err)
		}
	}
}

// Borrower (node 2) is busy handling an owner (node 1) request without
// releasing; the borrower's own unrelated call to node 1 cannot read its reply.
func TestStreamDedupDispatchBorrowerSlowHandlerBlocksOwnCalls(t *testing.T) {
	for _, dedup := range []bool{false, true} {
		name := "Dual"
		if dedup {
			name = "Dedup"
		}
		t.Run(name, func(t *testing.T) {
			servers := dispatchSetup(t, dedup)
			started := make(chan struct{}, 1)
			unblock := make(chan struct{})
			servers[1].RegisterHandler(mock.TestMethod, func(_ gorums.ServerContext, in *gorums.Message) (*gorums.Message, error) {
				started <- struct{}{}
				<-unblock // slow handler, no Release
				return gorums.NewResponseMessage(in, pb.String("slow")), nil
			})
			servers[0].RegisterHandler(mock.EchoMethod, stringEchoHandler("echo"))
			dispatchWait(t, servers)
			defer close(unblock)

			// node 1 -> node 2 slow request
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				n2 := dispatchNode(t, servers[0].PeerConfig(), 2)
				_, _ = gorumsimpl.RemoteCall[*pb.StringValue, *pb.StringValue](n2.Context(ctx), pb.String("x"), mock.TestMethod)
			}()
			<-started

			// node 2 -> node 1 unrelated echo
			n1 := dispatchNode(t, servers[1].PeerConfig(), 1)
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()
			start := time.Now()
			resp, err := gorumsimpl.RemoteCall[*pb.StringValue, *pb.StringValue](n1.Context(ctx), pb.String("hi"), mock.EchoMethod)
			t.Logf("dedup=%v node2->node1 echo: resp=%q err=%v elapsed=%v shared=%v", dedup, resp.GetValue(), err, time.Since(start), n1.IsShared())
			if err != nil {
				t.Errorf("node 2 -> node 1 echo failed while node 2 handles a node-1 request: %v", err)
			}
		})
	}
}

// Owner (node 1) is busy handling two borrower (node 2) requests without
// releasing; the owner's own unrelated call to node 2 cannot read its reply.
func TestStreamDedupDispatchOwnerSlowHandlerBlocksOwnCalls(t *testing.T) {
	for _, dedup := range []bool{false, true} {
		name := "Dual"
		if dedup {
			name = "Dedup"
		}
		t.Run(name, func(t *testing.T) {
			servers := dispatchSetup(t, dedup)
			started := make(chan struct{}, 4)
			unblock := make(chan struct{})
			servers[0].RegisterHandler(mock.TestMethod, func(_ gorums.ServerContext, in *gorums.Message) (*gorums.Message, error) {
				started <- struct{}{}
				<-unblock // slow handler, no Release
				return gorums.NewResponseMessage(in, pb.String("slow")), nil
			})
			servers[1].RegisterHandler(mock.EchoMethod, stringEchoHandler("echo"))
			dispatchWait(t, servers)
			defer close(unblock)

			n1 := dispatchNode(t, servers[1].PeerConfig(), 1)
			for range 2 {
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					_, _ = gorumsimpl.RemoteCall[*pb.StringValue, *pb.StringValue](n1.Context(ctx), pb.String("x"), mock.TestMethod)
				}()
			}
			<-started
			time.Sleep(100 * time.Millisecond) // let the second request arrive

			n2 := dispatchNode(t, servers[0].PeerConfig(), 2)
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()
			start := time.Now()
			resp, err := gorumsimpl.RemoteCall[*pb.StringValue, *pb.StringValue](n2.Context(ctx), pb.String("hi"), mock.EchoMethod)
			t.Logf("dedup=%v node1->node2 echo: resp=%q err=%v elapsed=%v", dedup, resp.GetValue(), err, time.Since(start))
			if err != nil {
				t.Errorf("node 1 -> node 2 echo failed while node 1 handles node-2 requests: %v", err)
			}
		})
	}
}

// Nested call from the borrower's handler back to the requesting owner,
// without Release, as permitted (only discouraged for throughput) by the
// user guide.
func TestStreamDedupDispatchNestedCallToRequesterWithoutRelease(t *testing.T) {
	for _, dedup := range []bool{false, true} {
		name := "Dual"
		if dedup {
			name = "Dedup"
		}
		t.Run(name, func(t *testing.T) {
			servers := dispatchSetup(t, dedup)
			servers[1].RegisterHandler(mock.TestMethod, func(ctx gorums.ServerContext, in *gorums.Message) (*gorums.Message, error) {
				n1 := dispatchNode(t, ctx.PeerConfig(), 1)
				cctx, cancel := context.WithTimeout(ctx, time.Second)
				defer cancel()
				resp, err := gorumsimpl.RemoteCall[*pb.StringValue, *pb.StringValue](n1.Context(cctx), pb.String("inner"), mock.EchoMethod)
				if err != nil {
					return nil, err
				}
				return gorums.NewResponseMessage(in, pb.String("outer | "+resp.GetValue())), nil
			})
			servers[0].RegisterHandler(mock.EchoMethod, stringEchoHandler("echo"))
			dispatchWait(t, servers)

			n2 := dispatchNode(t, servers[0].PeerConfig(), 2)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			start := time.Now()
			resp, err := gorumsimpl.RemoteCall[*pb.StringValue, *pb.StringValue](n2.Context(ctx), pb.String("x"), mock.TestMethod)
			t.Logf("dedup=%v nested: resp=%q err=%v elapsed=%v", dedup, resp.GetValue(), err, time.Since(start))
			if err != nil {
				t.Errorf("nested call to requester failed: %v", err)
			}
		})
	}
}
