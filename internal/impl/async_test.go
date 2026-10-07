package impl

import (
	"testing"

	"google.golang.org/protobuf/proto"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

// TestAsyncThresholdDispatchedFlagIsRaceFree exercises AsyncThreshold's
// concurrent writes to the dispatched flag against a concurrent read: the
// initial sendNow marks it on the caller's goroutine, and ranging over r.seq
// inside the spawned goroutine calls sendNow again, redundantly re-marking it.
// Run with -race: a plain bool here is flagged as a data race against any
// concurrent Intercept call, even though both writes agree on the value.
func TestAsyncThresholdDispatchedFlagIsRaceFree(t *testing.T) {
	responses := []NodeResponse[proto.Message]{
		{NodeID: 1, Value: pb.String("response1"), Err: nil},
		{NodeID: 2, Value: pb.String("response2"), Err: nil},
	}
	callCtx := makeCallContext[*pb.StringValue, *pb.StringValue](t, 2, responses)
	r := newResponses(callCtx)

	fut := r.AsyncThreshold(1)

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Already dispatched by AsyncThreshold, so this always panics; the
		// race is on the concurrent read of the flag that triggers it, not
		// on the outcome.
		defer func() { _ = recover() }()
		callCtx.intercept()
	}()

	if _, err := fut.Get(); err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	<-done
}
