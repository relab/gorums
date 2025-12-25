package gorums_test

import (
	"errors"
	"net"
	"testing"
	"time"

	"github.com/relab/gorums"
)

type mockCloser struct {
	closed bool
	err    error
}

func (m *mockCloser) Close() error {
	m.closed = true
	return m.err
}

func TestSystemLifecycle(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	sys := gorums.NewSystem(lis)
	if sys == nil {
		t.Fatal("NewSystem returned nil")
	}

	closer1 := &mockCloser{}
	closer2 := &mockCloser{}

	sys.RegisterService(closer1, func(srv *gorums.Server) {
		// In a real scenario, we would register a Gorums service here.
	})
	sys.RegisterService(closer2, func(srv *gorums.Server) {
		// Register another service or just use the callback.
	})

	go func() {
		// Serve acts as a blocking call, so run in goroutine
		if err := sys.Serve(); err != nil {
			// Serve returns error on Stop usually (or net closed)
			t.Logf("Serve returned: %v", err)
		}
	}()

	// Give it a moment to start
	time.Sleep(10 * time.Millisecond)

	// Stop the system
	if err := sys.Stop(); err != nil {
		t.Errorf("Stop returned error: %v", err)
	}

	if !closer1.closed {
		t.Error("closer1 was not closed")
	}
	if !closer2.closed {
		t.Error("closer2 was not closed")
	}
}

func TestSystemStopError(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	sys := gorums.NewSystem(lis)
	errCloser := &mockCloser{err: errors.New("closer error")}

	sys.RegisterService(errCloser, func(srv *gorums.Server) {})

	go func() {
		_ = sys.Serve()
	}()
	time.Sleep(10 * time.Millisecond)

	err = sys.Stop()
	if err == nil {
		t.Error("expected error from Stop, got nil")
	}
}
