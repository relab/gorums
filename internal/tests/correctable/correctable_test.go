package correctable

import (
	"context"
	"testing"
	"time"

	"github.com/relab/gorums"
)

// run a test on a correctable call.
// n is the number of replicas.
// the target level is n (quorum size).
func run(t testing.TB, n int, corr func(*gorums.ConfigContext) *gorums.Correctable[*CorrectableResponse]) {
	t.Helper()
	addrs, teardown := gorums.TestSetup(t, n, func(i int) gorums.ServerIface {
		gorumsSrv := gorums.NewServer()
		RegisterCorrectableTestServer(gorumsSrv, &testSrv{n})
		return gorumsSrv
	})
	defer teardown()

	cfg := gorums.NewConfig(t, addrs)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	configCtx := gorums.WithConfigContext(ctx, cfg)
	res := corr(configCtx)

	donech := res.Done()
	for i := 1; i <= n; i++ {
		select {
		case <-res.Watch(i):
		case <-donech:
		}
	}
	<-donech
	if _, _, err := res.Get(); err != nil {
		t.Error(err)
	}
}

func TestCorrectable(t *testing.T) {
	run(t, 4, func(ctx *gorums.ConfigContext) *gorums.Correctable[*CorrectableResponse] {
		// Use the new API - Correctable returns *Correctable[*CorrectableResponse] directly
		// with a default majority threshold
		return Correctable(ctx, &CorrectableRequest{})
	})
}

func TestCorrectableStream(t *testing.T) {
	run(t, 4, func(ctx *gorums.ConfigContext) *gorums.Correctable[*CorrectableResponse] {
		// Use the new API - CorrectableStream returns *Correctable[*CorrectableResponse] directly
		// with a default majority threshold
		return CorrectableStream(ctx, &CorrectableRequest{})
	})
}

type testSrv struct {
	n int
}

func (srv testSrv) Correctable(_ gorums.ServerCtx, request *CorrectableRequest) (*CorrectableResponse, error) {
	return CorrectableResponse_builder{Level: 1}.Build(), nil
}

func (srv testSrv) CorrectableStream(_ gorums.ServerCtx, request *CorrectableRequest, send func(response *CorrectableResponse) error) error {
	for i := range srv.n {
		err := send(CorrectableResponse_builder{Level: int32(i + 1)}.Build())
		if err != nil {
			return err
		}
	}
	return nil
}
