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

func orderQF(ctx context.Context, responses gorums.Responses[*Response], quorum int) (*Response, error) {
	replyCount := 0
	var orderMsg *Response
	var firstMsg *Response
	for response := range responses {
		msg, err, _ := response.Unpack()
		if err != nil {
			return nil, err
		}
		replyCount++
		if firstMsg == nil {
			firstMsg = msg
		}
		if orderMsg == nil && !msg.GetInOrder() {
			orderMsg = msg
		}

		if replyCount < quorum {
			continue
		}
		if orderMsg != nil {
			return orderMsg, nil
		}
		return firstMsg, nil
	}
	err := ctx.Err()
	if err != nil {
		return nil, err
	}
	return nil, errors.New("orderQF: quorum not found")
}

func setup(t *testing.T, cfgSize int) (cfg *Configuration, teardown func()) {
	t.Helper()
	addrs, closeServers := gorums.TestSetup(t, cfgSize, func(_ int) gorums.ServerIface {
		srv := gorums.NewServer()
		RegisterGorumsTestServer(srv, &testSrv{})
		return srv
	})
	mgr := NewManager(
		gorums.WithGrpcDialOptions(
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	)
	cfg, err := mgr.NewConfiguration(gorums.WithNodeList(addrs))
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
	node := cfg.Nodes()[0]
	// begin test
	stopTime := time.Now().Add(5 * time.Second)
	i := 1
	for time.Now().Before(stopTime) {
		i++
		resp, err := node.UnaryRPC(context.Background(), Request_builder{Num: uint64(i)}.Build())
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
	stopTime := time.Now().Add(5 * time.Second)
	i := 1
	for time.Now().Before(stopTime) {
		i++
		ctx := context.Background()
		req := Request_builder{Num: uint64(i)}.Build()
		resp, err := orderQF(ctx, cfg.QC(ctx, req), 4)
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
	// begin test
	var wg sync.WaitGroup
	stopTime := time.Now().Add(5 * time.Second)
	i := 1
	for time.Now().Before(stopTime) {
		i++
		responses := cfg.QCAsync(ctx, Request_builder{Num: uint64(i)}.Build())
		wg.Add(1)
		go func(responses gorums.Responses[*Response]) {
			defer wg.Done()
			resp, err := orderQF(ctx, responses, 4)
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
		}(responses)
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
	stopTime := time.Now().Add(5 * time.Second)
	i := 1
	for time.Now().Before(stopTime) {
		req := Request_builder{Num: uint64(i)}.Build()
		ctx := context.Background()
		resp, err := orderQF(ctx, cfg.QC(ctx, req), 4)
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
		wg.Add(len(nodes))
		for _, node := range nodes {
			go func(node *Node) {
				defer wg.Done()
				resp, err := node.UnaryRPC(context.Background(), Request_builder{Num: uint64(i)}.Build())
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
			}(node)
		}
		wg.Wait()
		i++
	}
}
