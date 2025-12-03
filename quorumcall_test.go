package gorums_test

import (
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/internal/testutils/mock"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

func TestQuorumCallSuccess(t *testing.T) {
	cfg := gorums.TestConfiguration(t, 3, nil)

	ctx := gorums.TestContext(t, 1*time.Second)
	cfgCtx := gorums.WithConfigContext(ctx, cfg)

	// Use the new Responses API with the All() terminal method
	response, err := gorums.QuorumCall[*pb.StringValue, *pb.StringValue](
		cfgCtx, pb.String(""), mock.TestMethod,
	).All()
	if err != nil {
		t.Fatalf("Unexpected error, got: %v, want: %v", err, nil)
	}
	if response == nil {
		t.Fatalf("Unexpected response, got: %v, want: %v", response, pb.String(""))
	}
}
