package dev

import (
	"io"
	"sync"
	"time"

	"golang.org/x/net/context"
)

// RegisterTestServer is a basic register server that in addition also can
// signal when a read or write has completed.
type RegisterTestServer interface {
	RegisterServer
	ReadExecuted()
	WriteExecuted()
}

// RegisterServerBasic represents a single State register.
type RegisterServerBasic struct {
	sync.RWMutex
	state State

	readExecutedChan  chan struct{}
	writeExecutedChan chan struct{}
}

// NewRegisterBasic returns a new basic register server.
func NewRegisterBasic() *RegisterServerBasic {
	return &RegisterServerBasic{
		// Use an appropriate larger buffer size if we construct test
		// scenarios where it's needed.
		writeExecutedChan: make(chan struct{}, 32),
		readExecutedChan:  make(chan struct{}, 32),
	}
}

// NewRegisterBasicWithState returns a new basic register server with an initial
// state set.
func NewRegisterBasicWithState(state *State) *RegisterServerBasic {
	return &RegisterServerBasic{
		state: *state,
		// Use an appropriate larger buffer size if we construct test
		// scenarios where it's needed.
		writeExecutedChan: make(chan struct{}, 32),
		readExecutedChan:  make(chan struct{}, 32),
	}
}

func (r *RegisterServerBasic) Read(ctx context.Context, rq *ReadRequest) (*State, error) {
	r.RLock()
	defer r.RUnlock()
	r.readExecutedChan <- struct{}{}
	return &State{Value: r.state.Value, Timestamp: r.state.Timestamp}, nil
}

func (r *RegisterServerBasic) Write(ctx context.Context, s *State) (*WriteResponse, error) {
	r.Lock()
	defer r.Unlock()
	writeResp := &WriteResponse{}
	if s.Timestamp > r.state.Timestamp {
		r.state = *s
		writeResp.New = true
	}
	r.writeExecutedChan <- struct{}{}
	return writeResp, nil
}

// WriteAsync implements the WriteAsync method from the RegisterServer interface.
func (r *RegisterServerBasic) WriteAsync(stream Register_WriteAsyncServer) error {
	for {
		state, err := stream.Recv()
		if err == io.EOF {
			return stream.SendAndClose(&Empty{})
		}
		if err != nil {
			return err
		}
		_, err = r.Write(context.Background(), state)
		if err != nil {
			return err
		}
	}
}

// ReadNoQC implements the ReadNoQC method from the RegisterServer interface.
func (r *RegisterServerBasic) ReadNoQC(ctx context.Context, rq *ReadRequest) (*State, error) {
	return r.Read(ctx, rq)
}

// ReadTwo implements the ReadTwo method from the RegisterServer interface.
func (r *RegisterServerBasic) ReadTwo(rq *ReadRequest, rrts Register_ReadTwoServer) error {
	return rrts.Send(&r.state)
}

// ReadExecuted returns when r has has completed a read.
func (r *RegisterServerBasic) ReadExecuted() {
	<-r.readExecutedChan
}

// WriteExecuted returns when r has has completed a write.
func (r *RegisterServerBasic) WriteExecuted() {
	<-r.writeExecutedChan
}

// RegisterServerError represents a register server that for any of its methods
// always returns an error.
type RegisterServerError struct {
	err error
}

// NewRegisterError returns a new error register server.
func NewRegisterError(err error) *RegisterServerError {
	return &RegisterServerError{
		err: err,
	}
}

func (r *RegisterServerError) Read(ctx context.Context, rq *ReadRequest) (*State, error) {
	return nil, r.err
}

func (r *RegisterServerError) Write(ctx context.Context, s *State) (*WriteResponse, error) {
	return nil, r.err
}

// WriteAsync implements the WriteAsync method from the RegisterServer interface.
func (r *RegisterServerError) WriteAsync(stream Register_WriteAsyncServer) error {
	return r.err
}

// ReadNoQC implements the ReadNoQC method from the RegisterServer interface.
func (r *RegisterServerError) ReadNoQC(ctx context.Context, rq *ReadRequest) (*State, error) {
	return r.Read(ctx, rq)
}

// ReadTwo implements the ReadTwo method from the RegisterServer interface.
func (r *RegisterServerError) ReadTwo(rq *ReadRequest, rrts Register_ReadTwoServer) error {
	return r.err
}

// ReadExecuted never returns since r always returns an error for Read.
func (r *RegisterServerError) ReadExecuted() {
	<-make(chan struct{})
}

// WriteExecuted never returns since r always returns an error for Write.
func (r *RegisterServerError) WriteExecuted() {
	<-make(chan struct{})
}

// RegisterServerSlow represents a register server that for any of its methods
// waits a given duration before returing a reply.
type RegisterServerSlow struct {
	delay      time.Duration
	realServer RegisterTestServer
}

// NewRegisterSlow returns a new slow register server.
func NewRegisterSlow(dur time.Duration) *RegisterServerSlow {
	return &RegisterServerSlow{
		delay:      dur,
		realServer: NewRegisterBasic(),
	}
}

// NewRegisterSlowWithState returns a new slow register server with an initial
// state set.
func NewRegisterSlowWithState(dur time.Duration, state *State) *RegisterServerSlow {
	return &RegisterServerSlow{
		delay:      dur,
		realServer: NewRegisterBasicWithState(state),
	}
}

func (r *RegisterServerSlow) Read(ctx context.Context, rq *ReadRequest) (*State, error) {
	time.Sleep(r.delay)
	return r.realServer.Read(ctx, rq)
}

func (r *RegisterServerSlow) Write(ctx context.Context, s *State) (*WriteResponse, error) {
	time.Sleep(r.delay)
	return r.realServer.Write(ctx, s)
}

// WriteAsync implements the WriteAsync method from the RegisterServer interface.
func (r *RegisterServerSlow) WriteAsync(stream Register_WriteAsyncServer) error {
	// There are no replies to wait for.
	return r.realServer.WriteAsync(stream)
}

// ReadNoQC implements the ReadNoQC method from the RegisterServer interface.
func (r *RegisterServerSlow) ReadNoQC(ctx context.Context, rq *ReadRequest) (*State, error) {
	time.Sleep(r.delay)
	return r.Read(ctx, rq)
}

// ReadTwo implements the ReadTwo method from the RegisterServer interface.
func (r *RegisterServerSlow) ReadTwo(rq *ReadRequest, rrts Register_ReadTwoServer) error {
	panic("not implemented")
}

// ReadExecuted returns when r has has completed a read.
func (r *RegisterServerSlow) ReadExecuted() {
	r.realServer.ReadExecuted()
}

// WriteExecuted returns when r has has completed a write.
func (r *RegisterServerSlow) WriteExecuted() {
	r.realServer.WriteExecuted()
}

// RegisterServerBench represents a single State register used for benchmarking.
type RegisterServerBench struct {
	sync.RWMutex
	state State
}

// NewRegisterBench returns a new register benchmark server.
func NewRegisterBench() *RegisterServerBench {
	return &RegisterServerBench{}
}

func (r *RegisterServerBench) Read(ctx context.Context, rq *ReadRequest) (*State, error) {
	r.RLock()
	defer r.RUnlock()
	return &State{Value: r.state.Value, Timestamp: r.state.Timestamp}, nil
}

func (r *RegisterServerBench) Write(ctx context.Context, s *State) (*WriteResponse, error) {
	r.Lock()
	defer r.Unlock()
	writeResp := &WriteResponse{}
	if s.Timestamp > r.state.Timestamp {
		r.state = *s
		writeResp.New = true
	}
	return writeResp, nil
}

// WriteAsync implements the WriteAsync method from the RegisterServer interface.
func (r *RegisterServerBench) WriteAsync(stream Register_WriteAsyncServer) error {
	for {
		state, err := stream.Recv()
		if err == io.EOF {
			return stream.SendAndClose(&Empty{})
		}
		if err != nil {
			return err
		}
		_, err = r.Write(context.Background(), state)
		if err != nil {
			return err
		}
	}
}

// ReadNoQC implements the ReadNoQC method from the RegisterServer interface.
func (r *RegisterServerBench) ReadNoQC(ctx context.Context, rq *ReadRequest) (*State, error) {
	return r.Read(ctx, rq)
}

// ReadTwo implements the ReadTwo method from the RegisterServer interface.
func (r *RegisterServerBench) ReadTwo(rq *ReadRequest, rrts Register_ReadTwoServer) error {
	panic("not implemented")
}

// ReadExecuted is a no-op.
func (r *RegisterServerBench) ReadExecuted() {}

// WriteExecuted is no-op.
func (r *RegisterServerBench) WriteExecuted() {}

// RegisterServerLockedWithState represents a register server with an initial
// state that does not reply to any requests before it's unlocked.
type RegisterServerLockedWithState struct {
	lock              chan struct{}
	realServer        *RegisterServerBasic
	readTwoNumReplies int
	readTwoLockChan   chan struct{}
}

// NewRegisterServerLockedWithState returns a new locked register server with an initial state.
func NewRegisterServerLockedWithState(state *State, readTwoNumReplies int) *RegisterServerLockedWithState {
	return &RegisterServerLockedWithState{
		lock:              make(chan struct{}),
		realServer:        NewRegisterBasicWithState(state),
		readTwoNumReplies: readTwoNumReplies,
		readTwoLockChan:   make(chan struct{}, 1),
	}
}

func (r *RegisterServerLockedWithState) Read(ctx context.Context, rq *ReadRequest) (*State, error) {
	<-r.lock
	return r.realServer.Read(ctx, rq)
}

func (r *RegisterServerLockedWithState) Write(ctx context.Context, s *State) (*WriteResponse, error) {
	<-r.lock
	return r.realServer.Write(ctx, s)
}

// WriteAsync implements the WriteAsync method from the RegisterServer interface.
func (r *RegisterServerLockedWithState) WriteAsync(stream Register_WriteAsyncServer) error {
	<-r.lock
	return r.realServer.WriteAsync(stream)
}

// ReadNoQC implements the ReadNoQC method from the RegisterServer interface.
func (r *RegisterServerLockedWithState) ReadNoQC(ctx context.Context, rq *ReadRequest) (*State, error) {
	return r.Read(ctx, rq)
}

// ReadTwo implements the ReadTwo method from the RegisterServer interface.
func (r *RegisterServerLockedWithState) ReadTwo(rq *ReadRequest, rrts Register_ReadTwoServer) error {
	<-r.lock

	r.realServer.RLock()
	state := r.realServer.state
	r.realServer.RUnlock()

	for i := 0; i < r.readTwoNumReplies; i++ {
		<-r.readTwoLockChan
		err := rrts.Send(&state)
		if err != nil {
			return err
		}
	}

	return nil
}

// ReadExecuted returns when r has has completed a read.
func (r *RegisterServerLockedWithState) ReadExecuted() {
	r.realServer.ReadExecuted()
}

// WriteExecuted returns when r has has completed a write.
func (r *RegisterServerLockedWithState) WriteExecuted() {
	r.realServer.WriteExecuted()
}

// Unlock unlocks the register server.
func (r *RegisterServerLockedWithState) Unlock() {
	close(r.lock)
}

// PerformSingleReadTwo lets the register server send a single reply from a
// single ReadTwo method handler.
func (r *RegisterServerLockedWithState) PerformSingleReadTwo() {
	r.readTwoLockChan <- struct{}{}
}
