package oneway_test

import (
	context "context"
	"fmt"
	"sync"
	"testing"

	"github.com/relab/gorums"
	"github.com/relab/gorums/internal/tests/oneway"
	"google.golang.org/protobuf/types/known/emptypb"
)

const numCalls = 50

type onewaySrv struct {
	benchmark bool
	wg        sync.WaitGroup
	received  chan *oneway.Request
}

func (s *onewaySrv) Unicast(ctx gorums.ServerCtx, r *oneway.Request) {
	if s.benchmark {
		return
	}
	s.received <- r
	s.wg.Done()
}

func (s *onewaySrv) Multicast(ctx gorums.ServerCtx, r *oneway.Request) {
	if s.benchmark {
		return
	}
	s.received <- r
	s.wg.Done()
}

// setupWithNodeMap sets up servers and configuration with sequential node IDs
// (0, 1, 2, ...) matching the server array indices. This is needed for tests like
// TestMulticastPerNode that verify per-node message transformations based on node ID.
func setupWithNodeMap(t testing.TB, cfgSize int) (cfg gorums.Configuration, srvs []*onewaySrv) {
	t.Helper()
	srvs = make([]*onewaySrv, cfgSize)
	for i := range cfgSize {
		srvs[i] = &onewaySrv{received: make(chan *oneway.Request, numCalls)}
	}

	cfg = gorums.TestConfiguration(t, cfgSize, func(i int) gorums.ServerIface {
		srv := gorums.NewServer()
		oneway.RegisterOnewayTestServer(srv, srvs[i])
		return srv
	})
	return cfg, srvs
}

func TestOnewayCalls(t *testing.T) {
	tests := []struct {
		name     string
		calls    int
		servers  int
		sendWait bool
	}{
		{name: "UnicastSendWaiting____", calls: numCalls, servers: 1, sendWait: true},
		{name: "UnicastNoSendWaiting__", calls: numCalls, servers: 1, sendWait: false},
		{name: "MulticastSendWaiting__", calls: numCalls, servers: 1, sendWait: true},
		{name: "MulticastNoSendWaiting", calls: numCalls, servers: 1, sendWait: false},
		{name: "MulticastSendWaiting__", calls: numCalls, servers: 3, sendWait: true},
		{name: "MulticastNoSendWaiting", calls: numCalls, servers: 3, sendWait: false},
		{name: "MulticastSendWaiting__", calls: numCalls, servers: 9, sendWait: true},
		{name: "MulticastNoSendWaiting", calls: numCalls, servers: 9, sendWait: false},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%s/Servers=%d", test.name, test.servers), func(t *testing.T) {
			cfg, srvs := setupWithNodeMap(t, test.servers)
			for i := range srvs {
				srvs[i].wg.Add(test.calls)
			}

			for c := 1; c <= test.calls; c++ {
				in := oneway.Request_builder{Num: uint64(c)}.Build()
				if cfg.Size() == 1 {
					node := cfg[0]
					nodeCtx := gorums.WithNodeContext(context.Background(), node)
					if test.sendWait {
						oneway.Unicast(nodeCtx, in)
					} else {
						oneway.Unicast(nodeCtx, in, gorums.WithNoSendWaiting())
					}
				} else {
					cfgCtx := gorums.WithConfigContext(context.Background(), cfg)
					if test.sendWait {
						oneway.Multicast(cfgCtx, in)
					} else {
						oneway.Multicast(cfgCtx, in, gorums.WithNoSendWaiting())
					}
				}
			}

			// Check that each server received expected oneway messages
			for i := range srvs {
				srvs[i].wg.Wait()
				close(srvs[i].received)
				expected := uint64(1)
				for r := range srvs[i].received {
					if expected != r.GetNum() {
						t.Errorf("%s(%d) = %d, expected %d", test.name, expected, r.GetNum(), expected)
					}
					expected++
				}
			}
		})
	}
}

func TestMulticastPerNode(t *testing.T) {
	add := func(n uint64, id uint32) uint64 { return n + uint64(id) }

	// transformation function that uses the MapRequest interceptor
	// to add the msg ID + node ID to the Num field
	f := func(msg *oneway.Request, node *gorums.Node) *oneway.Request {
		return oneway.Request_builder{Num: add(msg.GetNum(), node.ID())}.Build()
	}
	ignoreNodes := []int{}
	ignore := func(id uint32) bool {
		for _, ignore := range ignoreNodes {
			return id == uint32(ignore)
		}
		return false
	}
	// transformation for all except some nodes that are ignored
	g := func(msg *oneway.Request, node *gorums.Node) *oneway.Request {
		if ignore(node.ID()) {
			return nil
		}
		return oneway.Request_builder{Num: add(msg.GetNum(), node.ID())}.Build()
	}
	tests := []struct {
		name        string
		calls       int
		servers     int
		sendWait    bool
		ignoreNodes []int
		mapFunc     func(*oneway.Request, *gorums.Node) *oneway.Request
	}{
		{name: "MulticastPerNodeNoSendWaiting", calls: numCalls, servers: 1, sendWait: false, mapFunc: f},
		{name: "MulticastPerNodeNoSendWaiting", calls: numCalls, servers: 3, sendWait: false, mapFunc: f},
		{name: "MulticastPerNodeNoSendWaiting", calls: numCalls, servers: 9, sendWait: false, mapFunc: f},
		{name: "MulticastPerNodeSendWaiting", calls: numCalls, servers: 1, sendWait: true, mapFunc: f},
		{name: "MulticastPerNodeSendWaiting", calls: numCalls, servers: 3, sendWait: true, mapFunc: f},
		{name: "MulticastPerNodeSendWaiting", calls: numCalls, servers: 9, sendWait: true, mapFunc: f},
		{name: "MulticastPerNodeNoSendWaitingIgnoreNodes", calls: numCalls, servers: 3, sendWait: false, mapFunc: g, ignoreNodes: []int{0}},
		{name: "MulticastPerNodeNoSendWaitingIgnoreNodes", calls: numCalls, servers: 3, sendWait: false, mapFunc: g, ignoreNodes: []int{1}},
		{name: "MulticastPerNodeNoSendWaitingIgnoreNodes", calls: numCalls, servers: 3, sendWait: false, mapFunc: g, ignoreNodes: []int{0, 1}},
		{name: "MulticastPerNodeNoSendWaitingIgnoreNodes", calls: numCalls, servers: 3, sendWait: false, mapFunc: g, ignoreNodes: []int{0, 1, 2}},
		{name: "MulticastPerNodeSendWaitingIgnoreNodes", calls: numCalls, servers: 3, sendWait: true, mapFunc: g, ignoreNodes: []int{0}},
		{name: "MulticastPerNodeSendWaitingIgnoreNodes", calls: numCalls, servers: 3, sendWait: true, mapFunc: g, ignoreNodes: []int{1}},
		{name: "MulticastPerNodeSendWaitingIgnoreNodes", calls: numCalls, servers: 3, sendWait: true, mapFunc: g, ignoreNodes: []int{0, 1}},
		{name: "MulticastPerNodeSendWaitingIgnoreNodes", calls: numCalls, servers: 3, sendWait: true, mapFunc: g, ignoreNodes: []int{0, 1, 2}},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%s/Servers=%d/IgnoredNodes=%v", test.name, test.servers, test.ignoreNodes), func(t *testing.T) {
			cfg, srvs := setupWithNodeMap(t, test.servers)
			nodeIDs := cfg.NodeIDs()
			// set the ignoreNodes variable used by the ignore() function
			ignoreNodes = test.ignoreNodes

			for i := range srvs {
				if ignore(nodeIDs[i]) {
					continue // don't check ignored nodes
				}
				srvs[i].wg.Add(test.calls)
			}

			for c := 1; c <= test.calls; c++ {
				in := oneway.Request_builder{Num: uint64(c)}.Build()
				cfgCtx := gorums.WithConfigContext(context.Background(), cfg)
				if test.sendWait {
					oneway.Multicast(cfgCtx, in, gorums.Interceptors(gorums.MapRequest[*oneway.Request, *emptypb.Empty](test.mapFunc)))
				} else {
					oneway.Multicast(cfgCtx, in, gorums.Interceptors(gorums.MapRequest[*oneway.Request, *emptypb.Empty](test.mapFunc)), gorums.WithNoSendWaiting())
				}
			}

			// Check that each server received expected oneway messages
			for i := range srvs {
				if ignore(nodeIDs[i]) {
					continue // don't check ignored nodes
				}
				expected := add(uint64(1), nodeIDs[i])
				srvs[i].wg.Wait()
				close(srvs[i].received)
				for r := range srvs[i].received {
					if expected != r.GetNum() {
						t.Errorf("%s -> %d, expected %d, nodeID=%d, ignore=%t", test.name, r.GetNum(), expected, nodeIDs[i], ignore(nodeIDs[i]))
					}
					expected++
				}
			}
		})
	}
}

func BenchmarkUnicast(b *testing.B) {
	cfg, srvs := setupWithNodeMap(b, 1)
	for _, srv := range srvs {
		srv.benchmark = true
	}
	node := cfg[0]
	in := oneway.Request_builder{Num: 0}.Build()
	b.Run("UnicastSendWaiting__", func(b *testing.B) {
		for c := 1; c <= b.N; c++ {
			in.SetNum(uint64(c))
			nodeCtx := gorums.WithNodeContext(context.Background(), node)
			oneway.Unicast(nodeCtx, in)
		}
	})
	b.Run("UnicastNoSendWaiting", func(b *testing.B) {
		for c := 1; c <= b.N; c++ {
			in.SetNum(uint64(c))
			nodeCtx := gorums.WithNodeContext(context.Background(), node)
			oneway.Unicast(nodeCtx, in, gorums.WithNoSendWaiting())
		}
	})
}

func BenchmarkMulticast(b *testing.B) {
	cfg, srvs := setupWithNodeMap(b, 3)
	for _, srv := range srvs {
		srv.benchmark = true
	}
	in := oneway.Request_builder{Num: 0}.Build()
	b.Run("MulticastSendWaiting__", func(b *testing.B) {
		for c := 1; c <= b.N; c++ {
			in.SetNum(uint64(c))
			cfgCtx := gorums.WithConfigContext(context.Background(), cfg)
			oneway.Multicast(cfgCtx, in)
		}
	})
	b.Run("MulticastNoSendWaiting", func(b *testing.B) {
		for c := 1; c <= b.N; c++ {
			in.SetNum(uint64(c))
			cfgCtx := gorums.WithConfigContext(context.Background(), cfg)
			oneway.Multicast(cfgCtx, in, gorums.WithNoSendWaiting())
		}
	})
}
