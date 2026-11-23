package impl

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/relab/gorums/internal/conn"
	"github.com/relab/gorums/internal/stream"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

// TestCallEmptyConfig verifies that a multi-node call on a nil or empty
// configuration does not panic, but reports [ErrEmptyConfig] from every
// terminal method, from Wait after Async, and from Send.
func TestCallEmptyConfig(t *testing.T) {
	const method = "test.Method"
	req := pb.String("hello")
	type qcFunc func(context.Context, Config, *pb.StringValue, string) *Call[*pb.StringValue, *pb.StringValue]
	quorumCalls := []struct {
		name string
		call qcFunc
	}{
		{name: "QuorumCall", call: QuorumCall},
		{name: "QuorumCallStream", call: QuorumCallStream},
	}
	firstErr := func(seq ResponseSeq[*pb.StringValue]) (int, error) {
		var (
			n   int
			err error
		)
		for r := range seq {
			if n == 0 {
				err = r.Err
			}
			n++
		}
		if n != 1 {
			t.Errorf("sequence yielded %d responses, want 1", n)
		}
		return n, err
	}
	terminals := []struct {
		name string
		call func(*Call[*pb.StringValue, *pb.StringValue]) error
	}{
		{name: "First", call: func(c *Call[*pb.StringValue, *pb.StringValue]) error { _, err := c.First(); return err }},
		{name: "Majority", call: func(c *Call[*pb.StringValue, *pb.StringValue]) error { _, err := c.Majority(); return err }},
		{name: "All", call: func(c *Call[*pb.StringValue, *pb.StringValue]) error { _, err := c.All(); return err }},
		{name: "Threshold", call: func(c *Call[*pb.StringValue, *pb.StringValue]) error { _, err := c.Threshold(2); return err }},
		{name: "Reduce", call: func(c *Call[*pb.StringValue, *pb.StringValue]) error {
			_, err := c.Reduce(1, 0, func(acc int, _ NodeResponse[*pb.StringValue]) (int, error) { return acc + 1, nil })
			return err
		}},
		{name: "Matching", call: func(c *Call[*pb.StringValue, *pb.StringValue]) error {
			_, err := c.Matching(1, (*pb.StringValue).GetValue)
			return err
		}},
		{name: "Aggregate", call: func(c *Call[*pb.StringValue, *pb.StringValue]) error {
			_, err := c.Aggregate(firstErr)
			return err
		}},
		{name: "Results", call: func(c *Call[*pb.StringValue, *pb.StringValue]) error {
			_, err := firstErr(c.Results())
			return err
		}},
		{name: "Correctable", call: func(c *Call[*pb.StringValue, *pb.StringValue]) error {
			corr := c.Correctable(1)
			<-corr.Done()
			_, level, err := corr.Get()
			if level != LevelNotSet {
				t.Errorf("Correctable level = %d, want %d", level, LevelNotSet)
			}
			return err
		}},
		{name: "Async", call: func(c *Call[*pb.StringValue, *pb.StringValue]) error { _, err := c.Async().Majority(); return err }},
		{name: "Intercept", call: func(c *Call[*pb.StringValue, *pb.StringValue]) error {
			mapResp := MapResponse[*pb.StringValue](func(r *pb.StringValue, _ *Node) *pb.StringValue { return r })
			_, err := c.Intercept(mapResp).Majority()
			return err
		}},
	}
	configs := []struct {
		name   string
		config Config
	}{
		{name: "NilConfig", config: nil},
		{name: "EmptyConfig", config: Config{}},
	}
	checkErr := func(t *testing.T, err error) {
		t.Helper()
		if !errors.Is(err, ErrEmptyConfig) {
			t.Errorf("call error = %v, want %v", err, ErrEmptyConfig)
		}
		if got := status.Code(err); got != codes.InvalidArgument {
			t.Errorf("status.Code(err) = %v, want %v", got, codes.InvalidArgument)
		}
	}
	for _, cfg := range configs {
		for _, qc := range quorumCalls {
			for _, term := range terminals {
				t.Run(qc.name+"/"+cfg.name+"/"+term.name, func(t *testing.T) {
					checkErr(t, term.call(qc.call(t.Context(), cfg.config, req, method)))
				})
			}
		}
		onewayCalls := []struct {
			name string
			call func() error
		}{
			{name: "Send", call: func() error { return Multicast(t.Context(), cfg.config, req, method).Send() }},
			{name: "Async", call: func() error { return Multicast(t.Context(), cfg.config, req, method).Async().Wait() }},
			{name: "Intercept", call: func() error {
				skip := MapRequest[*pb.StringValue, *emptypb.Empty](func(*pb.StringValue, *Node) *pb.StringValue { return nil })
				return Multicast(t.Context(), cfg.config, req, method).Intercept(skip).Send()
			}},
		}
		for _, oc := range onewayCalls {
			t.Run("Multicast/"+cfg.name+"/"+oc.name, func(t *testing.T) {
				checkErr(t, oc.call())
			})
		}
	}
}

// TestCallEmptyConfigDispatched verifies that a terminal method on an empty
// configuration dispatches the call, so a later Intercept panics.
func TestCallEmptyConfigDispatched(t *testing.T) {
	call := QuorumCall[*pb.StringValue, *pb.StringValue](t.Context(), Config{}, pb.String("hello"), "test.Method")
	if _, err := call.Majority(); !errors.Is(err, ErrEmptyConfig) {
		t.Fatalf("Majority() error = %v, want %v", err, ErrEmptyConfig)
	}
	defer func() {
		if recover() == nil {
			t.Error("Intercept after Majority did not panic")
		}
	}()
	call.Intercept()
}

// TestCallNilNode verifies that a single-node call on a nil node does not
// panic, but reports [ErrNoNode]: a remote call returns it, and a unicast
// returns it from Send and from Wait after Async.
func TestCallNilNode(t *testing.T) {
	const method = "test.Method"
	req := pb.String("hello")
	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "RemoteCall",
			call: func() error {
				resp, err := RemoteCall[*pb.StringValue, *pb.StringValue](t.Context(), nil, req, method)
				if resp != nil {
					t.Errorf("RemoteCall() response = %v, want nil", resp)
				}
				return err
			},
		},
		{
			name: "Unicast/Send",
			call: func() error { return Unicast(t.Context(), nil, req, method).Send() },
		},
		{
			name: "Unicast/Async",
			call: func() error { return Unicast(t.Context(), nil, req, method).Async().Wait() },
		},
		{
			name: "Unicast/Intercept",
			call: func() error {
				skip := MapRequest[*pb.StringValue, *emptypb.Empty](func(*pb.StringValue, *Node) *pb.StringValue { return nil })
				return Unicast(t.Context(), nil, req, method).Intercept(skip).Send()
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if !errors.Is(err, ErrNoNode) {
				t.Errorf("call error = %v, want %v", err, ErrNoNode)
			}
			if got := status.Code(err); got != codes.InvalidArgument {
				t.Errorf("status.Code(err) = %v, want %v", got, codes.InvalidArgument)
			}
		})
	}
}

// TestCallOnewaySkipNode verifies how a one-way call reports a node that a
// request transform skips with [ErrSkipNode]: multicast drops the skipped node
// from its send errors, while unicast returns ErrSkipNode, since nothing was
// sent at all.
func TestCallOnewaySkipNode(t *testing.T) {
	const skipID = 2
	skipNode := MapRequest[*pb.StringValue, *emptypb.Empty](func(req *pb.StringValue, n *Node) *pb.StringValue {
		if n.ID() == skipID {
			return nil
		}
		return req
	})

	// newConfig returns a config with one node per ID. A node with ID down
	// has no channel, so a send to it fails with stream.ErrStreamDown.
	newConfig := func(down uint32, ids ...uint32) (Config, map[uint32]*seqNoRecorder) {
		config := make(Config, 0, len(ids))
		recorders := make(map[uint32]*seqNoRecorder)
		for _, id := range ids {
			transport := stream.NewTransport(id, func() uint64 { return 0 })
			if id != down {
				recorders[id] = &seqNoRecorder{}
				transport.StoreChannel(stream.NewLocalChannel(id, recorders[id], conn.DefaultRecvBufferSize, nil))
			}
			config = append(config, conn.NewNodeForTest(id, transport))
		}
		return config, recorders
	}
	received := func(r *seqNoRecorder) int {
		r.mu.Lock()
		defer r.mu.Unlock()
		return len(r.seqNos)
	}

	t.Run("Multicast/SkippedNodeIsNotAnError", func(t *testing.T) {
		config, recorders := newConfig(0, 1, 2, 3)
		err := Multicast(t.Context(), config, pb.String("hello"), "test.Method").Intercept(skipNode).Send()
		if err != nil {
			t.Fatalf("Send() = %v, want nil", err)
		}
		recorders[1].wait(t, 1)
		recorders[3].wait(t, 1)
		if got := received(recorders[skipID]); got != 0 {
			t.Errorf("skipped node %d received %d requests, want 0", skipID, got)
		}
	})

	t.Run("Multicast/OnlyFailedNodeIsAnError", func(t *testing.T) {
		config, _ := newConfig(3, 1, 2, 3)
		err := Multicast(t.Context(), config, pb.String("hello"), "test.Method").Intercept(skipNode).Send()
		var qcErr conn.QuorumCallError
		if !errors.As(err, &qcErr) {
			t.Fatalf("Send() = %v, want a QuorumCallError", err)
		}
		if !errors.Is(err, ErrSendFailure) {
			t.Errorf("Send() cause = %v, want %v", qcErr.Cause(), ErrSendFailure)
		}
		if got := qcErr.NumErrors(); got != 1 {
			t.Errorf("NumErrors() = %d, want 1 (the failed node only)", got)
		}
		if !errors.Is(err, stream.ErrStreamDown) {
			t.Errorf("Send() = %v, want it to wrap %v", err, stream.ErrStreamDown)
		}
		if errors.Is(err, ErrSkipNode) {
			t.Errorf("Send() = %v, want it to not wrap %v", err, ErrSkipNode)
		}
	})

	t.Run("Unicast/SkippedNodeIsAnError", func(t *testing.T) {
		config, recorders := newConfig(0, skipID)
		err := Unicast(t.Context(), config[0], pb.String("hello"), "test.Method").Intercept(skipNode).Send()
		if !errors.Is(err, ErrSkipNode) {
			t.Fatalf("Send() = %v, want %v", err, ErrSkipNode)
		}
		if got := received(recorders[skipID]); got != 0 {
			t.Errorf("skipped node %d received %d requests, want 0", skipID, got)
		}
	})
}

// valueRecorder records the values of the [pb.StringValue] requests dispatched
// to it, in arrival order. It sends no replies.
type valueRecorder struct {
	mu     sync.Mutex
	values []string
}

func (h *valueRecorder) HandleRequest(_ context.Context, _ ID, msg *stream.Message, release func(), _ func(*stream.Message)) {
	var req pb.StringValue
	if err := proto.Unmarshal(msg.GetPayload(), &req); err != nil {
		panic(err)
	}
	h.mu.Lock()
	h.values = append(h.values, req.GetValue())
	h.mu.Unlock()
	release()
}

// wait returns the recorded values once n requests have arrived, or fails
// the test after two seconds.
func (h *valueRecorder) wait(t *testing.T, n int) []string {
	t.Helper()
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		h.mu.Lock()
		if len(h.values) >= n {
			values := slices.Clone(h.values)
			h.mu.Unlock()
			return values
		}
		h.mu.Unlock()
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d requests", n)
	return nil
}

// newScatterConfig returns a configuration with one node per ID, in the given
// order, and the recorder of each node. A node with ID down has no channel, so
// a send to it fails with [stream.ErrStreamDown].
func newScatterConfig(t *testing.T, down ID, ids ...ID) (Config, map[ID]*valueRecorder) {
	t.Helper()
	config := make(Config, 0, len(ids))
	recorders := make(map[ID]*valueRecorder)
	for _, id := range ids {
		transport := stream.NewTransport(id, func() uint64 { return uint64(id) })
		if id != down {
			recorders[id] = &valueRecorder{}
			local := stream.NewLocalChannel(id, recorders[id], conn.DefaultRecvBufferSize, nil)
			t.Cleanup(local.Wait)
			transport.StoreChannel(local)
		}
		config = append(config, conn.NewNodeForTest(id, transport))
	}
	return config, recorders
}

// scatterRequests returns one request per node of config, "to-<id>" for
// config[i].
func scatterRequests(config Config) []*pb.StringValue {
	reqs := make([]*pb.StringValue, len(config))
	for i, n := range config {
		reqs[i] = pb.String(fmt.Sprintf("to-%d", n.ID()))
	}
	return reqs
}

// scatterDispatches lists the ways to dispatch a scatter call without waiting
// for replies; the recorders send none.
var scatterDispatches = []struct {
	name     string
	dispatch func(ctx context.Context, config Config, reqs []*pb.StringValue) error
}{
	{name: "QuorumCall", dispatch: func(ctx context.Context, config Config, reqs []*pb.StringValue) error {
		QuorumCallScatter[*pb.StringValue, *pb.StringValue](ctx, config, reqs, "test.Method").Async()
		return nil
	}},
	{name: "QuorumCallStream", dispatch: func(ctx context.Context, config Config, reqs []*pb.StringValue) error {
		QuorumCallStreamScatter[*pb.StringValue, *pb.StringValue](ctx, config, reqs, "test.Method").Async()
		return nil
	}},
	{name: "Multicast/Send", dispatch: func(ctx context.Context, config Config, reqs []*pb.StringValue) error {
		return MulticastScatter(ctx, config, reqs, "test.Method").Send()
	}},
	{name: "Multicast/Async", dispatch: func(ctx context.Context, config Config, reqs []*pb.StringValue) error {
		return MulticastScatter(ctx, config, reqs, "test.Method").Async().Wait()
	}},
}

// TestCallScatterPairing verifies that a scatter call sends reqs[i] to
// config[i], for a configuration whose node IDs are neither sorted nor
// consecutive. Repeated calls cover different random fan-out starts.
func TestCallScatterPairing(t *testing.T) {
	const calls = 20
	for _, sd := range scatterDispatches {
		t.Run(sd.name, func(t *testing.T) {
			config, recorders := newScatterConfig(t, 0, 7, 3, 12)
			reqs := scatterRequests(config)
			for range calls {
				if err := sd.dispatch(t.Context(), config, reqs); err != nil {
					t.Fatalf("dispatch = %v, want nil", err)
				}
			}
			for id, r := range recorders {
				want := slices.Repeat([]string{fmt.Sprintf("to-%d", id)}, calls)
				if got := r.wait(t, calls); !slices.Equal(got, want) {
					t.Errorf("node %d received %v, want %v", id, got, want)
				}
			}
		})
	}
}

// TestCallScatterCountMismatch verifies that a scatter call whose request
// count differs from the configuration size returns [ErrScatterCount] from
// every entry point and queues nothing. A broadcast sent afterwards must be the
// first request that each node receives.
func TestCallScatterCountMismatch(t *testing.T) {
	qc := func(ctx context.Context, config Config, reqs []*pb.StringValue) *Call[*pb.StringValue, *pb.StringValue] {
		return QuorumCallScatter[*pb.StringValue, *pb.StringValue](ctx, config, reqs, "test.Method")
	}
	qcs := func(ctx context.Context, config Config, reqs []*pb.StringValue) *Call[*pb.StringValue, *pb.StringValue] {
		return QuorumCallStreamScatter[*pb.StringValue, *pb.StringValue](ctx, config, reqs, "test.Method")
	}
	type entryPoint struct {
		name string
		call func(ctx context.Context, config Config, reqs []*pb.StringValue) error
	}
	var entryPoints []entryPoint
	for _, kind := range []struct {
		name string
		new  func(context.Context, Config, []*pb.StringValue) *Call[*pb.StringValue, *pb.StringValue]
	}{{"QuorumCall", qc}, {"QuorumCallStream", qcs}} {
		entryPoints = append(entryPoints,
			entryPoint{kind.name + "/First", func(ctx context.Context, c Config, r []*pb.StringValue) error {
				_, err := kind.new(ctx, c, r).First()
				return err
			}},
			entryPoint{kind.name + "/All", func(ctx context.Context, c Config, r []*pb.StringValue) error {
				_, err := kind.new(ctx, c, r).All()
				return err
			}},
			entryPoint{kind.name + "/AsyncMajority", func(ctx context.Context, c Config, r []*pb.StringValue) error {
				_, err := kind.new(ctx, c, r).Async().Majority()
				return err
			}},
			entryPoint{kind.name + "/Results", func(ctx context.Context, c Config, r []*pb.StringValue) error {
				for resp := range kind.new(ctx, c, r).Results() {
					return resp.Err
				}
				return nil
			}},
			entryPoint{kind.name + "/Correctable", func(ctx context.Context, c Config, r []*pb.StringValue) error {
				corr := kind.new(ctx, c, r).Correctable(1)
				<-corr.Done()
				_, level, err := corr.Get()
				if level != LevelNotSet {
					t.Errorf("Correctable level = %d, want %d", level, LevelNotSet)
				}
				return err
			}},
		)
	}
	entryPoints = append(entryPoints,
		entryPoint{"Multicast/Send", func(ctx context.Context, c Config, r []*pb.StringValue) error {
			return MulticastScatter(ctx, c, r, "test.Method").Send()
		}},
		entryPoint{"Multicast/Async", func(ctx context.Context, c Config, r []*pb.StringValue) error {
			return MulticastScatter(ctx, c, r, "test.Method").Async().Wait()
		}},
	)
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	contexts := []struct {
		name string
		ctx  context.Context
	}{{"Live", t.Context()}, {"Cancelled", cancelled}}

	// A nil slice must fail the count check too, not fall back to a broadcast.
	sizes := []struct {
		name string
		reqs func() []*pb.StringValue
	}{
		{"nil", func() []*pb.StringValue { return nil }},
		{"0", func() []*pb.StringValue { return []*pb.StringValue{} }},
		{"2", func() []*pb.StringValue { return []*pb.StringValue{pb.String("mismatch"), pb.String("mismatch")} }},
		{"4", func() []*pb.StringValue { return slices.Repeat([]*pb.StringValue{pb.String("mismatch")}, 4) }},
	}
	for _, ep := range entryPoints {
		for _, size := range sizes {
			for _, c := range contexts {
				t.Run(fmt.Sprintf("%s/Requests=%s/%s", ep.name, size.name, c.name), func(t *testing.T) {
					config, recorders := newScatterConfig(t, 0, 7, 3, 12)
					reqs := size.reqs()
					err := ep.call(c.ctx, config, reqs)
					if !errors.Is(err, ErrScatterCount) {
						t.Fatalf("call error = %v, want %v", err, ErrScatterCount)
					}
					if got := status.Code(err); got != codes.InvalidArgument {
						t.Errorf("status.Code(err) = %v, want %v", got, codes.InvalidArgument)
					}
					if err := Multicast(t.Context(), config, pb.String("after"), "test.Method").Send(); err != nil {
						t.Fatal(err)
					}
					for id, r := range recorders {
						if got := r.wait(t, 1); got[0] != "after" {
							t.Errorf("node %d first received %q, want %q", id, got[0], "after")
						}
					}
				})
			}
		}
	}

	t.Run("MapRequest", func(t *testing.T) {
		config, _ := newScatterConfig(t, 0, 7, 3, 12)
		ran := false
		mark := MapRequest[*pb.StringValue, *emptypb.Empty](func(req *pb.StringValue, _ *Node) *pb.StringValue {
			ran = true
			return req
		})
		reqs := []*pb.StringValue{pb.String("mismatch"), pb.String("mismatch")}
		if err := MulticastScatter(t.Context(), config, reqs, "test.Method").Intercept(mark).Send(); !errors.Is(err, ErrScatterCount) {
			t.Errorf("Send() = %v, want %v", err, ErrScatterCount)
		}
		if ran {
			t.Error("transform ran for a call with a count mismatch")
		}
	})

	t.Run("EmptyConfig", func(t *testing.T) {
		for _, ep := range entryPoints {
			if err := ep.call(t.Context(), nil, []*pb.StringValue{pb.String("x")}); !errors.Is(err, ErrEmptyConfig) {
				t.Errorf("%s: call error = %v, want %v", ep.name, err, ErrEmptyConfig)
			}
		}
	})
}

// TestCallScatterSkipNil verifies that a nil request skips its node with
// [ErrSkipNode] before any request transform runs, while the other nodes
// receive their own requests.
func TestCallScatterSkipNil(t *testing.T) {
	config, recorders := newScatterConfig(t, 0, 7, 3, 12)
	reqs := scatterRequests(config)
	reqs[1] = nil // node 3
	var (
		mu          sync.Mutex
		transformed []ID
	)
	record := MapRequest[*pb.StringValue, *emptypb.Empty](func(req *pb.StringValue, n *Node) *pb.StringValue {
		mu.Lock()
		transformed = append(transformed, n.ID())
		mu.Unlock()
		return req
	})
	if err := MulticastScatter(t.Context(), config, reqs, "test.Method").Intercept(record).Send(); err != nil {
		t.Fatalf("Send() = %v, want nil", err)
	}
	for _, id := range []ID{7, 12} {
		if got := recorders[id].wait(t, 1); got[0] != fmt.Sprintf("to-%d", id) {
			t.Errorf("node %d received %q, want %q", id, got[0], fmt.Sprintf("to-%d", id))
		}
	}
	slices.Sort(transformed)
	if want := []ID{7, 12}; !slices.Equal(transformed, want) {
		t.Errorf("transform called for %v, want %v", transformed, want)
	}
	if err := Multicast(t.Context(), config, pb.String("after"), "test.Method").Send(); err != nil {
		t.Fatal(err)
	}
	if got := recorders[3].wait(t, 1); got[0] != "after" {
		t.Errorf("skipped node 3 first received %q, want %q", got[0], "after")
	}

	t.Run("QuorumCall", func(t *testing.T) {
		config, recorders := newScatterConfig(t, 0, 7, 3, 12)
		ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
		defer cancel()
		reqs := scatterRequests(config)
		reqs[1] = nil
		var skipped []ID
		for resp := range QuorumCallScatter[*pb.StringValue, *pb.StringValue](ctx, config, reqs, "test.Method").Results() {
			if errors.Is(resp.Err, ErrSkipNode) {
				skipped = append(skipped, resp.NodeID)
			}
		}
		if want := []ID{3}; !slices.Equal(skipped, want) {
			t.Errorf("skipped nodes = %v, want %v", skipped, want)
		}
		for _, id := range []ID{7, 12} {
			if got := recorders[id].wait(t, 1); got[0] != fmt.Sprintf("to-%d", id) {
				t.Errorf("node %d received %q, want %q", id, got[0], fmt.Sprintf("to-%d", id))
			}
		}
	})
}

// TestCallScatterMapRequest verifies that each request transform of a scatter
// call receives the request of its own destination.
func TestCallScatterMapRequest(t *testing.T) {
	config, recorders := newScatterConfig(t, 0, 7, 3, 12)
	var (
		mu   sync.Mutex
		seen = make(map[ID]string)
	)
	suffix := MapRequest[*pb.StringValue, *emptypb.Empty](func(req *pb.StringValue, n *Node) *pb.StringValue {
		mu.Lock()
		seen[n.ID()] = req.GetValue()
		mu.Unlock()
		return pb.String(req.GetValue() + "!")
	})
	reqs := scatterRequests(config)
	if err := MulticastScatter(t.Context(), config, reqs, "test.Method").Intercept(suffix).Send(); err != nil {
		t.Fatalf("Send() = %v, want nil", err)
	}
	for id, r := range recorders {
		want := fmt.Sprintf("to-%d", id)
		if seen[id] != want {
			t.Errorf("transform for node %d received %q, want %q", id, seen[id], want)
		}
		if got := r.wait(t, 1); got[0] != want+"!" {
			t.Errorf("node %d received %q, want %q", id, got[0], want+"!")
		}
	}
	for i, req := range reqs {
		if want := fmt.Sprintf("to-%d", config[i].ID()); req.GetValue() != want {
			t.Errorf("reqs[%d] = %q after the call, want %q", i, req.GetValue(), want)
		}
	}
}

// onewayBuilders lists a broadcast and a scatter multicast on config. The
// broadcast sends "hello" to every node; the scatter call sends "to-<id>".
var onewayBuilders = []struct {
	name string
	want func(id ID) string
	new  func(ctx context.Context, config Config) *OnewayCall[*pb.StringValue]
}{
	{
		name: "Multicast",
		want: func(ID) string { return "hello" },
		new: func(ctx context.Context, config Config) *OnewayCall[*pb.StringValue] {
			return Multicast(ctx, config, pb.String("hello"), "test.Method")
		},
	},
	{
		name: "MulticastScatter",
		want: func(id ID) string { return fmt.Sprintf("to-%d", id) },
		new: func(ctx context.Context, config Config) *OnewayCall[*pb.StringValue] {
			return MulticastScatter(ctx, config, scatterRequests(config), "test.Method")
		},
	},
}

// TestCallMapRequestRunsAtIntercept verifies that a request transform runs
// when its interceptor is attached, once per node, in configuration order.
func TestCallMapRequestRunsAtIntercept(t *testing.T) {
	for _, b := range onewayBuilders {
		t.Run(b.name, func(t *testing.T) {
			config, recorders := newScatterConfig(t, 0, 7, 3, 12)
			var visited []ID
			record := MapRequest[*pb.StringValue, *emptypb.Empty](func(req *pb.StringValue, n *Node) *pb.StringValue {
				visited = append(visited, n.ID())
				return req
			})
			call := b.new(t.Context(), config).Intercept(record)
			if want := []ID{7, 3, 12}; !slices.Equal(visited, want) {
				t.Errorf("transform visited %v at Intercept, want %v", visited, want)
			}
			if err := call.Send(); err != nil {
				t.Fatalf("Send() = %v, want nil", err)
			}
			for id, r := range recorders {
				if got := r.wait(t, 1); got[0] != b.want(id) {
					t.Errorf("node %d received %q, want %q", id, got[0], b.want(id))
				}
			}
			if len(visited) != len(config) {
				t.Errorf("transform ran %d times, want %d", len(visited), len(config))
			}
		})
	}
}

// TestCallMapRequestNilShared verifies that a request transform of a broadcast
// call receives the shared request even when it is nil, so the transform can
// build each node's request.
func TestCallMapRequestNilShared(t *testing.T) {
	config, recorders := newScatterConfig(t, 0, 7, 3, 12)
	build := MapRequest[*pb.StringValue, *emptypb.Empty](func(_ *pb.StringValue, n *Node) *pb.StringValue {
		return pb.String(fmt.Sprintf("built-%d", n.ID()))
	})
	if err := Multicast[*pb.StringValue](t.Context(), config, nil, "test.Method").Intercept(build).Send(); err != nil {
		t.Fatalf("Send() = %v, want nil", err)
	}
	for id, r := range recorders {
		if want, got := fmt.Sprintf("built-%d", id), r.wait(t, 1)[0]; got != want {
			t.Errorf("node %d received %q, want %q", id, got, want)
		}
	}
}

// TestCallMapRequestChain verifies that chained request transforms compose in
// call-site order, and that a node that one transform skips reaches no later
// transform and receives nothing.
func TestCallMapRequestChain(t *testing.T) {
	const skipID = 3
	for _, b := range onewayBuilders {
		t.Run(b.name, func(t *testing.T) {
			config, recorders := newScatterConfig(t, 0, 7, 3, 12)
			first := MapRequest[*pb.StringValue, *emptypb.Empty](func(req *pb.StringValue, n *Node) *pb.StringValue {
				if n.ID() == skipID {
					return nil
				}
				return pb.String(req.GetValue() + "!")
			})
			var seen []ID
			second := MapRequest[*pb.StringValue, *emptypb.Empty](func(req *pb.StringValue, n *Node) *pb.StringValue {
				seen = append(seen, n.ID())
				return pb.String(req.GetValue() + "?")
			})
			if err := b.new(t.Context(), config).Intercept(first, second).Send(); err != nil {
				t.Fatalf("Send() = %v, want nil", err)
			}
			if want := []ID{7, 12}; !slices.Equal(seen, want) {
				t.Errorf("second transform saw %v, want %v", seen, want)
			}
			for _, id := range []ID{7, 12} {
				if got := recorders[id].wait(t, 1); got[0] != b.want(id)+"!?" {
					t.Errorf("node %d received %q, want %q", id, got[0], b.want(id)+"!?")
				}
			}
			if err := Multicast(t.Context(), config, pb.String("after"), "test.Method").Send(); err != nil {
				t.Fatal(err)
			}
			if got := recorders[skipID].wait(t, 1); got[0] != "after" {
				t.Errorf("skipped node %d first received %q, want %q", skipID, got[0], "after")
			}
		})
	}
}

// TestCallScatterRequest verifies that [CallContext.Request] returns the zero
// value for a scatter call.
func TestCallScatterRequest(t *testing.T) {
	config, _ := newScatterConfig(t, 0, 7, 3, 12)
	got := pb.String("unset")
	inspect := func(ctx *CallContext[*pb.StringValue, *emptypb.Empty], next ResponseSeq[*emptypb.Empty]) ResponseSeq[*emptypb.Empty] {
		got = ctx.Request()
		return next
	}
	if err := MulticastScatter(t.Context(), config, scatterRequests(config), "test.Method").Intercept(inspect).Send(); err != nil {
		t.Fatalf("Send() = %v, want nil", err)
	}
	if got != nil {
		t.Errorf("Request() = %v, want nil", got)
	}
}

// TestCallScatterFailedNode verifies that a send error of a scatter call is
// attributed to the node whose send failed.
func TestCallScatterFailedNode(t *testing.T) {
	config, recorders := newScatterConfig(t, 3, 7, 3, 12)
	err := MulticastScatter(t.Context(), config, scatterRequests(config), "test.Method").Send()
	qcErr, ok := errors.AsType[conn.QuorumCallError](err)
	if !ok {
		t.Fatalf("Send() = %v, want a QuorumCallError", err)
	}
	if got := qcErr.NumErrors(); got != 1 {
		t.Fatalf("NumErrors() = %d, want 1", got)
	}
	if !slices.Equal(config.WithoutErrors(qcErr).NodeIDs(), []ID{7, 12}) {
		t.Errorf("failed nodes = %v, want only node 3", err)
	}
	for _, id := range []ID{7, 12} {
		if got := recorders[id].wait(t, 1); got[0] != fmt.Sprintf("to-%d", id) {
			t.Errorf("node %d received %q, want %q", id, got[0], fmt.Sprintf("to-%d", id))
		}
	}
}
