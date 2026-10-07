package impl

import (
	"errors"
	"testing"

	"github.com/relab/gorums/internal/conn"
	"github.com/relab/gorums/internal/stream"
	"google.golang.org/protobuf/types/known/emptypb"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

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
		err := Multicast(config.Context(t.Context()), pb.String("hello"), "test.Method").Intercept(skipNode).Send()
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
		err := Multicast(config.Context(t.Context()), pb.String("hello"), "test.Method").Intercept(skipNode).Send()
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
		err := Unicast(config[0].Context(t.Context()), pb.String("hello"), "test.Method").Intercept(skipNode).Send()
		if !errors.Is(err, ErrSkipNode) {
			t.Fatalf("Send() = %v, want %v", err, ErrSkipNode)
		}
		if got := received(recorders[skipID]); got != 0 {
			t.Errorf("skipped node %d received %d requests, want 0", skipID, got)
		}
	})
}
