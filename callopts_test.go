package gorums

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/relab/gorums/internal/testutils/mock"
	"go.uber.org/goleak"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

// testLocalServers returns n started Gorums servers forming a symmetric peer
// group on random localhost ports. It is the in-package counterpart of
// gorumstest.LocalServers, which this file cannot use: gorumstest imports
// gorums, so importing it from package gorums's own tests would create an
// import cycle.
func testLocalServers(t testing.TB, n int) []*Server {
	t.Helper()
	if _, ok := t.(*testing.B); !ok {
		t.Cleanup(func() { goleak.VerifyNone(t) })
	}
	srvs, stop, err := NewLocalServers(n, WithLocalDialOptions(
		WithGRPCDialOptions(grpc.WithTransportCredentials(insecure.NewCredentials())),
	))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stop)
	for _, srv := range srvs {
		go srv.ListenAndServe()
	}
	return srvs
}

// testWaitUntil polls predicate until it returns true or timeout elapses.
// It is the in-package counterpart of gorumstest.WaitUntil.
func testWaitUntil(t testing.TB, timeout time.Duration, predicate func() bool) bool {
	t.Helper()
	if predicate() {
		return true
	}
	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return predicate()
		case <-ticker.C:
			if predicate() {
				return true
			}
		}
	}
}

func TestCallOptionsIgnoreErrors(t *testing.T) {
	tests := []struct {
		name             string
		callOpts         callOptions
		wantIgnoreErrors bool
	}{
		{name: "Default", callOpts: getCallOptions(), wantIgnoreErrors: false},
		{name: "IgnoreErrors", callOpts: getCallOptions(IgnoreErrors()), wantIgnoreErrors: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.callOpts.ignoreErrors; got != tt.wantIgnoreErrors {
				t.Errorf("ignoreErrors = %v, want %v", got, tt.wantIgnoreErrors)
			}
		})
	}
}

func TestCallOptionsIgnoreErrorsResourceLeak(t *testing.T) {
	// Previously leaked because fire-and-forget multicast still registered in router.
	// Now fixed: no replyChan → no ResponseChan → no Register.
	servers := testLocalServers(t, 3)
	for _, srv := range servers {
		srv.RegisterHandler(mock.TestMethod, func(_ ServerContext, _ *Message) (*Message, error) {
			return nil, nil
		})
	}
	for _, srv := range servers {
		srv.WaitForPeers(t.Context(), func(cfg Config) bool {
			return cfg.Size() == 3
		})
	}
	cfg := servers[0].PeerConfig()
	ctx := testTimeoutContext(t, 5*time.Second)
	for i := range 1000 {
		Multicast(cfg.Context(ctx), pb.String(fmt.Sprintf("mc-%d", i)), mock.TestMethod, IgnoreErrors())
	}
	testWaitUntil(t, 5*time.Second, func() bool {
		for _, node := range cfg.Nodes() {
			if node.PendingCount() > 0 {
				return false
			}
		}
		return true
	})

	for _, node := range cfg.Nodes() {
		if pc := node.PendingCount(); pc > 0 {
			t.Errorf("node %d: pending = %d; expected 0", node.ID(), pc)
		}
	}
}

func BenchmarkGetCallOptions(b *testing.B) {
	interceptor := func(_ *CallContext[msg, msg], next ResponseSeq[msg]) ResponseSeq[msg] { return next }
	tests := []struct {
		numOpts int
	}{
		{0}, {1}, {2}, {3}, {4}, {5},
	}

	for _, tc := range tests {
		opts := make([]CallOption, tc.numOpts)
		for i := range tc.numOpts {
			opts[i] = Interceptors(interceptor)
		}
		b.Run(fmt.Sprintf("options=%d", tc.numOpts), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = getCallOptions(opts...)
			}
		})
	}
}
