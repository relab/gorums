package oneway_test

import (
	context "context"
	"fmt"
	"sync"
	"testing"

	"github.com/relab/gorums"
	"github.com/relab/gorums/tests/oneway"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

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

func (s *onewaySrv) MulticastPerNode(_ gorums.ServerCtx, r *oneway.Request) {
	if s.benchmark {
		return
	}
	s.received <- r
	s.wg.Done()
}

func setup(t testing.TB, cfgSize int) (cfg *oneway.OnewayTestConfiguration, srvs []*onewaySrv, teardown func()) {
	t.Helper()
	srvs = make([]*onewaySrv, cfgSize)
	for i := range cfgSize {
		srvs[i] = &onewaySrv{received: make(chan *oneway.Request, numCalls)}
	}

	addrs, closeServers := gorums.TestSetup(t, cfgSize, func(i int) gorums.ServerIface {
		srv := gorums.NewServer()
		oneway.RegisterOnewayTestServer(srv, srvs[i])
		return srv
	})
	nodeMap := make(map[string]uint32)
	for i, addr := range addrs {
		nodeMap[addr] = uint32(i)
	}

	cfg, err := oneway.NewOnewayTestConfiguration(
		gorums.WithNodeMap(nodeMap),
		gorums.WithGrpcDialOptions(
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	teardown = func() {
		cfg.Close()
		closeServers()
	}
	return cfg, srvs, teardown
}

const numCalls = 50

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

	f := func(c *oneway.OnewayTestConfiguration) func(context.Context, *oneway.Request, ...gorums.CallOption) {
		if c.Size() == 1 {
			return c.Nodes()[0].Unicast
		}
		return c.Multicast
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%s/Servers=%d", test.name, test.servers), func(t *testing.T) {
			cfg, srvs, teardown := setup(t, test.servers)
			for i := range srvs {
				srvs[i].wg.Add(test.calls)
			}

			for c := 1; c <= test.calls; c++ {
				in := oneway.Request_builder{Num: uint64(c)}.Build()
				if test.sendWait {
					f(cfg)(context.Background(), in)
				} else {
					f(cfg)(context.Background(), in, gorums.WithNoSendWaiting())
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
			teardown()
		})
	}
}

func TestMulticastPerNode(t *testing.T) {
	add := func(n uint64, id uint32) uint64 { return n + uint64(id) }

	// simple transformation function
	f := func(msg *oneway.Request, id uint32) *oneway.Request {
		return oneway.Request_builder{Num: add(msg.GetNum(), id)}.Build()
	}
	var ignoreNodes []int
	ignore := func(id uint32) bool {
		for _, ignore := range ignoreNodes {
			return id == uint32(ignore)
		}
		return false
	}
	// transformation of all except some nodes
	g := func(msg *oneway.Request, id uint32) *oneway.Request {
		if ignore(id) {
			return nil
		}
		return oneway.Request_builder{Num: add(msg.GetNum(), id)}.Build()
	}
	tests := []struct {
		name        string
		calls       int
		servers     int
		sendWait    bool
		ignoreNodes []int
		f           func(*oneway.Request, uint32) *oneway.Request
	}{
		{name: "MulticastPerNodeNoSendWaiting", calls: numCalls, servers: 1, sendWait: false, f: f},
		{name: "MulticastPerNodeNoSendWaiting", calls: numCalls, servers: 3, sendWait: false, f: f},
		{name: "MulticastPerNodeNoSendWaiting", calls: numCalls, servers: 9, sendWait: false, f: f},
		{name: "MulticastPerNodeSendWaiting", calls: numCalls, servers: 1, sendWait: true, f: f},
		{name: "MulticastPerNodeSendWaiting", calls: numCalls, servers: 3, sendWait: true, f: f},
		{name: "MulticastPerNodeSendWaiting", calls: numCalls, servers: 9, sendWait: true, f: f},
		{name: "MulticastPerNodeNoSendWaitingIgnoreNodes", calls: numCalls, servers: 3, sendWait: false, f: g, ignoreNodes: []int{0}},
		{name: "MulticastPerNodeNoSendWaitingIgnoreNodes", calls: numCalls, servers: 3, sendWait: false, f: g, ignoreNodes: []int{1}},
		{name: "MulticastPerNodeNoSendWaitingIgnoreNodes", calls: numCalls, servers: 3, sendWait: false, f: g, ignoreNodes: []int{0, 1}},
		{name: "MulticastPerNodeNoSendWaitingIgnoreNodes", calls: numCalls, servers: 3, sendWait: false, f: g, ignoreNodes: []int{0, 1, 2}},
		{name: "MulticastPerNodeSendWaitingIgnoreNodes", calls: numCalls, servers: 3, sendWait: true, f: g, ignoreNodes: []int{0}},
		{name: "MulticastPerNodeSendWaitingIgnoreNodes", calls: numCalls, servers: 3, sendWait: true, f: g, ignoreNodes: []int{1}},
		{name: "MulticastPerNodeSendWaitingIgnoreNodes", calls: numCalls, servers: 3, sendWait: true, f: g, ignoreNodes: []int{0, 1}},
		{name: "MulticastPerNodeSendWaitingIgnoreNodes", calls: numCalls, servers: 3, sendWait: true, f: g, ignoreNodes: []int{0, 1, 2}},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%s/Servers=%d/IgnoredNodes=%v", test.name, test.servers, test.ignoreNodes), func(t *testing.T) {
			cfg, srvs, teardown := setup(t, test.servers)
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
				if test.sendWait {
					cfg.MulticastPerNode(context.Background(), in, test.f)
				} else {
					cfg.MulticastPerNode(context.Background(), in, test.f, gorums.WithNoSendWaiting())
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
			teardown()
		})
	}
}

func BenchmarkUnicast(b *testing.B) {
	cfg, srvs, teardown := setup(b, 1)
	for _, srv := range srvs {
		srv.benchmark = true
	}
	node := cfg.Nodes()[0]
	in := oneway.Request_builder{Num: 0}.Build()
	b.Run("UnicastSendWaiting__", func(b *testing.B) {
		for c := 1; c <= b.N; c++ {
			in.SetNum(uint64(c))
			node.Unicast(context.Background(), in)
		}
	})
	b.Run("UnicastNoSendWaiting", func(b *testing.B) {
		for c := 1; c <= b.N; c++ {
			in.SetNum(uint64(c))
			node.Unicast(context.Background(), in, gorums.WithNoSendWaiting())
		}
	})
	teardown()
}

func BenchmarkMulticast(b *testing.B) {
	cfg, srvs, teardown := setup(b, 3)
	for _, srv := range srvs {
		srv.benchmark = true
	}
	in := oneway.Request_builder{Num: 0}.Build()
	b.Run("MulticastSendWaiting__", func(b *testing.B) {
		for c := 1; c <= b.N; c++ {
			in.SetNum(uint64(c))
			cfg.Multicast(context.Background(), in)
		}
	})
	b.Run("MulticastNoSendWaiting", func(b *testing.B) {
		for c := 1; c <= b.N; c++ {
			in.SetNum(uint64(c))
			cfg.Multicast(context.Background(), in, gorums.WithNoSendWaiting())
		}
	})
	teardown()
}
