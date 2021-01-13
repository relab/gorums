package ordering

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/internal/leakcheck"
	"google.golang.org/grpc"
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

func (s *testSrv) QC(_ context.Context, req *Request, out func(*Response, error)) {
	out(&Response{
		InOrder: s.isInOrder(req.GetNum()),
	}, nil)
}

func (s *testSrv) QCFuture(_ context.Context, req *Request, out func(*Response, error)) {
	out(&Response{
		InOrder: s.isInOrder(req.GetNum()),
	}, nil)
}

func (s *testSrv) UnaryRPC(_ context.Context, req *Request, out func(*Response, error)) {
	out(&Response{
		InOrder: s.isInOrder(req.GetNum()),
	}, nil)
}

type testQSpec struct {
	quorum int
}

func (q testQSpec) qf(replies map[uint32]*Response) (*Response, bool) {
	if len(replies) < q.quorum {
		return nil, false
	}
	for _, reply := range replies {
		if !reply.InOrder {
			return reply, true
		}
	}
	var reply *Response
	for _, r := range replies {
		reply = r
		break
	}
	return reply, true
}

func (q testQSpec) QCQF(_ *Request, replies map[uint32]*Response) (*Response, bool) {
	return q.qf(replies)
}

func (q testQSpec) QCFutureQF(_ *Request, replies map[uint32]*Response) (*Response, bool) {
	return q.qf(replies)
}

func (q testQSpec) AsyncHandlerQF(_ *Request, replies map[uint32]*Response) (*Response, bool) {
	return q.qf(replies)
}

func setup(t *testing.T, cfgSize int) (cfg *Configuration, teardown func()) {
	t.Helper()
	addrs, closeServers := gorums.TestSetup(t, cfgSize, func() gorums.ServerIface {
		srv := gorums.NewServer()
		RegisterGorumsTestServer(srv, &testSrv{})
		return srv
	})
	mgr, err := NewManager(
		gorums.WithNodeList(addrs),
		gorums.WithDialTimeout(100*time.Millisecond),
		gorums.WithGrpcDialOptions(grpc.WithBlock(), grpc.WithInsecure()),
	)
	if err != nil {
		t.Fatalf("Failed to start manager: %v", err)
	}
	cfg, err = mgr.NewConfiguration(mgr.NodeIDs(), &testQSpec{cfgSize})
	if err != nil {
		t.Fatalf("Failed to create configuration: %v", err)
	}
	teardown = func() {
		mgr.Close()
		closeServers()
	}
	return cfg, teardown
}

func TestUnaryRPCOrdering(t *testing.T) {
	defer leakcheck.Check(t)
	cfg, teardown := setup(t, 1)
	defer teardown()
	node := cfg.Nodes()[0]
	// begin test
	stopTime := time.Now().Add(5 * time.Second)
	i := 1
	for time.Now().Before(stopTime) {
		i++
		resp, err := node.UnaryRPC(context.Background(), &Request{Num: uint64(i)})
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
	defer leakcheck.Check(t)
	cfg, teardown := setup(t, 4)
	defer teardown()
	// begin test
	stopTime := time.Now().Add(5 * time.Second)
	i := 1
	for time.Now().Before(stopTime) {
		i++
		resp, err := cfg.QC(context.Background(), &Request{Num: uint64(i)})
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

func TestQCFutureOrdering(t *testing.T) {
	defer leakcheck.Check(t)
	cfg, teardown := setup(t, 4)
	defer teardown()
	ctx, cancel := context.WithCancel(context.Background())
	// begin test
	var wg sync.WaitGroup
	stopTime := time.Now().Add(5 * time.Second)
	i := 1
	for time.Now().Before(stopTime) {
		i++
		promise := cfg.QCFuture(ctx, &Request{Num: uint64(i)})
		wg.Add(1)
		go func(promise *FutureResponse) {
			defer wg.Done()
			resp, err := promise.Get()
			if err != nil {
				if qcError, ok := err.(gorums.QuorumCallError); ok {
					if qcError.Reason == context.Canceled.Error() {
						return
					}
				}
				t.Errorf("QC error: %v", err)
			}
			if resp == nil {
				t.Errorf("Got nil response")
			}
			if !resp.GetInOrder() && !t.Failed() {
				t.Errorf("Message received out of order.")
			}
		}(promise)
	}
	wg.Wait()
	cancel()
}

func TestMixedOrdering(t *testing.T) {
	defer leakcheck.Check(t)
	cfg, teardown := setup(t, 4)
	defer teardown()
	nodes := cfg.Nodes()
	// begin test
	stopTime := time.Now().Add(5 * time.Second)
	i := 1
	for time.Now().Before(stopTime) {
		resp, err := cfg.QC(context.Background(), &Request{Num: uint64(i)})
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
				resp, err := node.UnaryRPC(context.Background(), &Request{Num: uint64(i)})
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
