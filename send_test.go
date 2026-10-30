package gorums_test

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/gorumstest"
	"github.com/relab/gorums/internal/testutils/mockcall"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestSendOversizedRequest verifies that a local send-size error fails the
// request without replaying it, and that the node can still serve other calls.
func TestSendOversizedRequest(t *testing.T) {
	for _, oneway := range []bool{false, true} {
		name := "TwoWay"
		if oneway {
			name = "Oneway"
		}
		t.Run(name, func(t *testing.T) {
			var sends atomic.Int32
			node := gorumstest.Node(t, nil, gorums.WithGRPCDialOptions(
				grpc.WithDefaultCallOptions(grpc.MaxCallSendMsgSize(256)),
				grpc.WithStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
					s, err := streamer(ctx, desc, cc, method, opts...)
					if err != nil {
						return nil, err
					}
					return &countingSendStream{ClientStream: s, sends: &sends}, nil
				}),
			))
			ctx := gorumstest.Context(t, 2*time.Second)
			large := strings.Repeat("x", 512)
			var err error
			if oneway {
				err = mockcall.Unicast(ctx, node, large).Send()
			} else {
				_, err = mockcall.RemoteCall(ctx, node, large)
			}
			if got := status.Code(err); got != codes.ResourceExhausted {
				t.Errorf("oversized request: code = %v, want ResourceExhausted (error: %v)", got, err)
			}
			if got := sends.Load(); got != 1 {
				t.Errorf("oversized request sent %d times, want 1", got)
			}
			if got := node.PendingCount(); got != 0 {
				t.Errorf("pending calls = %d, want 0", got)
			}

			ctx = gorumstest.Context(t, 2*time.Second)
			if _, err := mockcall.RemoteCall(ctx, node, "small"); err != nil {
				t.Fatalf("following normal call: %v", err)
			}
		})
	}
}

// countingSendStream counts attempts, including messages rejected by gRPC.
type countingSendStream struct {
	grpc.ClientStream
	sends *atomic.Int32
}

func (s *countingSendStream) SendMsg(msg any) error {
	s.sends.Add(1)
	return s.ClientStream.SendMsg(msg)
}
