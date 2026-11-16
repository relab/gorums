package impl

import (
	"context"
	"errors"
	"io"
	"testing"
	"testing/synctest"
	"time"

	"github.com/relab/gorums/internal/conn"
	"github.com/relab/gorums/internal/stream"
	"github.com/relab/gorums/internal/testutils/mock"
	"google.golang.org/protobuf/types/known/emptypb"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

// TestCallAdmission separates queue admission from stream completion under
// backpressure, including a multicast with one saturated destination.
func TestCallAdmission(t *testing.T) {
	for _, fail := range []bool{false, true} {
		name := "Complete"
		if fail {
			name = "Fail"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				gated := mock.NewGatedBidiStream[*stream.Message]()
				ch := stream.NewInboundChannel(t.Context(), 1, gated, stream.InboundOptions{SendBufferSize: 1, RecvBufferSize: 1})
				t.Cleanup(func() { gated.Close(); _ = ch.Close() })
				tr := stream.NewTransport(1, func() uint64 { return 1 })
				tr.StoreChannel(ch)
				node := conn.NewNodeForTest(1, tr)
				ctx, cancel := context.WithTimeout(t.Context(), time.Second)
				defer cancel()
				// Send must return before the gated stream write completes.
				h := Unicast(ctx, node, pb.String("first"), "test").Async()
				select {
				case <-gated.Entered():
				case <-ctx.Done():
					t.Fatal("sender did not start")
				}
				if err := Unicast(ctx, node, pb.String("queued"), "test").Send(); err != nil {
					t.Fatal(err)
				}
				// Force a fixed fan-out order using a request transform.
				healthy := &seqNoRecorder{}
				local := stream.NewLocalChannel(2, healthy, 1, nil)
				t.Cleanup(local.Wait)
				healthyTr := stream.NewTransport(2, func() uint64 { return 2 })
				healthyTr.StoreChannel(local)
				cfg := Config{node, conn.NewNodeForTest(2, healthyTr)}
				call := Multicast(ctx, cfg, pb.String("overflow"), "test").Intercept(MapRequest[*pb.StringValue, *emptypb.Empty](func(m *pb.StringValue, _ *Node) *pb.StringValue { return m }))
				err := call.Send()
				if !errors.Is(err, stream.ErrSendQueueFull) {
					t.Fatalf("Send = %v, want queue full", err)
				}
				healthy.wait(t, 1)
				if aggregate, ok := errors.AsType[conn.QuorumCallError](err); !ok || aggregate.NumErrors() != 1 {
					t.Fatalf("admission failures = %v, want one destination", err)
				}
				overflow := Multicast(ctx, cfg, pb.String("async overflow"), "test").Async()
				healthy.wait(t, 2)
				if err := overflow.Wait(); !errors.Is(err, stream.ErrSendQueueFull) {
					t.Fatalf("async overflow = %v", err)
				}
				select {
				case <-gated.Sent():
					t.Fatal("write completed before release")
				default:
				}
				completed := make(chan error, 1)
				go func() { completed <- h.Wait() }()
				synctest.Wait()
				select {
				case err := <-completed:
					t.Fatalf("Wait returned before write: %v", err)
				default:
				}
				if fail {
					gated.Close()
				} else {
					gated.Release()
				}
				err = <-completed
				if fail && !errors.Is(err, io.EOF) {
					t.Fatalf("Wait = %v, want EOF", err)
				}
				if !fail && err != nil {
					t.Fatal(err)
				}
				if again := h.Wait(); again != err {
					t.Fatalf("repeated Wait = %v, want %v", again, err)
				}
				if got := ch.DroppedReplies(); got != 0 {
					t.Fatalf("request counted as dropped reply: %d", got)
				}
			})
		})
	}
}
