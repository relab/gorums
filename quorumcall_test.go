package gorums_test

import (
	"context"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/internal/testutils/mock"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

func TestQuorumCallSuccess(t *testing.T) {
	addrs, teardown := gorums.TestSetup(t, 3, nil)
	t.Cleanup(teardown)

	cfg := gorums.NewConfig(t, addrs)

	// Use QuorumSpecFunc to create a quorum function that requires all 3 responses
	qf := gorums.QuorumSpecFunc(func(_ *pb.StringValue, replies map[uint32]*pb.StringValue) (*pb.StringValue, bool) {
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
	})

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	cfgCtx := gorums.WithConfigContext(ctx, cfg)
	response, err := gorums.QuorumCallWithInterceptor(
		cfgCtx, pb.String(""), mock.TestMethod, qf,
	)
	if err != nil {
		t.Fatalf("Unexpected error, got: %v, want: %v", err, nil)
	}
	if response == nil {
		t.Fatalf("Unexpected response, got: %v, want: %v", response, pb.String(""))
	}
}
