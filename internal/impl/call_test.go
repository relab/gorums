package impl

import (
	"context"
	"errors"
	"testing"

	"github.com/relab/gorums/internal/conn"
	"github.com/relab/gorums/internal/stream"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
				transport.StoreChannel(stream.NewLocalChannel(id, recorders[id], conn.DefaultRecvBufferSize))
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
