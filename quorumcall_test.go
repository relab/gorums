package gorums_test

import (
	"context"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/internal/testutils/mock"
	"google.golang.org/protobuf/proto"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

func TestQuorumCallSuccess(t *testing.T) {
	addrs, teardown := gorums.TestSetup(t, 3, nil)
	t.Cleanup(teardown)

	cfg := gorums.NewConfig(t, addrs)

	cd := gorums.QuorumCallData{
		Message: pb.String(""),
		Method:  mock.TestMethod,
		QuorumFunction: func(_ proto.Message, replies map[uint32]proto.Message) (proto.Message, bool) {
			t.Logf("Received %d replies: %v", len(replies), replies)
			if len(replies) > 2 {
				for _, reply := range replies {
					if reply == nil {
						continue
					}
					return reply, true
				}
			}
			return nil, false
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	response, err := cfg.QuorumCall(ctx, cd)
	if err != nil {
		t.Fatalf("Unexpected error, got: %v, want: %v", err, nil)
	}
	if response == nil {
		t.Fatalf("Unexpected response, got: %v, want: %v", response, pb.String(""))
	}
}
