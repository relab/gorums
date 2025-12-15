package ordering

import (
	"context"
	"errors"
	"iter"
	"sync"
	"testing"
	"time"

	"github.com/relab/gorums"
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

func (s *testSrv) isInOrder(num uint64) bool {
	s.Lock()
	defer s.Unlock()
	if num > s.lastNum {
		s.lastNum = num
		return true
	}
	return false
}

func (s *testSrv) QuorumCall(_ gorums.ServerCtx, req *Request) (resp *Response, err error) {
	return Response_builder{
		InOrder: s.isInOrder(req.GetNum()),
	}.Build(), nil
}

func (s *testSrv) UnaryRPC(_ gorums.ServerCtx, req *Request) (resp *Response, err error) {
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
	node := gorums.TestNode(t, serverFn)

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
	config := gorums.TestConfiguration(t, 4, serverFn)
	cfgCtx := config.Context(t.Context())

	for i := range iterations() {
		// Use CollectAll to get all responses and check for ordering
		responses := QuorumCall(cfgCtx, Request_builder{Num: uint64(i)}.Build())
		replies := responses.CollectAll()
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
	config := gorums.TestConfiguration(t, 4, serverFn)
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
	config := gorums.TestConfiguration(t, 4, serverFn)
	cfgCtx := config.Context(t.Context())

	for i := range iterations() {
		// Use CollectAll to get all responses and check for ordering
		responses := QuorumCall(cfgCtx, Request_builder{Num: uint64(2*i - 1)}.Build())
		replies := responses.CollectAll()
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
