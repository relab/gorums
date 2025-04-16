package correctable

import (
	"context"
	"iter"
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
func run(t *testing.T, n int, div int, corr func(context.Context, *Configuration, int, int) *gorums.Correctable[int]) {
	addrs, teardown := gorums.TestSetup(t, n, func(i int) gorums.ServerIface {
		gorumsSrv := gorums.NewServer()
		RegisterCorrectableTestServer(gorumsSrv, &testSrv{n})
		return gorumsSrv
	})
	defer teardown()

	mgr := NewManager(
		gorums.WithGrpcDialOptions(
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	)

	cfg, err := mgr.NewConfiguration(gorums.WithNodeList(addrs))
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

func qcorr(responses gorums.Iterator[*CorrectableResponse], q qspec) iter.Seq2[int, int] {
	return func(yield func(int, int) bool) {
		sum := 0
		for response := range responses {
			reply, err, _ := response.Unpack()
			if err != nil {
				continue
			}
			sum += int(reply.GetLevel())
			level := sum / q.div
			if !yield(level, level) {
				return
			}
			if level >= q.doneLevel {
				return
			}
		}
	}
}

func TestCorrectable(t *testing.T) {
	run(t, 4, 1, func(ctx context.Context, c *Configuration, n int, div int) *gorums.Correctable[int] {
		corr := c.Correctable(ctx, &CorrectableRequest{})
		return gorums.IterToCorrectable(corr, qspec{div, n}, qcorr)
	})
}

func TestCorrectableStream(t *testing.T) {
	run(t, 4, 4, func(ctx context.Context, c *Configuration, n int, div int) *gorums.Correctable[int] {
		corr := c.CorrectableStream(ctx, &CorrectableRequest{})
		return gorums.IterToCorrectable(corr, qspec{div, n}, qcorr)
	})
}

type qspec struct {
	div       int
	doneLevel int
}

type testSrv struct {
	n int
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

func (srv testSrv) Correctable(_ gorums.ServerCtx, request *CorrectableRequest) (response *CorrectableResponse, err error) {
	return CorrectableResponse_builder{Level: 1}.Build(), nil
}
