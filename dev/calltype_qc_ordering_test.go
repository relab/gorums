// +build order

package dev_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	qc "github.com/relab/gorums/dev"
	"github.com/relab/gorums/internal/leakcheck"
	"golang.org/x/net/context"
)

func TestQuorumCallOrdering(t *testing.T) {
	defer leakcheck.Check(t)
	failMsg := make(chan string)
	servers, dialOpts, stopGrpcServe, closeListeners := setup(
		t,
		[]storageServer{
			{impl: newStorageServerRequiringOrdering(failMsg)},
			{impl: newStorageServerRequiringOrdering(failMsg)},
			{impl: newStorageServerRequiringOrdering(failMsg)},
			{impl: newStorageServerRequiringOrdering(failMsg)},
			{impl: newStorageServerRequiringOrdering(failMsg)},
		},
		false,
		false,
	)
	defer closeListeners(allServers)
	defer stopGrpcServe(allServers)

	mgr, err := qc.NewManager(servers.addrs(), dialOpts, qc.WithDialTimeout(time.Second))
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer mgr.Close()

	ids := mgr.NodeIDs()

	// Quorum spec: rq=1 (not used), wq=3, n=5.
	qspec := NewStorageQSpec(1, 3)

	config, err := mgr.NewConfiguration(ids, qspec)
	if err != nil {
		t.Fatalf("error creating config: %v", err)
	}

	state := &qc.State{
		Value:     "42",
		Timestamp: 1, // Servers start at 0.
	}

	const maxNumberOfQCs = 1 << 18

	// Test description:
	//
	// Sequentially call Write with an increasing timestamp.
	//
	// n=5, wq=3 -> Write QC will return after three replies.
	//
	// Do _not_ cancel context to allow the two remaining goroutines per
	// call to be handled on the servers.
	//
	// Servers will report error if a message received out-of-order.
	for i := 1; i < maxNumberOfQCs; i++ {
		select {
		case err := <-failMsg:
			t.Fatalf(err)
		default:
			_, err := config.Write(context.Background(), state)
			if err != nil {
				t.Fatalf("write quorum call error: %v", err)
			}
			state.Timestamp++
		}
	}
}

// StorageServerRequiringOrdering represents a single state storage that
// requires strict ordering of received messages.
type storageServerRequiringOrdering struct {
	mu        sync.Mutex
	state     qc.State
	writeResp qc.WriteResponse
	failMsg   chan string
}

// NewStorageBasic returns a new basic storage server.
func newStorageServerRequiringOrdering(failMsg chan string) *storageServerRequiringOrdering {
	return &storageServerRequiringOrdering{failMsg: failMsg}
}

// Read implements the Read method.
func (s *storageServerRequiringOrdering) Read(ctx context.Context, rq *qc.ReadRequest) (*qc.State, error) {
	panic("not implemented")
}

// ReadCorrectable implements the ReadCorrectable method.
func (s *storageServerRequiringOrdering) ReadCorrectable(ctx context.Context, rq *qc.ReadRequest) (*qc.State, error) {
	panic("not implemented")
}

// ReadFuture implements the ReadFuture method.
func (s *storageServerRequiringOrdering) ReadFuture(ctx context.Context, rq *qc.ReadRequest) (*qc.State, error) {
	panic("not implemented")
}

// ReadCustomReturn implements the ReadCustomReturn method.
func (s *storageServerRequiringOrdering) ReadCustomReturn(ctx context.Context, rq *qc.ReadRequest) (*qc.State, error) {
	panic("not implemented")
}

// Write implements the Write method.
func (s *storageServerRequiringOrdering) Write(ctx context.Context, state *qc.State) (*qc.WriteResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if state.Timestamp != s.state.Timestamp+1 {
		err := fmt.Sprintf(
			"server: message received out-of-order: got: %d, expected: %d",
			state.Timestamp, s.state.Timestamp+1,
		)
		s.failMsg <- err
	} else {
		s.state = *state
	}
	return &s.writeResp, nil
}

// WriteFuture implements the WriteFuture method.
func (s *storageServerRequiringOrdering) WriteFuture(ctx context.Context, state *qc.State) (*qc.WriteResponse, error) {
	panic("not implemented")
}

// WritePerNode implements the WritePerNode method.
func (s *storageServerRequiringOrdering) WritePerNode(ctx context.Context, state *qc.State) (*qc.WriteResponse, error) {
	panic("not implemented")
}

// WriteAsync implements the WriteAsync method from the StorageServer interface.
func (s *storageServerRequiringOrdering) WriteAsync(stream qc.Storage_WriteAsyncServer) error {
	return nil
}

// ReadNoQC implements the ReadNoQC method from the StorageServer interface.
func (s *storageServerRequiringOrdering) ReadNoQC(ctx context.Context, rq *qc.ReadRequest) (*qc.State, error) {
	panic("not implemented")
}

// ReadCorrectableStream implements the ReadCorrectableStream method from the StorageServer interface.
func (s *storageServerRequiringOrdering) ReadCorrectableStream(rq *qc.ReadRequest, rrts qc.Storage_ReadCorrectableStreamServer) error {
	panic("not implemented")
}

// ReadExecuted returns when r has has completed a read.
func (s *storageServerRequiringOrdering) ReadExecuted() {
	panic("not implemented")
}

// WriteExecuted returns when r has has completed a write.
func (s *storageServerRequiringOrdering) WriteExecuted() {
	panic("not implemented")
}
