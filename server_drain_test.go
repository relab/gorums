package gorums_test

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/gorumstest"
	"github.com/relab/gorums/internal/testutils/mock"
	"github.com/relab/gorums/internal/testutils/mockcall"
	"google.golang.org/protobuf/proto"
)

// TestServerDrainFinishesAcceptedOneWayHandlers verifies that GracefulStop
// waits for accepted one-way requests: one running handler and one queued
// behind it. Neither has a pending reply entry on the client.
func TestServerDrainFinishesAcceptedOneWayHandlers(t *testing.T) {
	gate := make(chan struct{})
	started := make(chan struct{}, 2)
	var finished atomic.Int32
	srv := gorums.NewServer()
	srv.RegisterHandler(mock.TestMethod, func(_ gorums.ServerContext, _ *gorums.Message, _ func(proto.Message)) error {
		started <- struct{}{}
		<-gate
		finished.Add(1)
		return nil
	})
	node := gorumstest.Node(t, func(int) gorumstest.ServerIface { return srv })

	for range 2 {
		ctx := gorumstest.Context(t, 2*time.Second)
		if err := mockcall.Unicast(ctx, node, "x").Send(); err != nil {
			t.Fatalf("Unicast: %v", err)
		}
	}
	<-started // the first handler runs; the second is queued behind it

	stopped := make(chan struct{})
	go func() {
		srv.GracefulStop()
		close(stopped)
	}()

	select {
	case <-stopped:
		t.Fatal("GracefulStop returned while accepted one-way handlers were blocked")
	case <-time.After(300 * time.Millisecond):
	}

	close(gate)
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("GracefulStop did not return after the handlers finished")
	}
	if got := finished.Load(); got != 2 {
		t.Errorf("finished handlers = %d, want 2", got)
	}
}

// TestServerDrainRejectsNewSends verifies that a sender that learned of the
// drain fails further requests with ErrServerDraining.
func TestServerDrainRejectsNewSends(t *testing.T) {
	gate := make(chan struct{})
	started := make(chan struct{}, 1)
	srv := gorums.NewServer()
	srv.RegisterHandler(mock.TestMethod, func(_ gorums.ServerContext, _ *gorums.Message, _ func(proto.Message)) error {
		started <- struct{}{}
		<-gate
		return nil
	})
	node := gorumstest.Node(t, func(int) gorumstest.ServerIface { return srv })

	ctx := gorumstest.Context(t, 2*time.Second)
	if err := mockcall.Unicast(ctx, node, "x").Send(); err != nil {
		t.Fatalf("Unicast: %v", err)
	}
	<-started

	stopped := make(chan struct{})
	go func() {
		srv.GracefulStop()
		close(stopped)
	}()
	t.Cleanup(func() {
		close(gate)
		<-stopped
	})

	deadline := time.Now().Add(3 * time.Second)
	for {
		ctx := gorumstest.Context(t, time.Second)
		err := mockcall.Unicast(ctx, node, "y").Send()
		if errors.Is(err, gorums.ErrServerDraining) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("Unicast after drain began: err = %v, want ErrServerDraining", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestServerDrainDeadline verifies that the drain deadline bounds
// GracefulStop when an accepted handler never finishes.
func TestServerDrainDeadline(t *testing.T) {
	gate := make(chan struct{})
	started := make(chan struct{}, 1)
	srv := gorums.NewServer(gorums.WithDrainTimeout(200 * time.Millisecond))
	srv.RegisterHandler(mock.TestMethod, func(_ gorums.ServerContext, _ *gorums.Message, _ func(proto.Message)) error {
		started <- struct{}{}
		<-gate
		return nil
	})
	node := gorumstest.Node(t, func(int) gorumstest.ServerIface { return srv })
	t.Cleanup(func() { close(gate) })

	ctx := gorumstest.Context(t, 2*time.Second)
	if err := mockcall.Unicast(ctx, node, "x").Send(); err != nil {
		t.Fatalf("Unicast: %v", err)
	}
	<-started

	stopped := make(chan struct{})
	go func() {
		srv.GracefulStop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("GracefulStop was not bounded by the drain timeout")
	}
}
