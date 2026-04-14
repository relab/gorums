package gorums

import (
	"fmt"
	"testing"
	"time"

	"github.com/relab/gorums/internal/testutils/mock"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

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
	systems := TestSystems(t, 3)
	for _, sys := range systems {
		sys.RegisterService(nil, func(srv *Server) {
			srv.RegisterHandler(mock.TestMethod, func(_ ServerCtx, _ *Message) (*Message, error) {
				return nil, nil
			})
		})
	}
	for _, sys := range systems {
		sys.WaitForConfig(t.Context(), func(cfg Configuration) bool {
			return cfg.Size() == 3
		})
	}
	cfg := systems[0].OutboundConfig()
	for i := range 1000 {
		ctx := TestContext(t, 5*time.Second)
		Multicast(cfg.Context(ctx), pb.String(fmt.Sprintf("mc-%d", i)), mock.TestMethod, IgnoreErrors())
	}
	time.Sleep(500 * time.Millisecond)
	for _, node := range cfg.Nodes() {
		if pc := node.PendingCount(); pc > 0 {
			t.Errorf("node %d: pending = %d; expected 0", node.ID(), pc)
		}
	}
}

func BenchmarkGetCallOptions(b *testing.B) {
	interceptor := func(_ *ClientCtx[msg, msg], next ResponseSeq[msg]) ResponseSeq[msg] { return next }
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
