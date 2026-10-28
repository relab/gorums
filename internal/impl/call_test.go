package impl

import (
	"errors"
	"fmt"
	"testing"

	"github.com/relab/gorums/internal/conn"
	"github.com/relab/gorums/internal/stream"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

// TestCallConstructorsPanic verifies that each multi-node call constructor
// panics on a nil or empty configuration.
func TestCallConstructorsPanic(t *testing.T) {
	const method = "test.Method"
	req := pb.String("hello")
	tests := []struct {
		name      string
		call      func()
		wantPanic string
	}{
		{
			name:      "QuorumCall/NilConfig",
			call:      func() { QuorumCall[*pb.StringValue, *pb.StringValue](t.Context(), nil, req, method) },
			wantPanic: `gorums: QuorumCall "test.Method" needs a non-empty configuration`,
		},
		{
			name:      "QuorumCall/EmptyConfig",
			call:      func() { QuorumCall[*pb.StringValue, *pb.StringValue](t.Context(), Config{}, req, method) },
			wantPanic: `gorums: QuorumCall "test.Method" needs a non-empty configuration`,
		},
		{
			name:      "QuorumCallStream/NilConfig",
			call:      func() { QuorumCallStream[*pb.StringValue, *pb.StringValue](t.Context(), nil, req, method) },
			wantPanic: `gorums: QuorumCallStream "test.Method" needs a non-empty configuration`,
		},
		{
			name:      "QuorumCallStream/EmptyConfig",
			call:      func() { QuorumCallStream[*pb.StringValue, *pb.StringValue](t.Context(), Config{}, req, method) },
			wantPanic: `gorums: QuorumCallStream "test.Method" needs a non-empty configuration`,
		},
		{
			name:      "Multicast/NilConfig",
			call:      func() { Multicast(t.Context(), nil, req, method) },
			wantPanic: `gorums: Multicast "test.Method" needs a non-empty configuration`,
		},
		{
			name:      "Multicast/EmptyConfig",
			call:      func() { Multicast(t.Context(), Config{}, req, method) },
			wantPanic: `gorums: Multicast "test.Method" needs a non-empty configuration`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if got := fmt.Sprint(recover()); got != tt.wantPanic {
					t.Errorf("panic = %q, want %q", got, tt.wantPanic)
				}
			}()
			tt.call()
		})
	}
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
			if got := status.Code(err); got != codes.Unavailable {
				t.Errorf("status.Code(err) = %v, want %v", got, codes.Unavailable)
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
				transport.StoreChannel(stream.NewLocalChannel(id, recorders[id]))
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
