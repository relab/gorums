package gorums_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/internal/testutils/mock"
	gorumsimpl "github.com/relab/gorums/runtime/gorumsimpl"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

// TestStreamDedupInflightCallFailsWithStreamDown verifies that a borrower call
// in flight when the shared stream drops fails with ErrStreamDown, so callers
// that retry on it retry.
func TestStreamDedupInflightCallFailsWithStreamDown(t *testing.T) {
	servers := dispatchSetup(t, true)
	owner, borrower := servers[0], servers[1]
	started := make(chan struct{}, 1)
	unblock := make(chan struct{})
	defer close(unblock)
	owner.RegisterHandler(mock.TestMethod, func(ctx gorums.ServerContext, _ *gorums.Message) (*gorums.Message, error) {
		started <- struct{}{}
		select {
		case <-unblock:
		case <-ctx.Done():
		}
		return nil, ctx.Err()
	})
	wctx, wcancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer wcancel()
	for _, srv := range servers {
		if _, err := srv.WaitForAll(wctx); err != nil {
			t.Fatal(err)
		}
	}
	n1 := dispatchNode(t, borrower.PeerConfig(), 1)
	errc := make(chan error, 1)
	go func() {
		c, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		_, err := gorumsimpl.RemoteCall[*pb.StringValue, *pb.StringValue](n1.Context(c), pb.String("x"), mock.TestMethod)
		errc <- err
	}()
	<-started
	owner.Stop() // the owner's outbound channel closes; the shared stream drops
	err := <-errc
	if !errors.Is(err, gorums.ErrStreamDown) {
		t.Errorf("in-flight call on dropped shared stream: got %v, want ErrStreamDown", err)
	}
}
