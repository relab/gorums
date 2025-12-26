package correctable

import (
	"testing"
	"time"

	"github.com/relab/gorums"
)

// run a test on a correctable call.
// n is the number of replicas.
// the target level is n (quorum size).
func run(t testing.TB, n int, corr func(*gorums.ConfigContext, int) CorrectableResponse) {
	t.Helper()
	config := gorums.TestConfiguration(t, n, func(_ int) gorums.ServerIface {
		gorumsSrv := gorums.NewServer()
		RegisterCorrectableTestServer(gorumsSrv, &testSrv{n})
		return gorumsSrv
	})

	ctx := gorums.TestContext(t, 100*time.Millisecond)
	configCtx := config.Context(ctx)
	res := corr(configCtx, n)

	done := res.Done()
	for i := 1; i <= n; i++ {
		select {
		case <-res.Watch(i):
		case <-done:
		}
	}
	<-done
	if _, _, err := res.Get(); err != nil {
		t.Error(err)
	}
}

func TestCorrectable(t *testing.T) {
	run(t, 4, func(ctx *gorums.ConfigContext, n int) CorrectableResponse {
		// Correctable returns *Responses, user calls Correctable to get *Correctable
		return Correctable(ctx, &Request{}).Correctable(n)
	})
}

func TestCorrectableStream(t *testing.T) {
	run(t, 4, func(ctx *gorums.ConfigContext, n int) CorrectableResponse {
		// CorrectableStream returns *Responses, user calls Correctable to get *Correctable
		return CorrectableStream(ctx, &Request{}).Correctable(n)
	})
}

// TestCorrectableWithWatch tests progressive level watching using the type alias
func TestCorrectableWithWatch(t *testing.T) {
	n := 4
	config := gorums.TestConfiguration(t, n, func(_ int) gorums.ServerIface {
		gorumsSrv := gorums.NewServer()
		RegisterCorrectableTestServer(gorumsSrv, &testSrv{n})
		return gorumsSrv
	})

	ctx := gorums.TestContext(t, 100*time.Millisecond)
	configCtx := config.Context(ctx)

	// Use the type alias for the correctable result
	corr := CorrectableStream(configCtx, &Request{}).Correctable(n)

	// Watch for each level progressively
	for level := 1; level <= n; level++ {
		select {
		case <-corr.Watch(level):
			resp, gotLevel, err := corr.Get()
			if err != nil {
				t.Errorf("level %d: unexpected error: %v", level, err)
			}
			if gotLevel < level {
				t.Errorf("level %d: expected level >= %d, got %d", level, level, gotLevel)
			}
			if resp == nil {
				t.Errorf("level %d: expected non-nil response", level)
			}
		case <-corr.Done():
			// Done is also acceptable
		}
	}

	<-corr.Done()
	resp, finalLevel, err := corr.Get()
	if err != nil {
		t.Errorf("final: unexpected error: %v", err)
	}
	if finalLevel != n {
		t.Errorf("final: expected level %d, got %d", n, finalLevel)
	}
	if resp == nil {
		t.Error("final: expected non-nil response")
	}
}

type testSrv struct {
	n int
}

func (testSrv) Correctable(_ gorums.ServerCtx, _ *Request) (*Response, error) {
	return Response_builder{Level: 1}.Build(), nil
}

func (srv testSrv) CorrectableStream(_ gorums.ServerCtx, _ *Request, send func(response *Response) error) error {
	for i := range srv.n {
		err := send(Response_builder{Level: int32(i + 1)}.Build())
		if err != nil {
			return err
		}
	}
	return nil
}
