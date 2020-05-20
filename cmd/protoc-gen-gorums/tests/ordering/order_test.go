package ordering

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/relab/gorums/internal/leakcheck"
	"google.golang.org/grpc"
)

type portSupplier struct {
	p int
	sync.Mutex
}

func (p *portSupplier) get() int {
	p.Lock()
	_p := p.p
	p.p++
	p.Unlock()
	return _p
}

var supplier = portSupplier{p: 22332}

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

func (s *testSrv) QC(req *Request) *Response {
	return &Response{
		InOrder: s.isInOrder(req.GetNum()),
	}
}

func (s *testSrv) QCFuture(req *Request) *Response {
	return &Response{
		InOrder: s.isInOrder(req.GetNum()),
	}
}

func (s *testSrv) UnaryRPC(req *Request) *Response {
	return &Response{
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

func getListener() (net.Listener, error) {
	return net.Listen("tcp", fmt.Sprintf(":%d", supplier.get()))
}

func TestUnaryRPCOrdering(t *testing.T) {
	defer leakcheck.Check(t)
	// server setup
	tSrv := &testSrv{}
	srv := NewGorumsServer()
	srv.RegisterUnaryRPCHandler(tSrv)
	lis, err := getListener()
	if err != nil {
		t.Fatalf("Failed to listen on port: %v", err)
	}
	go srv.Serve(lis)

	// client setup
	man, err := NewManager([]string{lis.Addr().String()},
		WithGrpcDialOptions(grpc.WithInsecure()),
		WithDialTimeout(10*time.Second),
	)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	if len(man.Nodes()) < 1 {
		t.Fatalf("No nodes were created!")
	}
	node := man.Nodes()[0]

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

	man.Close()
	srv.Stop()
}

func TestQCOrdering(t *testing.T) {
	defer leakcheck.Check(t)
	// servers setup
	numServers := 4
	srvs := make([]*GorumsServer, numServers)
	addrs := make([]string, numServers)
	for i := 0; i < numServers; i++ {
		srv := NewGorumsServer()
		srv.RegisterQCHandler(&testSrv{})
		lis, err := getListener()
		if err != nil {
			t.Fatalf("Failed to listen on port: %v", err)
		}
		addrs[i] = lis.Addr().String()
		srvs[i] = srv
		go srv.Serve(lis)
	}

	// client setup
	man, err := NewManager(addrs,
		WithGrpcDialOptions(grpc.WithInsecure()),
		WithDialTimeout(10*time.Second),
	)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	c, err := man.NewConfiguration(man.NodeIDs(), &testQSpec{})
	if err != nil {
		t.Fatalf("Failed to create configuration: %v", err)
	}

	// begin test
	numRuns := 1 << 16
	for i := 1; i < numRuns; i++ {
		resp, err := c.QC(context.Background(), &Request{Num: uint64(i)})
		if err != nil {
			t.Fatalf("QC error: %v", err)
		}
		if !resp.GetInOrder() {
			t.Fatalf("Message received out of order.")
		}
	}

	man.Close()
	for _, srv := range srvs {
		srv.Stop()
	}
}

func TestQCFutureOrdering(t *testing.T) {
	defer leakcheck.Check(t)
	// servers setup
	numServers := 4
	srvs := make([]*GorumsServer, numServers)
	addrs := make([]string, numServers)
	for i := 0; i < numServers; i++ {
		srv := NewGorumsServer()
		srv.RegisterQCFutureHandler(&testSrv{})
		lis, err := getListener()
		if err != nil {
			t.Fatalf("Failed to listen on port: %v", err)
		}
		addrs[i] = lis.Addr().String()
		srvs[i] = srv
		go srv.Serve(lis)
	}

	// client setup
	man, err := NewManager(addrs,
		WithGrpcDialOptions(grpc.WithInsecure()),
		WithDialTimeout(10*time.Second),
	)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	c, err := man.NewConfiguration(man.NodeIDs(), &testQSpec{})
	if err != nil {
		t.Fatalf("Failed to create configuration: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// begin test
	var wg sync.WaitGroup
	numRuns := 1 << 16
	for i := 1; i < numRuns; i++ {
		promise, err := c.QCFuture(ctx, &Request{Num: uint64(i)})
		if err != nil {
			t.Fatalf("QC error: %v", err)
		}
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
	man.Close()
	for _, srv := range srvs {
		srv.Stop()
	}
}

func TestMixedOrdering(t *testing.T) {
	defer leakcheck.Check(t)
	// servers setup
	numServers := 4
	srvs := make([]*GorumsServer, numServers)
	addrs := make([]string, numServers)
	for i := 0; i < numServers; i++ {
		srv := NewGorumsServer()
		srv.RegisterQCHandler(&testSrv{})
		srv.RegisterUnaryRPCHandler(&testSrv{})
		lis, err := getListener()
		if err != nil {
			t.Fatalf("Failed to listen on port: %v", err)
		}
		addrs[i] = lis.Addr().String()
		srvs[i] = srv
		go srv.Serve(lis)
	}

	// client setup
	man, err := NewManager(addrs,
		WithGrpcDialOptions(grpc.WithInsecure()),
		WithDialTimeout(10*time.Second),
	)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	c, err := man.NewConfiguration(man.NodeIDs(), &testQSpec{})
	if err != nil {
		t.Fatalf("Failed to create configuration: %v", err)
	}

	nodes := man.Nodes()
	// begin test
	numRuns := 1 << 15
	num := uint64(1)
	for i := 0; i < numRuns; i++ {
		resp, err := c.QC(context.Background(), &Request{Num: num})
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

	man.Close()
	for _, srv := range srvs {
		srv.Stop()
	}
}
