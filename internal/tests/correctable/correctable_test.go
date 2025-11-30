package correctable

import (
	"context"
	"testing"
	"time"

	"github.com/relab/gorums"
)

// run a test on a correctable call.
// n is the number of replicas, and div is a divider.
// the target level is n, and the level is calculated by the quorum function
// by dividing the sum of levels from the servers with the divider.
func run(t testing.TB, n int, corr func(*gorums.ConfigContext) *CorrectableCorrectableResponse) {
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

var correctableQF = func(div, doneLevel int) gorums.QuorumFunc[*CorrectableRequest, *CorrectableResponse, *gorums.Correctable[*CorrectableResponse]] {
	return func(ctx *gorums.ClientCtx[*CorrectableRequest, *CorrectableResponse]) (*gorums.Correctable[*CorrectableResponse], error) {
		corr := gorums.NewCorrectable[*CorrectableResponse]()
		go func() {
			replies := make(map[uint32]*CorrectableResponse)
			for resp := range ctx.Responses().IgnoreErrors() {
				replies[resp.NodeID] = resp.Value

				// Incremental update logic
				sum := 0
				for _, r := range replies {
					sum += int(r.GetLevel())
				}
				level := sum / div
				done := level >= doneLevel
				corr.Update(CorrectableResponse_builder{Level: int32(level)}.Build(), level, done, nil)
				if done {
					return
				}
			}
		}()
		return corr, nil
	}
}

func TestCorrectable(t *testing.T) {
	run(t, 4, func(ctx *gorums.ConfigContext) *CorrectableCorrectableResponse {
		qf := correctableQF(1, 4)
		return Correctable(ctx, &CorrectableRequest{}, gorums.WithQuorumFunc(qf))
	})
}

func TestCorrectableStream(t *testing.T) {
	run(t, 4, func(ctx *gorums.ConfigContext) *CorrectableStreamCorrectableResponse {
		qf := correctableQF(4, 4)
		return CorrectableStream(ctx, &CorrectableRequest{}, gorums.WithQuorumFunc(qf))
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
