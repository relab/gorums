package scatter_test

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/gorumstest"
	"github.com/relab/gorums/internal/tests/scatter"
)

// timeout bounds each call and each wait for a server to receive a request.
const timeout = 5 * time.Second

// scatterSrv records the value of every request it receives, from any of its
// handlers, and echoes the value in its replies.
type scatterSrv struct {
	received chan string
}

func (s *scatterSrv) Write(_ gorums.ServerContext, req *scatter.Request) (*scatter.Response, error) {
	s.received <- req.GetValue()
	return scatter.Response_builder{Value: req.GetValue()}.Build(), nil
}

func (s *scatterSrv) WriteStream(_ gorums.ServerContext, req *scatter.Request, send func(*scatter.Response)) {
	s.received <- req.GetValue()
	send(scatter.Response_builder{Value: req.GetValue()}.Build())
}

func (s *scatterSrv) Send(_ gorums.ServerContext, req *scatter.Request) {
	s.received <- req.GetValue()
}

// cluster is a configuration of servers with IDs 5, 3, and 1, in that order,
// drawn from five servers with IDs 1 to 5. Its node IDs are neither sorted nor
// consecutive, so a scatter that paired by ID or by position in ID order would
// send to the wrong node.
type cluster struct {
	cfg  scatter.Config
	srvs map[gorums.ID]*scatterSrv
	stop func(...int)
}

func newCluster(t *testing.T) *cluster {
	t.Helper()
	const numServers = 5
	c := &cluster{srvs: make(map[gorums.ID]*scatterSrv)}
	srvs := make([]*scatterSrv, numServers)
	for i := range srvs {
		srvs[i] = &scatterSrv{received: make(chan string, 16)}
	}
	full := gorumstest.Config(t, numServers, func(i int) gorumstest.ServerIface {
		srv := gorums.NewServer()
		scatter.RegisterScatterTestServer(srv, srvs[i])
		return srv
	}, gorumstest.WithStopFunc(t, &c.stop))
	c.cfg = full.Remove(2, 4).Sort(func(a, b *gorums.Node) int { return cmp.Compare(b.ID(), a.ID()) })
	if got, want := c.cfg.NodeIDs(), []gorums.ID{5, 3, 1}; !slices.Equal(got, want) {
		t.Fatalf("config node IDs = %v, want %v", got, want)
	}
	for _, id := range c.cfg.NodeIDs() {
		c.srvs[id] = srvs[id-1] // gorumstest.Config gives server i the ID i+1.
	}
	return c
}

// requests returns one request per node of the configuration: "<prefix>-<id>"
// for the i-th node.
func (c *cluster) requests(prefix string) []*scatter.Request {
	reqs := make([]*scatter.Request, c.cfg.Size())
	for i, node := range c.cfg {
		reqs[i] = scatter.Request_builder{Value: fmt.Sprintf("%s-%d", prefix, node.ID())}.Build()
	}
	return reqs
}

// next returns the next value that the server with the given ID received.
func (c *cluster) next(t *testing.T, id gorums.ID) string {
	t.Helper()
	got := gorumstest.Collect(t, timeout, 1, c.srvs[id].received)
	if len(got) != 1 {
		t.Fatalf("server %d received nothing within %v", id, timeout)
	}
	return got[0]
}

// replies collects the first reply of each node of a call, keyed by node ID.
func replies(t *testing.T, seq gorums.ResponseSeq[*scatter.Response], n int) map[gorums.ID]string {
	t.Helper()
	got := make(map[gorums.ID]string)
	for resp := range seq {
		if resp.Err != nil {
			t.Fatalf("node %d: %v", resp.NodeID, resp.Err)
		}
		got[resp.NodeID] = resp.Value.GetValue()
		if len(got) == n {
			break
		}
	}
	return got
}

// TestScatterPairing verifies that each scatter method sends the i-th request
// to the i-th node of the configuration, and to no other node.
func TestScatterPairing(t *testing.T) {
	c := newCluster(t)
	client := scatter.NewScatterTestClient(c.cfg)
	want := func(prefix string) map[gorums.ID]string {
		m := make(map[gorums.ID]string)
		for _, id := range c.cfg.NodeIDs() {
			m[id] = fmt.Sprintf("%s-%d", prefix, id)
		}
		return m
	}
	checkReceived := func(t *testing.T, prefix string) {
		t.Helper()
		for id, value := range want(prefix) {
			if got := c.next(t, id); got != value {
				t.Errorf("server %d received %q, want %q", id, got, value)
			}
		}
	}

	t.Run("QuorumCall", func(t *testing.T) {
		ctx := gorumstest.Context(t, timeout)
		got := replies(t, client.WriteScatter(ctx, c.requests("write")).Results(), c.cfg.Size())
		if w := want("write"); !maps.Equal(got, w) {
			t.Errorf("replies = %v, want %v", got, w)
		}
		checkReceived(t, "write")
	})
	t.Run("QuorumCallStream", func(t *testing.T) {
		ctx := gorumstest.Context(t, timeout)
		got := replies(t, client.WriteStreamScatter(ctx, c.requests("stream")).Results(), c.cfg.Size())
		if w := want("stream"); !maps.Equal(got, w) {
			t.Errorf("replies = %v, want %v", got, w)
		}
		checkReceived(t, "stream")
	})
	t.Run("Multicast", func(t *testing.T) {
		ctx := gorumstest.Context(t, timeout)
		if err := client.SendScatter(ctx, c.requests("send")).Async().Wait(); err != nil {
			t.Fatalf("SendScatter: %v", err)
		}
		checkReceived(t, "send")
	})
}

// TestScatterCountMismatch verifies that a scatter call with too few or too
// many requests fails with [gorums.ErrScatterCount] and sends nothing. Each
// stream delivers requests in order, so if a valid scatter sent afterwards is
// the first request that every server receives, the failed calls sent nothing.
func TestScatterCountMismatch(t *testing.T) {
	c := newCluster(t)
	client := scatter.NewScatterTestClient(c.cfg)
	calls := []struct {
		name string
		call func(context.Context, []*scatter.Request) error
	}{
		{name: "QuorumCall", call: func(ctx context.Context, in []*scatter.Request) error {
			_, err := client.WriteScatter(ctx, in).All()
			return err
		}},
		{name: "QuorumCallStream", call: func(ctx context.Context, in []*scatter.Request) error {
			_, err := client.WriteStreamScatter(ctx, in).Majority()
			return err
		}},
		{name: "Multicast/Send", call: func(ctx context.Context, in []*scatter.Request) error {
			return client.SendScatter(ctx, in).Send()
		}},
		{name: "Multicast/Async", call: func(ctx context.Context, in []*scatter.Request) error {
			return client.SendScatter(ctx, in).Async().Wait()
		}},
	}
	reqs := c.requests("mismatch")
	for _, call := range calls {
		for _, in := range [][]*scatter.Request{reqs[:2], append(slices.Clone(reqs), reqs[0])} {
			t.Run(fmt.Sprintf("%s/Requests=%d", call.name, len(in)), func(t *testing.T) {
				if err := call.call(gorumstest.Context(t, timeout), in); !errors.Is(err, gorums.ErrScatterCount) {
					t.Errorf("call error = %v, want %v", err, gorums.ErrScatterCount)
				}
			})
		}
	}
	if err := client.SendScatter(gorumstest.Context(t, timeout), c.requests("valid")).Send(); err != nil {
		t.Fatalf("SendScatter: %v", err)
	}
	for _, id := range c.cfg.NodeIDs() {
		if got, want := c.next(t, id), fmt.Sprintf("valid-%d", id); got != want {
			t.Errorf("server %d first received %q, want %q", id, got, want)
		}
	}
}

// TestScatterFailedNode verifies that the error of a scatter call to a
// stopped server is attributed to that server's node, while the other nodes
// receive their own requests.
func TestScatterFailedNode(t *testing.T) {
	const failed = 3
	c := newCluster(t)
	client := scatter.NewScatterTestClient(c.cfg)
	c.stop(failed - 1) // server i has ID i+1
	checkErr := func(t *testing.T, err error) {
		t.Helper()
		qcErr, ok := errors.AsType[gorums.QuorumCallError](err)
		if !ok {
			t.Fatalf("call error = %v, want a QuorumCallError", err)
		}
		var ids []gorums.ID
		for id := range qcErr.NodeErrors() {
			ids = append(ids, id)
		}
		if want := []gorums.ID{failed}; !slices.Equal(ids, want) {
			t.Errorf("failed nodes = %v, want %v (%v)", ids, want, err)
		}
	}

	t.Run("QuorumCall", func(t *testing.T) {
		_, err := client.WriteScatter(gorumstest.Context(t, timeout), c.requests("write")).All()
		checkErr(t, err)
		for _, id := range []gorums.ID{5, 1} {
			if got, want := c.next(t, id), fmt.Sprintf("write-%d", id); got != want {
				t.Errorf("server %d received %q, want %q", id, got, want)
			}
		}
	})
	t.Run("Multicast", func(t *testing.T) {
		err := client.SendScatter(gorumstest.Context(t, timeout), c.requests("send")).Async().Wait()
		checkErr(t, err)
		for _, id := range []gorums.ID{5, 1} {
			if got, want := c.next(t, id), fmt.Sprintf("send-%d", id); got != want {
				t.Errorf("server %d received %q, want %q", id, got, want)
			}
		}
	})
}

// TestScatterBroadcast verifies that the method of a scatter RPC that takes
// one request still sends that request to every node, through the same server
// handler as the scatter method.
func TestScatterBroadcast(t *testing.T) {
	c := newCluster(t)
	client := scatter.NewScatterTestClient(c.cfg)
	req := scatter.Request_builder{Value: "all"}.Build()
	if err := client.Send(gorumstest.Context(t, timeout), req).Async().Wait(); err != nil {
		t.Fatalf("Send: %v", err)
	}
	for _, id := range c.cfg.NodeIDs() {
		if got := c.next(t, id); got != "all" {
			t.Errorf("server %d received %q, want %q", id, got, "all")
		}
	}
}

// TestScatterSkipNil verifies that a nil request skips its node: the node
// receives nothing, the call reports no error for it, and the other nodes
// receive their own requests.
func TestScatterSkipNil(t *testing.T) {
	const skipped = 3 // the second node of the configuration
	c := newCluster(t)
	client := scatter.NewScatterTestClient(c.cfg)
	withNil := func(prefix string) []*scatter.Request {
		reqs := c.requests(prefix)
		reqs[1] = nil
		return reqs
	}

	t.Run("QuorumCall", func(t *testing.T) {
		got := make(map[gorums.ID]string)
		for resp := range client.WriteScatter(gorumstest.Context(t, timeout), withNil("write")).Results() {
			switch {
			case resp.NodeID == skipped && !errors.Is(resp.Err, gorums.ErrSkipNode):
				t.Errorf("node %d error = %v, want %v", skipped, resp.Err, gorums.ErrSkipNode)
			case resp.NodeID != skipped && resp.Err != nil:
				t.Errorf("node %d error = %v, want nil", resp.NodeID, resp.Err)
			case resp.Err == nil:
				got[resp.NodeID] = resp.Value.GetValue()
			}
		}
		if want := map[gorums.ID]string{5: "write-5", 1: "write-1"}; !maps.Equal(got, want) {
			t.Errorf("replies = %v, want %v", got, want)
		}
	})
	t.Run("Multicast", func(t *testing.T) {
		if err := client.SendScatter(gorumstest.Context(t, timeout), withNil("send")).Async().Wait(); err != nil {
			t.Fatalf("SendScatter: %v", err)
		}
	})
	for _, id := range []gorums.ID{5, 1} {
		for _, prefix := range []string{"write", "send"} {
			if got, want := c.next(t, id), fmt.Sprintf("%s-%d", prefix, id); got != want {
				t.Errorf("server %d received %q, want %q", id, got, want)
			}
		}
	}
	// Each stream delivers requests in order, so if a later request is the
	// first that the skipped server receives, both calls sent it nothing.
	if err := client.SendScatter(gorumstest.Context(t, timeout), c.requests("valid")).Send(); err != nil {
		t.Fatalf("SendScatter: %v", err)
	}
	if got, want := c.next(t, skipped), fmt.Sprintf("valid-%d", skipped); got != want {
		t.Errorf("skipped server %d first received %q, want %q", skipped, got, want)
	}
}

// TestScatterMapRequest verifies that a MapRequest interceptor on a scatter
// call receives the request of its own node, and that each node receives the
// transformed request.
func TestScatterMapRequest(t *testing.T) {
	c := newCluster(t)
	client := scatter.NewScatterTestClient(c.cfg)
	var (
		mu   sync.Mutex
		seen = make(map[gorums.ID]string)
	)
	mark := gorums.MapRequest[*scatter.Request, *scatter.Response](func(req *scatter.Request, node *gorums.Node) *scatter.Request {
		mu.Lock()
		seen[node.ID()] = req.GetValue()
		mu.Unlock()
		return scatter.Request_builder{Value: req.GetValue() + "!"}.Build()
	})
	ctx := gorumstest.Context(t, timeout)
	got := replies(t, client.WriteScatter(ctx, c.requests("write")).Intercept(mark).Results(), c.cfg.Size())
	for _, id := range c.cfg.NodeIDs() {
		value := fmt.Sprintf("write-%d", id)
		if seen[id] != value {
			t.Errorf("MapRequest for node %d received %q, want %q", id, seen[id], value)
		}
		if got[id] != value+"!" {
			t.Errorf("node %d replied %q, want %q", id, got[id], value+"!")
		}
	}
}
