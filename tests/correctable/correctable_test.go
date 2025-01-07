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
func run(t *testing.T, n int, div int, corr func(context.Context, *Configuration) *gorums.Correctable) {
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

	cfg, err := mgr.NewConfiguration(qspec{div, n}, gorums.WithNodeList(addrs))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	res := corr(ctx, cfg)

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

func TestCorrectable(t *testing.T) {
	run(t, 4, 1, func(ctx context.Context, c *Configuration) *gorums.Correctable {
		corr := c.Correctable(ctx, &CorrectableRequest{})
		return corr.Correctable
	})
}

func TestCorrectableStream(t *testing.T) {
	run(t, 4, 4, func(ctx context.Context, c *Configuration) *gorums.Correctable {
		corr := c.CorrectableStream(ctx, &CorrectableRequest{})
		return corr.Correctable
	})
}

type qspec struct {
	div       int
	doneLevel int
}

func (q qspec) q(replies map[uint32]*CorrectableResponse) (*CorrectableResponse, int, bool) {
	sum := 0
	for _, reply := range replies {
		sum += int(reply.Level)
	}
	level := sum / q.div
	return &CorrectableResponse{Level: int32(level)}, level, level >= q.doneLevel
}

// CorrectableStreamQF is the quorum function for the CorrectableStream
// correctable stream quorum call method. The in parameter is the request object
// supplied to the CorrectableStream method at call time, and may or may not
// be used by the quorum function. If the in parameter is not needed
// you should implement your quorum function with '_ *CorrectableRequest'.
func (q qspec) CorrectableStreamQF(_ *CorrectableRequest, replies map[uint32]*CorrectableResponse) (*CorrectableResponse, int, bool) {
	return q.q(replies)
}

// CorrectableQF is the quorum function for the Correctable
// correctable quorum call method. The in parameter is the request object
// supplied to the Correctable method at call time, and may or may not
// be used by the quorum function. If the in parameter is not needed
// you should implement your quorum function with '_ *CorrectableRequest'.
func (q qspec) CorrectableQF(_ *CorrectableRequest, replies map[uint32]*CorrectableResponse) (*CorrectableResponse, int, bool) {
	return q.q(replies)
}

type testSrv struct {
	n int
}

func (srv testSrv) CorrectableStream(_ gorums.ServerCtx, request *CorrectableRequest, send func(response *CorrectableResponse) error) error {
	for i := 0; i < srv.n; i++ {
		err := send(&CorrectableResponse{Level: int32(i + 1)})
		if err != nil {
			return err
		}
	}
	return nil
}

func (srv testSrv) Correctable(_ gorums.ServerCtx, request *CorrectableRequest) (response *CorrectableResponse, err error) {
	return &CorrectableResponse{Level: 1}, nil
}
