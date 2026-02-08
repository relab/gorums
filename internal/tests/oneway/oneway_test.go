package oneway_test

import (
	context "context"
	"fmt"
	"slices"
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

func (s *onewaySrv) Unicast(_ gorums.ServerCtx, r *oneway.Request) {
	if s.benchmark {
		return
	}
	s.received <- r
	s.wg.Done()
}

func (s *onewaySrv) Multicast(_ gorums.ServerCtx, r *oneway.Request) {
	if s.benchmark {
		return
	}
	s.received <- r
	s.wg.Done()
}

// setupWithNodeMap sets up servers and configuration with sequential node IDs
// (0, 1, 2, ...) matching the server array indices. This is needed for tests like
// TestMulticastPerNode that verify per-node message transformations based on node ID.
func setupWithNodeMap(t testing.TB, cfgSize int) (cfg oneway.Configuration, srvs []*onewaySrv) {
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
			config, srvs := setupWithNodeMap(t, test.servers)
			for i := range srvs {
				srvs[i].wg.Add(test.calls)
			}

			for c := 1; c <= test.calls; c++ {
				in := oneway.Request_builder{Num: uint64(c)}.Build()
				if config.Size() == 1 {
					node := config[0]
					nodeCtx := node.Context(context.Background())
					if test.sendWait {
						if err := oneway.Unicast(nodeCtx, in); err != nil {
							t.Error(err)
						}
					} else {
						if err := oneway.Unicast(nodeCtx, in, gorums.IgnoreErrors()); err != nil {
							t.Error(err)
						}
					}
				} else {
					cfgCtx := config.Context(context.Background())
					if test.sendWait {
						if err := oneway.Multicast(cfgCtx, in); err != nil {
							t.Error(err)
						}
					} else {
						if err := oneway.Multicast(cfgCtx, in, gorums.IgnoreErrors()); err != nil {
							t.Error(err)
						}
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

	// makeIgnoreFunc creates a function that checks if a node ID should be ignored.
	makeIgnoreFunc := func(ignoreNodes []uint32) func(uint32) bool {
		return func(id uint32) bool { return slices.Contains(ignoreNodes, id) }
	}

	// transformation function that uses the MapRequest interceptor
	// to add the msg ID + node ID to the Num field
	makeMapFunc := func(ignore func(uint32) bool) func(*oneway.Request, *oneway.Node) *oneway.Request {
		return func(msg *oneway.Request, node *oneway.Node) *oneway.Request {
			if ignore != nil && ignore(node.ID()) {
				return nil
			}
			return oneway.Request_builder{Num: add(msg.GetNum(), node.ID())}.Build()
		}
	}

	tests := []struct {
		name        string
		calls       int
		servers     int
		sendWait    bool
		ignoreNodes []uint32
	}{
		{name: "MulticastPerNodeNoSendWaiting", calls: numCalls, servers: 1, sendWait: false},
		{name: "MulticastPerNodeNoSendWaiting", calls: numCalls, servers: 3, sendWait: false},
		{name: "MulticastPerNodeNoSendWaiting", calls: numCalls, servers: 9, sendWait: false},
		{name: "MulticastPerNodeSendWaiting", calls: numCalls, servers: 1, sendWait: true},
		{name: "MulticastPerNodeSendWaiting", calls: numCalls, servers: 3, sendWait: true},
		{name: "MulticastPerNodeSendWaiting", calls: numCalls, servers: 9, sendWait: true},
		{name: "MulticastPerNodeNoSendWaitingIgnoreNodes", calls: numCalls, servers: 3, sendWait: false, ignoreNodes: []uint32{0}},
		{name: "MulticastPerNodeNoSendWaitingIgnoreNodes", calls: numCalls, servers: 3, sendWait: false, ignoreNodes: []uint32{1}},
		{name: "MulticastPerNodeNoSendWaitingIgnoreNodes", calls: numCalls, servers: 3, sendWait: false, ignoreNodes: []uint32{0, 1}},
		{name: "MulticastPerNodeNoSendWaitingIgnoreNodes", calls: numCalls, servers: 3, sendWait: false, ignoreNodes: []uint32{0, 1, 2}},
		{name: "MulticastPerNodeSendWaitingIgnoreNodes", calls: numCalls, servers: 3, sendWait: true, ignoreNodes: []uint32{0}},
		{name: "MulticastPerNodeSendWaitingIgnoreNodes", calls: numCalls, servers: 3, sendWait: true, ignoreNodes: []uint32{1}},
		{name: "MulticastPerNodeSendWaitingIgnoreNodes", calls: numCalls, servers: 3, sendWait: true, ignoreNodes: []uint32{0, 1}},
		{name: "MulticastPerNodeSendWaitingIgnoreNodes", calls: numCalls, servers: 3, sendWait: true, ignoreNodes: []uint32{0, 1, 2}},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%s/Servers=%d/IgnoredNodes=%v", test.name, test.servers, test.ignoreNodes), func(t *testing.T) {
			config, srvs := setupWithNodeMap(t, test.servers)
			nodeIDs := config.NodeIDs()
			// create a test-local ignore function to avoid data races between tests
			ignore := makeIgnoreFunc(test.ignoreNodes)
			mapFunc := makeMapFunc(ignore)

			for i := range srvs {
				if ignore(nodeIDs[i]) {
					continue // don't check ignored nodes
				}
				srvs[i].wg.Add(test.calls)
			}

			for c := 1; c <= test.calls; c++ {
				in := oneway.Request_builder{Num: uint64(c)}.Build()
				cfgCtx := config.Context(context.Background())
				mapInterceptor := gorums.MapRequest[*oneway.Request, *emptypb.Empty](mapFunc)
				if test.sendWait {
					if err := oneway.Multicast(cfgCtx, in,
						gorums.Interceptors(mapInterceptor),
					); err != nil {
						t.Error(err)
					}
				} else {
					if err := oneway.Multicast(cfgCtx, in,
						gorums.Interceptors(mapInterceptor),
						gorums.IgnoreErrors(),
					); err != nil {
						t.Error(err)
					}
				}
			}

			// Check that each server received expected oneway messages
			for i := range srvs {
				if ignore(nodeIDs[i]) {
					continue // don't check ignored nodes
				}
				srvs[i].wg.Wait()
				close(srvs[i].received)
				expected := add(uint64(1), nodeIDs[i])
				for r := range srvs[i].received {
					if expected != r.GetNum() {
						t.Errorf("%s -> %d, expected %d, nodeID=%d", test.name, r.GetNum(), expected, nodeIDs[i])
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
			nodeCtx := node.Context(context.Background())
			oneway.Unicast(nodeCtx, in)
		}
	})
	b.Run("UnicastNoSendWaiting", func(b *testing.B) {
		for c := 1; c <= b.N; c++ {
			in.SetNum(uint64(c))
			nodeCtx := node.Context(context.Background())
			oneway.Unicast(nodeCtx, in, gorums.IgnoreErrors())
		}
	})
}

func BenchmarkMulticast(b *testing.B) {
	config, srvs := setupWithNodeMap(b, 3)
	for _, srv := range srvs {
		srv.benchmark = true
	}
	in := oneway.Request_builder{Num: 0}.Build()
	b.Run("MulticastSendWaiting__", func(b *testing.B) {
		for c := 1; c <= b.N; c++ {
			in.SetNum(uint64(c))
			cfgCtx := config.Context(context.Background())
			oneway.Multicast(cfgCtx, in)
		}
	})
	b.Run("MulticastNoSendWaiting", func(b *testing.B) {
		for c := 1; c <= b.N; c++ {
			in.SetNum(uint64(c))
			cfgCtx := config.Context(context.Background())
			oneway.Multicast(cfgCtx, in, gorums.IgnoreErrors())
		}
	})
}
