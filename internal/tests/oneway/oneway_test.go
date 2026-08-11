package oneway_test

import (
	context "context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/gorumstest"
	"github.com/relab/gorums/internal/tests/oneway"
	"google.golang.org/protobuf/types/known/emptypb"
)

const numCalls = 50

// recvTimeout bounds how long a subtest waits for the messages it sent. A send
// that succeeded still leaves the server free to drop or delay the message, so
// an unbounded wait would hang the package until the test binary's timeout
// instead of reporting the shortfall.
const recvTimeout = 10 * time.Second

type onewaySrv struct {
	benchmark bool
	// received buffers the messages of one subtest, with headroom so that a
	// straggler arriving after a failed subtest cannot block a handler.
	received chan *oneway.Request
}

func (s *onewaySrv) Unicast(_ gorums.ServerContext, r *oneway.Request) {
	if s.benchmark {
		return
	}
	s.received <- r
}

func (s *onewaySrv) Multicast(_ gorums.ServerContext, r *oneway.Request) {
	if s.benchmark {
		return
	}
	s.received <- r
}

// cluster is a set of servers and the configuration addressing them, shared by
// every subtest of a table that needs that many servers.
type cluster struct {
	cfg  oneway.Config
	srvs []*onewaySrv
}

// reset discards messages left over from an earlier subtest so the next one
// starts from a known state. A subtest that received everything it sent leaves
// nothing behind.
func (c *cluster) reset() {
	for _, srv := range c.srvs {
		for range len(srv.received) {
			<-srv.received
		}
	}
}

// received returns the messages that the given server received, sorted by
// their Num field. Sorting avoids flakiness from multicast reordering. If
// fewer than want messages arrive within [recvTimeout] it reports the
// shortfall and returns nil, since every later message then compares against
// the wrong expected value; a dropped one-way message surfaces this way.
func (c *cluster) received(t *testing.T, i, want int) []uint64 {
	t.Helper()
	got := gorumstest.Collect(t, recvTimeout, want, c.srvs[i].received)
	if len(got) != want {
		t.Errorf("server %d received %d messages, expected %d", i, len(got), want)
		return nil
	}
	nums := make([]uint64, len(got))
	for j, r := range got {
		nums[j] = r.GetNum()
	}
	slices.Sort(nums)
	return nums
}

// clusters returns a lookup that lazily creates one shared cluster per
// configuration size and resets it before each use. Sharing clusters across
// the subtests of a table keeps the number of connections proportional to the
// distinct sizes rather than to the number of subtests: with real TCP
// listeners, every subtest would otherwise leave one socket per node in
// TIME_WAIT, and a high -count run can exhaust the ephemeral port range.
//
// The lookup must be called from the goroutine running t, not from a subtest,
// since it registers servers and cleanup on t. Only the first cluster
// registers a goroutine leak check: cleanup functions run in reverse
// registration order, so that check runs after every cluster has been torn
// down, whereas a check registered by a later cluster would run while earlier
// clusters are still serving.
func clusters(t *testing.T) func(cfgSize int) *cluster {
	cache := make(map[int]*cluster)
	return func(cfgSize int) *cluster {
		t.Helper()
		if c, ok := cache[cfgSize]; ok {
			c.reset()
			return c
		}
		var opts []gorumstest.Option
		if len(cache) > 0 {
			opts = append(opts, gorumstest.SkipGoleak())
		}
		cfg, srvs := setupWithNodeMap(t, cfgSize, opts...)
		c := &cluster{cfg: cfg, srvs: srvs}
		cache[cfgSize] = c
		return c
	}
}

// setupWithNodeMap sets up servers and configuration with sequential node IDs
// (1, 2, 3, ...) matching the server array indices. This is needed for tests like
// TestMulticastPerNode that verify per-node message transformations based on node ID.
func setupWithNodeMap(t testing.TB, cfgSize int, opts ...gorumstest.Option) (cfg oneway.Config, srvs []*onewaySrv) {
	t.Helper()
	srvs = make([]*onewaySrv, cfgSize)
	for i := range cfgSize {
		srvs[i] = &onewaySrv{received: make(chan *oneway.Request, 2*numCalls)}
	}
	cfg = gorumstest.Config(t, cfgSize, func(i int) gorums.ServerIface {
		srv := gorums.NewServer()
		oneway.RegisterOnewayTestServer(srv, srvs[i])
		return srv
	}, opts...)
	return cfg, srvs
}

func TestOnewayCalls(t *testing.T) {
	tests := []struct {
		name    string
		calls   int
		servers int
		unicast bool
	}{
		{name: "Unicast__", calls: numCalls, servers: 1, unicast: true},
		{name: "Multicast", calls: numCalls, servers: 1},
		{name: "Multicast", calls: numCalls, servers: 3},
		{name: "Multicast", calls: numCalls, servers: 9},
	}
	newCluster := clusters(t)
	for _, test := range tests {
		c := newCluster(test.servers)
		t.Run(fmt.Sprintf("%s/Servers=%d", test.name, test.servers), func(t *testing.T) {
			for i := 1; i <= test.calls; i++ {
				in := oneway.Request_builder{Num: uint64(i)}.Build()
				var err error
				if test.unicast {
					err = oneway.Unicast(c.cfg[0].Context(context.Background()), in).Send()
				} else {
					err = oneway.Multicast(c.cfg.Context(context.Background()), in).Send()
				}
				if err != nil {
					t.Error(err)
				}
			}

			// Check that each server received expected oneway messages
			for i := range c.srvs {
				for j, got := range c.received(t, i, test.calls) {
					want := uint64(j + 1)
					if want != got {
						t.Errorf("%s: received[%d] = %d, expected %d", test.name, j, got, want)
					}
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
		ignoreNodes []uint32
	}{
		{name: "MulticastPerNode", calls: numCalls, servers: 1},
		{name: "MulticastPerNode", calls: numCalls, servers: 3},
		{name: "MulticastPerNode", calls: numCalls, servers: 9},
		{name: "MulticastPerNodeIgnoreNodes", calls: numCalls, servers: 3, ignoreNodes: []uint32{0}},
		{name: "MulticastPerNodeIgnoreNodes", calls: numCalls, servers: 3, ignoreNodes: []uint32{1}},
		{name: "MulticastPerNodeIgnoreNodes", calls: numCalls, servers: 3, ignoreNodes: []uint32{0, 1}},
		{name: "MulticastPerNodeIgnoreNodes", calls: numCalls, servers: 3, ignoreNodes: []uint32{0, 1, 2}},
	}
	newCluster := clusters(t)
	for _, test := range tests {
		c := newCluster(test.servers)
		t.Run(fmt.Sprintf("%s/Servers=%d/IgnoredNodes=%v", test.name, test.servers, test.ignoreNodes), func(t *testing.T) {
			nodeIDs := c.cfg.NodeIDs()
			// create a test-local ignore function to avoid data races between tests
			ignore := makeIgnoreFunc(test.ignoreNodes)
			mapFunc := makeMapFunc(ignore)

			for i := 1; i <= test.calls; i++ {
				in := oneway.Request_builder{Num: uint64(i)}.Build()
				cfgCtx := c.cfg.Context(context.Background())
				mapInterceptor := gorums.MapRequest[*oneway.Request, *emptypb.Empty](mapFunc)
				if err := oneway.Multicast(cfgCtx, in).Intercept(mapInterceptor).Send(); err != nil {
					t.Error(err)
				}
			}

			// Check that each server received expected oneway messages
			for i := range c.srvs {
				if ignore(nodeIDs[i]) {
					continue // don't check ignored nodes
				}
				for j, got := range c.received(t, i, test.calls) {
					want := add(uint64(j+1), nodeIDs[i])
					if want != got {
						t.Errorf("%s: received[%d] = %d, expected %d, nodeID=%d", test.name, j, got, want, nodeIDs[i])
					}
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
	for c := 1; c <= b.N; c++ {
		in.SetNum(uint64(c))
		nodeCtx := node.Context(context.Background())
		if err := oneway.Unicast(nodeCtx, in).Send(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMulticast(b *testing.B) {
	config, srvs := setupWithNodeMap(b, 3)
	for _, srv := range srvs {
		srv.benchmark = true
	}
	in := oneway.Request_builder{Num: 0}.Build()
	for c := 1; c <= b.N; c++ {
		in.SetNum(uint64(c))
		cfgCtx := config.Context(context.Background())
		if err := oneway.Multicast(cfgCtx, in).Send(); err != nil {
			b.Fatal(err)
		}
	}
}
