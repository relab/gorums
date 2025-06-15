package correctable

import (
	"context"
	"testing"
	"time"

	"github.com/relab/gorums"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// run a test on a correctable call.
// n is the number of replicas, and div is a divider.
// the target level is n, and the level is calculated by the quorum function
// by dividing the sum of levels from the servers with the divider.
func run(t *testing.T, n, div int, corr func(context.Context, *CorrectableTestConfiguration, int, int) *gorums.Correctable[int]) {
	addrs, teardown := gorums.TestSetup(t, n, func(_ int) gorums.ServerIface {
		gorumsSrv := gorums.NewServer()
		RegisterCorrectableTestServer(gorumsSrv, &testSrv{n})
		return gorumsSrv
	})
	defer teardown()

	cfg, err := NewCorrectableTestConfiguration(
		gorums.WithNodeList(addrs),
		gorums.WithGrpcDialOptions(
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	res := corr(ctx, cfg, n, div)

	donech := res.Done()

	for i := 1; i <= n; i++ {
		select {
		case <-res.Watch(i):
		case <-donech:
		}
	}

	<-donech

	_, _, err = res.Get()
	if err != nil {
		t.Error(err)
	}
}

type qspec struct {
	div       int
	doneLevel int
}

func (q qspec) corrQF(responses gorums.Responses[*CorrectableResponse], levelSet func(int, int, error)) {
	sum := 0
	for response := range responses {
		msg, err, _ := response.Unpack()
		if err != nil {
			levelSet(0, 0, err)
			continue
		}
		sum += int(msg.GetLevel())
		level := sum / q.div
		levelSet(level, level, nil)
		if level >= q.doneLevel {
			return
		}
	}
}

func TestCorrectable(t *testing.T) {
	run(t, 4, 1, func(ctx context.Context, c *CorrectableTestConfiguration, n, div int) *gorums.Correctable[int] {
		corr := c.Correctable(ctx, &CorrectableRequest{})
		qspec := qspec{div, n}
		return gorums.NewCorrectable(corr, qspec.corrQF)
	})
}

func TestCorrectableStream(t *testing.T) {
	run(t, 4, 4, func(ctx context.Context, c *CorrectableTestConfiguration, n, div int) *gorums.Correctable[int] {
		corr := c.CorrectableStream(ctx, &CorrectableRequest{})
		qspec := qspec{div, n}
		return gorums.NewCorrectable(corr, qspec.corrQF)
	})
}

type testSrv struct {
	n int
}

func (srv testSrv) CorrectableStream(_ gorums.ServerCtx, _ *CorrectableRequest, send func(response *CorrectableResponse) error) error {
	for i := range srv.n {
		err := send(CorrectableResponse_builder{Level: int32(i + 1)}.Build())
		if err != nil {
			return err
		}
	}
	return nil
}

func (testSrv) Correctable(_ gorums.ServerCtx, _ *CorrectableRequest) (response *CorrectableResponse, err error) {
	return CorrectableResponse_builder{Level: 1}.Build(), nil
}
