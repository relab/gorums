package dev

import (
	"io"
	"sync"
	"time"

	"golang.org/x/net/context"
)

// StorageTestServer is a basic storage server that in addition also can
// signal when a read or write has completed.
type StorageTestServer interface {
	StorageServer
	ReadExecuted()
	WriteExecuted()
}

// StorageServerBasic represents a single state storage.
type StorageServerBasic struct {
	mu    sync.RWMutex
	state State

	readExecutedChan  chan struct{}
	writeExecutedChan chan struct{}
}

// NewStorageBasic returns a new basic storage server.
func NewStorageBasic() *StorageServerBasic {
	return &StorageServerBasic{
		// Use an appropriate larger buffer size if we construct test
		// scenarios where it's needed.
		writeExecutedChan: make(chan struct{}, 32),
		readExecutedChan:  make(chan struct{}, 32),
	}
}

// NewStorageBasicWithState returns a new basic storage server with an initial
// state set.
func NewStorageBasicWithState(state *State) *StorageServerBasic {
	return &StorageServerBasic{
		state: *state,
		// Use an appropriate larger buffer size if we construct test
		// scenarios where it's needed.
		writeExecutedChan: make(chan struct{}, 32),
		readExecutedChan:  make(chan struct{}, 32),
	}
}

// Read implements the Read method.
func (s *StorageServerBasic) Read(ctx context.Context, rq *ReadRequest) (*State, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	s.readExecutedChan <- struct{}{}
	return &State{Value: s.state.Value, Timestamp: s.state.Timestamp}, nil
}

// ReadCorrectable implements the ReadCorrectable method.
func (s *StorageServerBasic) ReadCorrectable(ctx context.Context, rq *ReadRequest) (*State, error) {
	return s.Read(ctx, rq)
}

// ReadFuture implements the ReadFuture method.
func (s *StorageServerBasic) ReadFuture(ctx context.Context, rq *ReadRequest) (*State, error) {
	return s.Read(ctx, rq)
}

// ReadCustomReturn implements the ReadCustomReturn method.
func (s *StorageServerBasic) ReadCustomReturn(ctx context.Context, rq *ReadRequest) (*State, error) {
	return s.Read(ctx, rq)
}

// Write implements the Write method.
func (s *StorageServerBasic) Write(ctx context.Context, state *State) (*WriteResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	writeResp := &WriteResponse{}
	if state.Timestamp > s.state.Timestamp {
		s.state = *state
		writeResp.New = true
	}
	s.writeExecutedChan <- struct{}{}
	return writeResp, nil
}

// WriteFuture implements the WriteFuture method.
func (s *StorageServerBasic) WriteFuture(ctx context.Context, state *State) (*WriteResponse, error) {
	return s.Write(ctx, state)
}

// WritePerNode implements the WritePerNode method.
func (s *StorageServerBasic) WritePerNode(ctx context.Context, state *State) (*WriteResponse, error) {
	return s.Write(ctx, state)
}

// WriteAsync implements the WriteAsync method from the StorageServer interface.
func (s *StorageServerBasic) WriteAsync(stream Storage_WriteAsyncServer) error {
	for {
		state, err := stream.Recv()
		if err == io.EOF {
			return stream.SendAndClose(&Empty{})
		}
		if err != nil {
			return err
		}
		_, err = s.Write(context.Background(), state)
		if err != nil {
			return err
		}
	}
}

// WriteOrdered is not implemented
func (s *StorageServerBasic) WriteOrdered(srv Storage_WriteOrderedServer) error {
	return writeOrderedStub(srv)
}

// ReadNoQC implements the ReadNoQC method from the StorageServer interface.
func (s *StorageServerBasic) ReadNoQC(ctx context.Context, rq *ReadRequest) (*State, error) {
	return s.Read(ctx, rq)
}

// ReadCorrectableStream implements the ReadCorrectableStream method from the StorageServer interface.
func (s *StorageServerBasic) ReadCorrectableStream(rq *ReadRequest, srts Storage_ReadCorrectableStreamServer) error {
	return srts.Send(&s.state)
}

// ReadExecuted returns when s has completed a read.
func (s *StorageServerBasic) ReadExecuted() {
	<-s.readExecutedChan
}

// WriteExecuted returns when s has completed a write.
func (s *StorageServerBasic) WriteExecuted() {
	<-s.writeExecutedChan
}

// StorageServerError represents a storage server that for any of its methods
// always returns an error.
type StorageServerError struct {
	err error
}

// NewStorageError returns a new error storage server.
func NewStorageError(err error) *StorageServerError {
	return &StorageServerError{
		err: err,
	}
}

// Read implements the Read method.
func (s *StorageServerError) Read(ctx context.Context, rq *ReadRequest) (*State, error) {
	return nil, s.err
}

// ReadCorrectable implements the ReadCorrectable method.
func (s *StorageServerError) ReadCorrectable(ctx context.Context, rq *ReadRequest) (*State, error) {
	return nil, s.err
}

// ReadFuture implements the ReadFuture method.
func (s *StorageServerError) ReadFuture(ctx context.Context, rq *ReadRequest) (*State, error) {
	return nil, s.err
}

// ReadCustomReturn implements the ReadCustomReturn method.
func (s *StorageServerError) ReadCustomReturn(ctx context.Context, rq *ReadRequest) (*State, error) {
	return nil, s.err
}

// Write implements the Write method.
func (s *StorageServerError) Write(ctx context.Context, state *State) (*WriteResponse, error) {
	return nil, s.err
}

// WriteFuture implements the WriteFuture method.
func (s *StorageServerError) WriteFuture(ctx context.Context, state *State) (*WriteResponse, error) {
	return nil, s.err
}

// WritePerNode implements the WritePerNode method.
func (s *StorageServerError) WritePerNode(ctx context.Context, state *State) (*WriteResponse, error) {
	return nil, s.err
}

// WriteAsync implements the WriteAsync method from the StorageServer interface.
func (s *StorageServerError) WriteAsync(stream Storage_WriteAsyncServer) error {
	return s.err
}

// WriteOrdered implements the WriteOrdered method from the StorageServer interface
func (s *StorageServerError) WriteOrdered(srv Storage_WriteOrderedServer) error {
	return writeOrderedStub(srv)
}

// ReadNoQC implements the ReadNoQC method from the StorageServer interface.
func (s *StorageServerError) ReadNoQC(ctx context.Context, rq *ReadRequest) (*State, error) {
	return nil, s.err
}

// ReadCorrectableStream implements the ReadCorrectableStream method from the StorageServer interface.
func (s *StorageServerError) ReadCorrectableStream(rq *ReadRequest, srts Storage_ReadCorrectableStreamServer) error {
	return s.err
}

// ReadExecuted never returns since s always returns an error for Read.
func (s *StorageServerError) ReadExecuted() {
	<-make(chan struct{})
}

// WriteExecuted never returns since s always returns an error for Write.
func (s *StorageServerError) WriteExecuted() {
	<-make(chan struct{})
}

// StorageServerSlow represents a storage server that for any of its methods
// waits a given duration before returning a reply.
type StorageServerSlow struct {
	delay      time.Duration
	realServer StorageTestServer
}

// NewStorageSlow returns a slow storage server.
func NewStorageSlow(dur time.Duration) *StorageServerSlow {
	return &StorageServerSlow{
		delay:      dur,
		realServer: NewStorageBasic(),
	}
}

// NewStorageSlowWithState returns a slow storage server with some initial state.
func NewStorageSlowWithState(dur time.Duration, state *State) *StorageServerSlow {
	return &StorageServerSlow{
		delay:      dur,
		realServer: NewStorageBasicWithState(state),
	}
}

// Read implements the Read method.
func (s *StorageServerSlow) Read(ctx context.Context, rq *ReadRequest) (*State, error) {
	time.Sleep(s.delay)
	return s.realServer.Read(ctx, rq)
}

// ReadCorrectable implements the ReadCorrectable method.
func (s *StorageServerSlow) ReadCorrectable(ctx context.Context, rq *ReadRequest) (*State, error) {
	return s.Read(ctx, rq)
}

// ReadFuture implements the ReadFuture method.
func (s *StorageServerSlow) ReadFuture(ctx context.Context, rq *ReadRequest) (*State, error) {
	return s.Read(ctx, rq)
}

// ReadCustomReturn implements the ReadCustomReturn method.
func (s *StorageServerSlow) ReadCustomReturn(ctx context.Context, rq *ReadRequest) (*State, error) {
	return s.Read(ctx, rq)
}

// Write implements the Write method.
func (s *StorageServerSlow) Write(ctx context.Context, state *State) (*WriteResponse, error) {
	time.Sleep(s.delay)
	return s.realServer.Write(ctx, state)
}

// WriteFuture implements the WriteFuture method.
func (s *StorageServerSlow) WriteFuture(ctx context.Context, state *State) (*WriteResponse, error) {
	return s.Write(ctx, state)
}

// WritePerNode implements the WritePerNode method.
func (s *StorageServerSlow) WritePerNode(ctx context.Context, state *State) (*WriteResponse, error) {
	return s.Write(ctx, state)
}

// WriteAsync implements the WriteAsync method from the StorageServer interface.
func (s *StorageServerSlow) WriteAsync(stream Storage_WriteAsyncServer) error {
	// There are no replies to wait for.
	return s.realServer.WriteAsync(stream)
}

// WriteOrdered is not implemented
func (s *StorageServerSlow) WriteOrdered(srv Storage_WriteOrderedServer) error {
	return writeOrderedStub(srv)
}

// ReadNoQC implements the ReadNoQC method from the StorageServer interface.
func (s *StorageServerSlow) ReadNoQC(ctx context.Context, rq *ReadRequest) (*State, error) {
	return s.Read(ctx, rq)
}

// ReadCorrectableStream implements the ReadCorrectableStream method from the StorageServer interface.
func (s *StorageServerSlow) ReadCorrectableStream(rq *ReadRequest, srts Storage_ReadCorrectableStreamServer) error {
	time.Sleep(s.delay)
	return s.realServer.ReadCorrectableStream(rq, srts)
}

// ReadExecuted returns when r has has completed a read.
func (s *StorageServerSlow) ReadExecuted() {
	s.realServer.ReadExecuted()
}

// WriteExecuted returns when r has has completed a write.
func (s *StorageServerSlow) WriteExecuted() {
	s.realServer.WriteExecuted()
}

// StorageServerBench represents a single State storage used for benchmarking.
type StorageServerBench struct {
	mu    sync.RWMutex
	state State
}

// NewStorageBench returns a new storage benchmark server.
func NewStorageBench() *StorageServerBench {
	return &StorageServerBench{}
}

// Read implements the Read method.
func (s *StorageServerBench) Read(ctx context.Context, rq *ReadRequest) (*State, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return &State{Value: s.state.Value, Timestamp: s.state.Timestamp}, nil
}

// ReadCorrectable implements the ReadCorrectable method.
func (s *StorageServerBench) ReadCorrectable(ctx context.Context, rq *ReadRequest) (*State, error) {
	return s.Read(ctx, rq)
}

// ReadFuture implements the ReadFuture method.
func (s *StorageServerBench) ReadFuture(ctx context.Context, rq *ReadRequest) (*State, error) {
	return s.Read(ctx, rq)
}

// ReadCustomReturn implements the ReadCustomReturn method.
func (s *StorageServerBench) ReadCustomReturn(ctx context.Context, rq *ReadRequest) (*State, error) {
	return s.Read(ctx, rq)
}

// Write implements the Write method.
func (s *StorageServerBench) Write(ctx context.Context, state *State) (*WriteResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	writeResp := &WriteResponse{}
	if state.Timestamp > s.state.Timestamp {
		s.state = *state
		writeResp.New = true
	}
	return writeResp, nil
}

// WriteFuture implements the WriteFuture method.
func (s *StorageServerBench) WriteFuture(ctx context.Context, state *State) (*WriteResponse, error) {
	return s.Write(ctx, state)
}

// WritePerNode implements the WritePerNode method.
func (s *StorageServerBench) WritePerNode(ctx context.Context, state *State) (*WriteResponse, error) {
	return s.Write(ctx, state)
}

// WriteAsync implements the WriteAsync method from the StorageServer interface.
func (s *StorageServerBench) WriteAsync(stream Storage_WriteAsyncServer) error {
	for {
		state, err := stream.Recv()
		if err == io.EOF {
			return stream.SendAndClose(&Empty{})
		}
		if err != nil {
			return err
		}
		_, err = s.Write(context.Background(), state)
		if err != nil {
			return err
		}
	}
}

// WriteOrdered is not implemented
func (s *StorageServerBench) WriteOrdered(srv Storage_WriteOrderedServer) error {
	return writeOrderedStub(srv)
}

// ReadNoQC implements the ReadNoQC method from the StorageServer interface.
func (s *StorageServerBench) ReadNoQC(ctx context.Context, rq *ReadRequest) (*State, error) {
	return s.Read(ctx, rq)
}

// ReadCorrectableStream implements the ReadCorrectableStream method from the StorageServer interface.
func (s *StorageServerBench) ReadCorrectableStream(rq *ReadRequest, srts Storage_ReadCorrectableStreamServer) error {
	return srts.Send(&s.state)
}

// ReadExecuted is a no-op.
func (s *StorageServerBench) ReadExecuted() {}

// WriteExecuted is no-op.
func (s *StorageServerBench) WriteExecuted() {}

// StorageServerLockedWithState represents a storage server with an initial
// state that does not reply to any requests before it's unlocked.
type StorageServerLockedWithState struct {
	lock                            chan struct{}
	realServer                      *StorageServerBasic
	ReadCorrectableStreamNumReplies int
	ReadCorrectableStreamLockChan   chan struct{}
}

// NewStorageServerLockedWithState returns a new locked storage server with an initial state.
func NewStorageServerLockedWithState(state *State, ReadCorrectableStreamNumReplies int) *StorageServerLockedWithState {
	return &StorageServerLockedWithState{
		lock:                            make(chan struct{}),
		realServer:                      NewStorageBasicWithState(state),
		ReadCorrectableStreamNumReplies: ReadCorrectableStreamNumReplies,
		ReadCorrectableStreamLockChan:   make(chan struct{}, 1),
	}
}

// Read implements the Read method.
func (s *StorageServerLockedWithState) Read(ctx context.Context, rq *ReadRequest) (*State, error) {
	<-s.lock
	return s.realServer.Read(ctx, rq)
}

// ReadFuture implements the ReadFuture method.
func (s *StorageServerLockedWithState) ReadFuture(ctx context.Context, rq *ReadRequest) (*State, error) {
	return s.Read(ctx, rq)
}

// ReadCustomReturn implements the ReadCustomReturn method.
func (s *StorageServerLockedWithState) ReadCustomReturn(ctx context.Context, rq *ReadRequest) (*State, error) {
	return s.Read(ctx, rq)
}

// ReadCorrectable implements the ReadCorrectable method.
func (s *StorageServerLockedWithState) ReadCorrectable(ctx context.Context, rq *ReadRequest) (*State, error) {
	return s.Read(ctx, rq)
}

func (s *StorageServerLockedWithState) Write(ctx context.Context, state *State) (*WriteResponse, error) {
	<-s.lock
	return s.realServer.Write(ctx, state)
}

// WriteFuture implements the WriteFuture method.
func (s *StorageServerLockedWithState) WriteFuture(ctx context.Context, state *State) (*WriteResponse, error) {
	return s.Write(ctx, state)
}

// WritePerNode implements the WritePerNode method.
func (s *StorageServerLockedWithState) WritePerNode(ctx context.Context, state *State) (*WriteResponse, error) {
	return s.Write(ctx, state)
}

// WriteAsync implements the WriteAsync method from the StorageServer interface.
func (s *StorageServerLockedWithState) WriteAsync(stream Storage_WriteAsyncServer) error {
	<-s.lock
	return s.realServer.WriteAsync(stream)
}

// WriteOrdered is not implemented
func (s *StorageServerLockedWithState) WriteOrdered(srv Storage_WriteOrderedServer) error {
	return writeOrderedStub(srv)
}

// ReadNoQC implements the ReadNoQC method from the StorageServer interface.
func (s *StorageServerLockedWithState) ReadNoQC(ctx context.Context, rq *ReadRequest) (*State, error) {
	return s.Read(ctx, rq)
}

// ReadCorrectableStream implements the ReadCorrectableStream method from the StorageServer interface.
func (s *StorageServerLockedWithState) ReadCorrectableStream(rq *ReadRequest, srts Storage_ReadCorrectableStreamServer) error {
	<-s.lock

	s.realServer.mu.RLock()
	state := s.realServer.state
	s.realServer.mu.RUnlock()

	for i := 0; i < s.ReadCorrectableStreamNumReplies; i++ {
		<-s.ReadCorrectableStreamLockChan
		err := srts.Send(&state)
		if err != nil {
			return err
		}
	}

	return nil
}

// ReadExecuted returns when r has has completed a read.
func (s *StorageServerLockedWithState) ReadExecuted() {
	s.realServer.ReadExecuted()
}

// WriteExecuted returns when r has has completed a write.
func (s *StorageServerLockedWithState) WriteExecuted() {
	s.realServer.WriteExecuted()
}

// Unlock unlocks the storage server.
func (s *StorageServerLockedWithState) Unlock() {
	close(s.lock)
}

// PerformSingleReadCorrectableStream lets the storage server send a single reply from a
// single ReadCorrectableStream method handler.
func (s *StorageServerLockedWithState) PerformSingleReadCorrectableStream() {
	s.ReadCorrectableStreamLockChan <- struct{}{}
}

func writeOrderedStub(srv Storage_WriteOrderedServer) error {
	return WriteOrderedServerLoop(srv, func(req *State) *WriteResponse {
		panic("not implemented")
	})
}
