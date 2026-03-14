package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/relab/gorums"
	pb "github.com/relab/gorums/examples/storage/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func runServer(address string, opts ...gorums.Option) {
	sys, err := gorums.NewSystem(address, opts...)
	if err != nil {
		log.Fatalf("Failed to create system on '%s': %v", address, err)
	}
	storage := newStorageServer(os.Stderr, sys.Addr())
	sys.RegisterService(nil, func(srv *gorums.Server) {
		pb.RegisterStorageServer(srv, storage)
	})

	// catch signals in order to shut down gracefully
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	go func() {
		if err := sys.Serve(); err != nil {
			log.Printf("Server error: %v", err)
		}
	}()

	log.Printf("Started storage server on %s\n", sys.Addr())

	<-signals
	_ = sys.Stop()
}

type state struct {
	Value string
	Time  time.Time
}

// storageServer is an implementation of pb.Storage
type storageServer struct {
	storage map[string]state
	mut     sync.Mutex
	logger  *log.Logger
}

func newStorageServer(out io.Writer, label string) *storageServer {
	return &storageServer{
		storage: make(map[string]state),
		logger:  log.New(rawWriter{out}, label+": ", 0),
	}
}

// rawWriter wraps a writer and replaces lone \n with \r\n,
// so that log output renders correctly in raw terminal mode.
type rawWriter struct {
	w io.Writer
}

func (rw rawWriter) Write(p []byte) (n int, err error) {
	replaced := bytes.ReplaceAll(p, []byte("\n"), []byte("\r\n"))
	wn, err := rw.w.Write(replaced)

	// Treat a short write to the underlying writer as an error
	if err == nil && wn < len(replaced) {
		err = io.ErrShortWrite
	}

	// If an error occurred, conservatively report 0 bytes written to
	// prevent the caller from assuming data was fully processed.
	if err != nil {
		return 0, err
	}

	// Success: satisfy the io.Writer contract by returning the original length
	return len(p), nil
}

// ReadRPC is an RPC handler
func (s *storageServer) ReadRPC(_ gorums.ServerCtx, req *pb.ReadRequest) (resp *pb.ReadResponse, err error) {
	return s.Read(req)
}

// WriteRPC is an RPC handler
func (s *storageServer) WriteRPC(_ gorums.ServerCtx, req *pb.WriteRequest) (resp *pb.WriteResponse, err error) {
	return s.Write(req)
}

// WriteUnicast is an RPC handler for one-way unicast writes.
func (s *storageServer) WriteUnicast(_ gorums.ServerCtx, req *pb.WriteRequest) {
	if _, err := s.Write(req); err != nil {
		s.logger.Printf("WriteUnicast error: %v", err)
	}
}

// WriteMulticast is an RPC handler for one-way multicast writes.
func (s *storageServer) WriteMulticast(_ gorums.ServerCtx, req *pb.WriteRequest) {
	_, err := s.Write(req)
	if err != nil {
		s.logger.Printf("Write error: %v", err)
	}
}

// ReadQC is an RPC handler for a quorum call.
func (s *storageServer) ReadQC(_ gorums.ServerCtx, req *pb.ReadRequest) (resp *pb.ReadResponse, err error) {
	return s.Read(req)
}

// WriteQC is an RPC handler for a quorum call.
func (s *storageServer) WriteQC(_ gorums.ServerCtx, req *pb.WriteRequest) (resp *pb.WriteResponse, err error) {
	return s.Write(req)
}

// ReadCorrectable is an RPC handler for a correctable quorum call. It sends multiple responses.
func (s *storageServer) ReadCorrectable(_ gorums.ServerCtx, req *pb.ReadRequest, send func(response *pb.ReadResponse) error) error {
	resp, err := s.Read(req)
	if err != nil {
		return err
	}
	// Note: in a real application, the server might send multiple updates.
	// For this storage example, we just send the current state once.
	return send(resp)
}

// ReadNestedQC is a quorum-call handler that performs a nested quorum call
// using the server's known-peer configuration from WithConfig.
func (s *storageServer) ReadNestedQC(ctx gorums.ServerCtx, req *pb.ReadRequest) (resp *pb.ReadResponse, err error) {
	cfg := ctx.Config()
	if len(cfg) == 0 {
		return nil, fmt.Errorf("read_nested_qc: requires server peer configuration")
	}
	// Release before nested outbound calls to avoid blocking inbound recv processing.
	ctx.Release()
	return newestValue(pb.ReadQC(cfg.Context(ctx), req))
}

// WriteNestedMulticast is a quorum-call handler that performs a nested multicast
// using the server's known-peer configuration from WithConfig.
func (s *storageServer) WriteNestedMulticast(ctx gorums.ServerCtx, req *pb.WriteRequest) (resp *pb.WriteResponse, err error) {
	cfg := ctx.Config()
	if len(cfg) == 0 {
		return nil, fmt.Errorf("write_nested_multicast: requires server peer configuration")
	}
	// Release before nested outbound calls to avoid blocking inbound recv processing.
	ctx.Release()
	if err := pb.WriteMulticast(cfg.Context(ctx), req); err != nil {
		return nil, fmt.Errorf("write_nested_multicast: %w", err)
	}
	return pb.WriteResponse_builder{New: true}.Build(), nil
}

// Read reads a value from storage
func (s *storageServer) Read(req *pb.ReadRequest) (*pb.ReadResponse, error) {
	s.logger.Printf("Read '%s'\n", req.GetKey())
	s.mut.Lock()
	defer s.mut.Unlock()
	state, ok := s.storage[req.GetKey()]
	if !ok {
		return pb.ReadResponse_builder{OK: false}.Build(), nil
	}
	return pb.ReadResponse_builder{OK: true, Value: state.Value, Time: timestamppb.New(state.Time)}.Build(), nil
}

// Write writes a new value to storage if it is newer than the old value
func (s *storageServer) Write(req *pb.WriteRequest) (*pb.WriteResponse, error) {
	s.logger.Printf("Write '%s' = '%s'\n", req.GetKey(), req.GetValue())
	s.mut.Lock()
	defer s.mut.Unlock()
	oldState, ok := s.storage[req.GetKey()]
	if ok && oldState.Time.After(req.GetTime().AsTime()) {
		return pb.WriteResponse_builder{New: false}.Build(), nil
	}
	s.storage[req.GetKey()] = state{Value: req.GetValue(), Time: req.GetTime().AsTime()}
	return pb.WriteResponse_builder{New: true}.Build(), nil
}
