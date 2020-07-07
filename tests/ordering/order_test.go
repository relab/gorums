package ordering

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/internal/leakcheck"
	"google.golang.org/grpc"
)

type portSupplier struct {
	p int
	sync.Mutex
}

func (p *portSupplier) get() int {
	p.Lock()
	newPort := p.p
	p.p++
	p.Unlock()
	return newPort
}

var supplier = portSupplier{p: 22332}

func getListener() (net.Listener, error) {
	return net.Listen("tcp", fmt.Sprintf(":%d", supplier.get()))
}

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

func (s *testSrv) QC(req *Request, c chan<- *Response) {
	c <- &Response{
		InOrder: s.isInOrder(req.GetNum()),
	}
}

func (s *testSrv) QCFuture(req *Request, c chan<- *Response) {
	c <- &Response{
		InOrder: s.isInOrder(req.GetNum()),
	}
}

func (s *testSrv) UnaryRPC(req *Request, c chan<- *Response) {
	c <- &Response{
		InOrder: s.isInOrder(req.GetNum()),
	}
}

type testQSpec struct {
	quorum int
}

func (q testQSpec) qf(replies []*Response) (*Response, bool) {
	if len(replies) < q.quorum {
		return nil, false
	}
	for _, reply := range replies {
		if !reply.InOrder {
			return reply, true
		}
	}
	return replies[0], true
}

func (q testQSpec) QCQF(_ *Request, replies []*Response) (*Response, bool) {
	return q.qf(replies)
}

func (q testQSpec) QCFutureQF(_ *Request, replies []*Response) (*Response, bool) {
	return q.qf(replies)
}

func (q testQSpec) AsyncHandlerQF(_ *Request, replies []*Response) (*Response, bool) {
	return q.qf(replies)
}

func setup(t *testing.T, cfgSize int) (cfg *Configuration, teardown func()) {
	t.Helper()
	addrs, closeServers := gorums.TestSetup(t, cfgSize, func() interface{} {
		srv := NewGorumsServer()
		srv.RegisterGorumsTestServer(&testSrv{})
		return srv
	})
	mgr, err := NewManager(addrs, WithDialTimeout(100*time.Millisecond), WithGrpcDialOptions(
		grpc.WithBlock(), grpc.WithInsecure(),
	))
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
	numRuns := 1 << 16
	for i := 1; i < numRuns; i++ {
		resp, err := node.UnaryRPC(context.Background(), &Request{Num: uint64(i)})
		if err != nil {
			t.Fatalf("RPC error: %v", err)
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
	numRuns := 1 << 16
	for i := 1; i < numRuns; i++ {
		resp, err := cfg.QC(context.Background(), &Request{Num: uint64(i)})
		if err != nil {
			t.Fatalf("QC error: %v", err)
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
	numRuns := 1 << 16
	for i := 1; i < numRuns; i++ {
		promise := cfg.QCFuture(ctx, &Request{Num: uint64(i)})
		wg.Add(1)
		go func(promise *FutureResponse) {
			defer wg.Done()
			resp, err := promise.Get()
			if err != nil {
				if qcError, ok := err.(QuorumCallError); ok {
					if qcError.Reason == context.Canceled.Error() {
						return
					}
				}
				t.Errorf("QC error: %v", err)
			}
			if !resp.GetInOrder() {
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
	numRuns := 1 << 15
	num := uint64(1)
	for i := 0; i < numRuns; i++ {
		resp, err := cfg.QC(context.Background(), &Request{Num: num})
		num++
		if err != nil {
			t.Fatalf("QC error: %v", err)
		}
		if !resp.GetInOrder() {
			t.Fatalf("Message received out of order.")
		}
		var wg sync.WaitGroup
		wg.Add(len(nodes))
		for _, node := range nodes {
			go func(node *Node) {
				defer wg.Done()
				resp, err := node.UnaryRPC(context.Background(), &Request{Num: num})
				if err != nil {
					t.Errorf("RPC error: %v", err)
					return
				}
				if !resp.GetInOrder() {
					t.Errorf("Message received out of order.")
					return
				}
			}(node)
		}
		wg.Wait()
		num++
	}
}
