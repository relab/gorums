package ordering

import (
	"context"
	"errors"
	"iter"
	"sync"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/gorumstest"
)

// stressMode controls whether tests run in stress mode (time-based) or normal mode (iteration-based).
// This is set to true in order_stress_test.go via the stress build tag.
var stressMode = false

// testIterations is the number of iterations for ordering tests in normal mode.
const testIterations = 100

// stressDuration is the duration for stress tests when stressMode is true.
const stressDuration = 5 * time.Second

// iterations returns an iterator that yields sequential integers starting from 1.
// In normal mode, it yields testIterations values.
// In stress mode, it yields values for stressDuration.
func iterations() iter.Seq[int] {
	if stressMode {
		return func(yield func(int) bool) {
			stopTime := time.Now().Add(stressDuration)
			for i := 1; time.Now().Before(stopTime); i++ {
				if !yield(i) {
					return
				}
			}
		}
	}
	return func(yield func(int) bool) {
		for i := range testIterations {
			if !yield(i + 1) {
				return
			}
		}
	}
}

type testSrv struct {
	sync.Mutex
	lastNum uint64
}

type blockingOrderSrv struct {
	firstStarted  chan struct{}
	secondStarted chan struct{}
	releaseFirst  chan struct{}
}

func newBlockingOrderSrv() *blockingOrderSrv {
	return &blockingOrderSrv{
		firstStarted:  make(chan struct{}),
		secondStarted: make(chan struct{}),
		releaseFirst:  make(chan struct{}),
	}
}

func (s *blockingOrderSrv) handle(req *Request) (*Response, error) {
	switch req.GetNum() {
	case 1:
		close(s.firstStarted)
		<-s.releaseFirst
	case 2:
		close(s.secondStarted)
	}
	return Response_builder{InOrder: true}.Build(), nil
}

func (s *blockingOrderSrv) QuorumCall(_ gorums.ServerContext, req *Request) (*Response, error) {
	return s.handle(req)
}

func (s *blockingOrderSrv) UnaryRPC(_ gorums.ServerContext, req *Request) (*Response, error) {
	return s.handle(req)
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

func (s *testSrv) QuorumCall(_ gorums.ServerContext, req *Request) (resp *Response, err error) {
	return Response_builder{
		InOrder: s.isInOrder(req.GetNum()),
	}.Build(), nil
}

func (s *testSrv) UnaryRPC(_ gorums.ServerContext, req *Request) (resp *Response, err error) {
	return Response_builder{
		InOrder: s.isInOrder(req.GetNum()),
	}.Build(), nil
}

// serverFn creates a new server with an independent testSrv instance.
// Each server needs its own testSrv to track ordering independently.
func serverFn(_ int) gorums.ServerIface {
	srv := gorums.NewServer()
	RegisterGorumsTestServer(srv, &testSrv{})
	return srv
}

func TestUnaryRPCOrdering(t *testing.T) {
	node := gorumstest.Node(t, serverFn)

	for i := range iterations() {
		nodeCtx := node.Context(t.Context())
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

func TestQuorumCallOrdering(t *testing.T) {
	config := gorumstest.Config(t, 4, serverFn)
	cfgCtx := config.Context(t.Context())

	for i := range iterations() {
		// Use CollectAll to get all responses and check for ordering
		responses := QuorumCall(cfgCtx, Request_builder{Num: uint64(i)}.Build())
		replies := responses.Results().CollectAll()
		if len(replies) < config.Size() {
			t.Fatalf("incomplete call: %d replies", len(replies))
		}
		for _, reply := range replies {
			if !reply.GetInOrder() {
				t.Fatalf("Message received out of order.")
			}
		}
	}
}

func TestQuorumCallAsyncOrdering(t *testing.T) {
	config := gorumstest.Config(t, 4, serverFn)
	cfgCtx := config.Context(t.Context())

	var wg sync.WaitGroup
	for i := range iterations() {
		// QuorumCall returns Responses; use .AsyncMajority() to get an Async future
		promise := QuorumCall(cfgCtx, Request_builder{Num: uint64(i)}.Build()).AsyncMajority()
		wg.Go(func() {
			resp, err := promise.Get()
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				t.Errorf("QuorumCall error: %v", err)
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
}

func TestMixedOrdering(t *testing.T) {
	config := gorumstest.Config(t, 4, serverFn)
	cfgCtx := config.Context(t.Context())

	for i := range iterations() {
		// Use CollectAll to get all responses and check for ordering
		responses := QuorumCall(cfgCtx, Request_builder{Num: uint64(2*i - 1)}.Build())
		replies := responses.Results().CollectAll()
		if len(replies) < config.Size() {
			t.Fatalf("incomplete call: %d replies", len(replies))
		}
		for _, reply := range replies {
			if !reply.GetInOrder() {
				t.Fatalf("Message received out of order.")
			}
		}
		var wg sync.WaitGroup
		for _, node := range config.Nodes() {
			wg.Go(func() {
				nodeCtx := node.Context(t.Context())
				resp, err := UnaryRPC(nodeCtx, Request_builder{Num: uint64(2 * i)}.Build())
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
	}
}

func TestDedupBackChannelOrdering(t *testing.T) {
	servers := gorumstest.LocalServers(t, 2, gorums.WithStreamDedup())
	handler := newBlockingOrderSrv()
	var releaseOnce sync.Once
	releaseFirst := func() { releaseOnce.Do(func() { close(handler.releaseFirst) }) }
	t.Cleanup(releaseFirst)
	RegisterGorumsTestServer(servers[0], handler)

	ctx := gorumstest.Context(t, 10*time.Second)
	for _, srv := range servers {
		if _, err := srv.WaitForAll(ctx); err != nil {
			t.Fatalf("WaitForAll: %v", err)
		}
	}

	var target *gorums.Node
	for _, node := range servers[1].PeerConfig() {
		if node.ID() == 1 {
			target = node
			break
		}
	}
	if target == nil {
		t.Fatal("node 1 not found in server 2 outbound configuration")
	}
	if !target.IsShared() {
		t.Fatal("node 1 is not using the deduplicated stream")
	}

	call := func(num uint64, done chan<- error) {
		resp, err := UnaryRPC(target.Context(ctx), Request_builder{Num: num}.Build())
		if err == nil && !resp.GetInOrder() {
			err = errors.New("handler reported an out-of-order request")
		}
		done <- err
	}
	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)
	go call(1, firstDone)
	select {
	case <-handler.firstStarted:
	case <-time.After(time.Second):
		t.Fatal("first handler did not start")
	}

	go call(2, secondDone)
	select {
	case <-handler.secondStarted:
		t.Fatal("second handler started before first handler released")
	case <-time.After(50 * time.Millisecond):
	}

	releaseFirst()
	select {
	case <-handler.secondStarted:
	case <-time.After(time.Second):
		t.Fatal("second handler did not start after first handler released")
	}
	for i, done := range []<-chan error{firstDone, secondDone} {
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("call %d failed: %v", i+1, err)
			}
		case <-time.After(time.Second):
			t.Fatalf("call %d did not complete", i+1)
		}
	}
}
