package impl

import (
	"errors"
	"testing"

	"github.com/relab/gorums/internal/conn"
	"google.golang.org/protobuf/proto"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

// TestCorrectableEmptyConfigWatch verifies that watches registered after an
// empty call completes close immediately, including levels it cannot reach.
func TestCorrectableEmptyConfigWatch(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		name := "QuorumCall"
		if streaming {
			name = "QuorumCallStream"
		}
		t.Run(name, func(t *testing.T) {
			call := invokeQuorumCall[*pb.StringValue, *pb.StringValue](t.Context(), nil, pb.String("hello"), "test.Method", streaming)
			corr := call.Correctable(1)
			select {
			case <-corr.Done():
			default:
				t.Fatal("empty correctable did not complete synchronously")
			}
			if _, level, err := corr.Get(); level != LevelNotSet || !errors.Is(err, ErrEmptyConfig) {
				t.Fatalf("Get() level = %d, error = %v; want %d, %v", level, err, LevelNotSet, ErrEmptyConfig)
			}
			for _, level := range []int{1, 2} {
				select {
				case <-corr.Watch(level):
				default:
					t.Errorf("Watch(%d) remained open after completion", level)
				}
			}
		})
	}
}

// TestCorrectableSkipNodeSemantics verifies that Correctable treats
// ErrSkipNode consistently with the terminal methods: a skipped node must
// count toward neither the reached level nor the node-error count.
func TestCorrectableSkipNodeSemantics(t *testing.T) {
	responses := []NodeResponse[proto.Message]{
		{NodeID: 1, Value: pb.String("response1"), Err: nil},
		{NodeID: 2, Value: nil, Err: ErrSkipNode},
		{NodeID: 3, Value: pb.String("response3"), Err: nil},
	}
	callCtx := makeCallContext[*pb.StringValue, *pb.StringValue](t, 3, responses)
	r := newResponses(callCtx)

	// Threshold of 3 can never be reached: only 2 of 3 nodes produced a
	// real response, and the skipped node must not pad the count.
	corr := r.Correctable(3)
	<-corr.Done()

	_, level, err := corr.Get()
	if level != 2 {
		t.Errorf("level = %d, want 2 (the skipped node must not count)", level)
	}
	if !errors.Is(err, ErrIncomplete) {
		t.Fatalf("err = %v, want ErrIncomplete", err)
	}
	var qcErr conn.QuorumCallError
	if errors.As(err, &qcErr) && qcErr.NumErrors() != 0 {
		t.Errorf("NumErrors() = %d, want 0 (a skipped node is not a node error)", qcErr.NumErrors())
	}
}
