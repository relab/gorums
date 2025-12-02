package ordering

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/relab/gorums"
	"go.uber.org/goleak"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// The duration that each ordering test will run
const testDuration = 5 * time.Second

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

func setup(t *testing.T, cfgSize int) (cfg gorums.Configuration, teardown func()) {
	t.Helper()
	addrs, closeServers := gorums.TestSetup(t, cfgSize, func(_ int) gorums.ServerIface {
		srv := gorums.NewServer()
		RegisterGorumsTestServer(srv, &testSrv{})
		return srv
	})
	mgr := gorums.NewManager(
		gorums.WithGrpcDialOptions(
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	)
	cfg, err := gorums.NewConfiguration(mgr, gorums.WithNodeList(addrs))
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

	stopTime := time.Now().Add(testDuration)
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

	cfgCtx := gorums.WithConfigContext(context.Background(), cfg)
	stopTime := time.Now().Add(testDuration)
	i := 1
	for time.Now().Before(stopTime) {
		i++
		// Use CollectAll to get all responses and check for ordering
		responses := QC(cfgCtx, Request_builder{Num: uint64(i)}.Build())
		replies := responses.CollectAll()
		if len(replies) < cfg.Size() {
			t.Fatalf("incomplete call: %d replies", len(replies))
		}
		for _, reply := range replies {
			if !reply.GetInOrder() {
				t.Fatalf("Message received out of order.")
			}
		}
	}
}

func TestQCAsyncOrdering(t *testing.T) {
	defer goleak.VerifyNone(t)
	cfg, teardown := setup(t, 4)
	defer teardown()
	ctx, cancel := context.WithCancel(context.Background())

	cfgCtx := gorums.WithConfigContext(ctx, cfg)
	var wg sync.WaitGroup
	stopTime := time.Now().Add(testDuration)
	i := 1
	for time.Now().Before(stopTime) {
		i++
		// QCAsync returns an Async future that uses majority quorum by default
		promise := QCAsync(cfgCtx, Request_builder{Num: uint64(i)}.Build())
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

	cfgCtx := gorums.WithConfigContext(context.Background(), cfg)
	stopTime := time.Now().Add(testDuration)
	i := 1
	for time.Now().Before(stopTime) {
		// Use CollectAll to get all responses and check for ordering
		responses := QC(cfgCtx, Request_builder{Num: uint64(i)}.Build())
		replies := responses.CollectAll()
		i++
		if len(replies) < cfg.Size() {
			t.Fatalf("incomplete call: %d replies", len(replies))
		}
		for _, reply := range replies {
			if !reply.GetInOrder() {
				t.Fatalf("Message received out of order.")
			}
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
