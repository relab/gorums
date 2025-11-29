package ordering

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/relab/gorums"
	"go.uber.org/goleak"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type testSrv struct {
	sync.Mutex
	lastNum uint64
}

func (s *testSrv) isInOrder(num uint64) bool {
	s.Lock()
	defer s.Unlock()
	if num > s.lastNum {
		s.lastNum = num
		return true
	}
	return false
}

func (s *testSrv) QC(_ gorums.ServerCtx, req *Request) (resp *Response, err error) {
	return Response_builder{
		InOrder: s.isInOrder(req.GetNum()),
	}.Build(), nil
}

func (s *testSrv) QCAsync(_ gorums.ServerCtx, req *Request) (resp *Response, err error) {
	return Response_builder{
		InOrder: s.isInOrder(req.GetNum()),
	}.Build(), nil
}

func (s *testSrv) UnaryRPC(_ gorums.ServerCtx, req *Request) (resp *Response, err error) {
	return Response_builder{
		InOrder: s.isInOrder(req.GetNum()),
	}.Build(), nil
}

func setup(t *testing.T, cfgSize int) (cfg gorums.RawConfiguration, teardown func()) {
	t.Helper()
	addrs, closeServers := gorums.TestSetup(t, cfgSize, func(_ int) gorums.ServerIface {
		srv := gorums.NewServer()
		RegisterGorumsTestServer(srv, &testSrv{})
		return srv
	})
	mgr := gorums.NewRawManager(
		gorums.WithGrpcDialOptions(
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	)
	cfg, err := gorums.NewRawConfiguration(mgr, gorums.WithNodeList(addrs))
	if err != nil {
		t.Fatal(err)
	}
	teardown = func() {
		mgr.Close()
		closeServers()
	}
	return cfg, teardown
}

func TestUnaryRPCOrdering(t *testing.T) {
	defer goleak.VerifyNone(t)
	cfg, teardown := setup(t, 1)
	defer teardown()
	node := cfg[0]
	// begin test
	stopTime := time.Now().Add(5 * time.Second)
	i := 1
	for time.Now().Before(stopTime) {
		i++
		nodeCtx := gorums.WithNodeContext(context.Background(), node)
		resp, err := UnaryRPC(nodeCtx, Request_builder{Num: uint64(i)}.Build())
		if err != nil {
			t.Fatalf("RPC error: %v", err)
		}
		if resp == nil {
			t.Fatal("Got nil response")
		}
		if !resp.GetInOrder() {
			t.Fatalf("Message received out of order.")
		}
	}
}

func TestQCOrdering(t *testing.T) {
	defer goleak.VerifyNone(t)
	cfg, teardown := setup(t, 4)
	defer teardown()
	// begin test
	qf := func(ctx *gorums.ClientCtx[*Request, *Response]) (*Response, error) {
		replies := ctx.Responses().IgnoreErrors().CollectAll()
		if len(replies) < ctx.Size() {
			return nil, fmt.Errorf("incomplete call: %d replies", len(replies))
		}
		for _, reply := range replies {
			if !reply.GetInOrder() {
				return reply, nil
			}
		}
		var reply *Response
		for _, r := range replies {
			reply = r
			break
		}
		return reply, nil
	}
	cfgCtx := gorums.WithConfigContext(context.Background(), cfg)
	stopTime := time.Now().Add(5 * time.Second)
	i := 1
	for time.Now().Before(stopTime) {
		i++
		resp, err := QC(cfgCtx,
			Request_builder{Num: uint64(i)}.Build(),
			gorums.WithQuorumFunc(qf))
		if err != nil {
			t.Fatalf("QC error: %v", err)
		}
		if resp == nil {
			t.Fatal("Got nil response")
		}
		if !resp.GetInOrder() {
			t.Fatalf("Message received out of order.")
		}
	}
}

func TestQCAsyncOrdering(t *testing.T) {
	defer goleak.VerifyNone(t)
	cfg, teardown := setup(t, 4)
	defer teardown()
	ctx, cancel := context.WithCancel(context.Background())
	cfgCtx := gorums.WithConfigContext(ctx, cfg)
	qf := func(ctx *gorums.ClientCtx[*Request, *Response]) (*Response, error) {
		replies := ctx.Responses().IgnoreErrors().CollectAll()
		if len(replies) < ctx.Size() {
			return nil, fmt.Errorf("incomplete call: %d replies", len(replies))
		}
		for _, reply := range replies {
			if !reply.GetInOrder() {
				return reply, nil
			}
		}
		var reply *Response
		for _, r := range replies {
			reply = r
			break
		}
		return reply, nil
	}
	// begin test
	var wg sync.WaitGroup
	stopTime := time.Now().Add(5 * time.Second)
	i := 1
	for time.Now().Before(stopTime) {
		i++
		promise := QCAsync(cfgCtx, Request_builder{Num: uint64(i)}.Build(), gorums.WithQuorumFunc(qf))
		wg.Go(func() {
			resp, err := promise.Get()
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				t.Errorf("QC error: %v", err)
			}
			if resp == nil {
				t.Errorf("Got nil response")
			}
			if !resp.GetInOrder() && !t.Failed() {
				t.Errorf("Message received out of order.")
			}
		})
	}
	wg.Wait()
	cancel()
}

func TestMixedOrdering(t *testing.T) {
	defer goleak.VerifyNone(t)
	cfg, teardown := setup(t, 4)
	defer teardown()
	nodes := cfg.Nodes()
	// begin test
	qf := func(ctx *gorums.ClientCtx[*Request, *Response]) (*Response, error) {
		replies := ctx.Responses().IgnoreErrors().CollectAll()
		if len(replies) < ctx.Size() {
			return nil, fmt.Errorf("incomplete call: %d replies", len(replies))
		}
		for _, reply := range replies {
			if !reply.GetInOrder() {
				return reply, nil
			}
		}
		var reply *Response
		for _, r := range replies {
			reply = r
			break
		}
		return reply, nil
	}
	cfgCtx := gorums.WithConfigContext(context.Background(), cfg)
	stopTime := time.Now().Add(5 * time.Second)
	i := 1
	for time.Now().Before(stopTime) {
		resp, err := QC(cfgCtx,
			Request_builder{Num: uint64(i)}.Build(),
			gorums.WithQuorumFunc(qf))
		i++
		if err != nil {
			t.Fatalf("QC error: %v", err)
		}
		if resp == nil {
			t.Fatal("Got nil response")
		}
		if !resp.GetInOrder() {
			t.Fatalf("Message received out of order.")
		}
		var wg sync.WaitGroup
		for _, node := range nodes {
			wg.Go(func() {
				nodeCtx := gorums.WithNodeContext(context.Background(), node)
				resp, err := UnaryRPC(nodeCtx, Request_builder{Num: uint64(i)}.Build())
				if err != nil {
					t.Errorf("RPC error: %v", err)
					return
				}
				if resp == nil {
					t.Error("Got nil response")
				}
				if !resp.GetInOrder() {
					t.Errorf("Message received out of order.")
					return
				}
			})
		}
		wg.Wait()
		i++
	}
}
