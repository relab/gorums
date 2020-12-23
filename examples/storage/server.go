package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/relab/gorums"
	"github.com/relab/gorums/examples/storage/proto"
)

func startServer(address string) (*gorums.Server, string) {
	// listen on given address
	lis, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatalf("Failed to listen on '%s': %v\n", address, err)
	}

	// init server implementation
	storage := newStorageServer()
	storage.logger = log.New(os.Stderr, fmt.Sprintf("%s: ", lis.Addr()), log.Ltime|log.Lmicroseconds|log.Lmsgprefix)

	// create Gorums server
	srv := gorums.NewServer()
	// register server implementation with Gorums server
	proto.RegisterStorageServer(srv, storage)
	// handle requests on listener
	go func() {
		err := srv.Serve(lis)
		if err != nil {
			storage.logger.Fatalf("Server error: %v\n", err)
		}
	}()

	return srv, lis.Addr().String()
}

func runServer(address string) {
	// catch signals in order to shut down gracefully
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	srv, addr := startServer(address)

	log.Printf("Started storage server on %s\n", addr)

	<-signals
	// shutdown Gorums server
	srv.Stop()
}

type state struct {
	Value string
	Time  time.Time
}

// storageServer is an implementation of proto.Storage
type storageServer struct {
	storage map[string]state
	mut     sync.RWMutex
	logger  *log.Logger
}

func newStorageServer() *storageServer {
	return &storageServer{
		storage: make(map[string]state),
	}
}

// ReadRPC is an RPC handler
func (s *storageServer) ReadRPC(_ context.Context, req *proto.ReadRequest, ret func(*proto.ReadResponse, error)) {
	ret(s.Read(req))
}

// WriteRPC is an RPC handler
func (s *storageServer) WriteRPC(_ context.Context, req *proto.WriteRequest, ret func(*proto.WriteResponse, error)) {
	ret(s.Write(req))
}

// ReadQC is an RPC handler for a quorum call
func (s *storageServer) ReadQC(_ context.Context, req *proto.ReadRequest, ret func(*proto.ReadResponse, error)) {
	ret(s.Read(req))
}

// WriteQC is an RPC handler for a quorum call
func (s *storageServer) WriteQC(_ context.Context, req *proto.WriteRequest, ret func(*proto.WriteResponse, error)) {
	ret(s.Write(req))
}

func (s *storageServer) WriteMulticast(_ context.Context, req *proto.WriteRequest) {
	_, err := s.Write(req)
	if err != nil {
		s.logger.Printf("Write error: %v", err)
	}
}

// Read reads a value from storage
func (s *storageServer) Read(req *proto.ReadRequest) (*proto.ReadResponse, error) {
	s.logger.Printf("Read '%s'\n", req.GetKey())
	s.mut.RLock()
	defer s.mut.RUnlock()
	state, ok := s.storage[req.GetKey()]
	if !ok {
		return &proto.ReadResponse{OK: false}, nil
	}
	time, err := ptypes.TimestampProto(state.Time)
	if err != nil {
		s.logger.Printf("Failed to marshal time: %v\n", err)
		return nil, err
	}
	return &proto.ReadResponse{OK: true, Value: state.Value, Time: time}, nil
}

// Write writes a new value to storage if it is newer than the old value
func (s *storageServer) Write(req *proto.WriteRequest) (*proto.WriteResponse, error) {
	s.logger.Printf("Write '%s' = '%s'\n", req.GetKey(), req.GetValue())
	s.mut.Lock()
	defer s.mut.Unlock()
	oldState, ok := s.storage[req.GetKey()]
	if ok && oldState.Time.After(req.GetTime().AsTime()) {
		return &proto.WriteResponse{New: false}, nil
	}
	s.storage[req.GetKey()] = state{Value: req.GetValue(), Time: req.GetTime().AsTime()}
	return &proto.WriteResponse{New: true}, nil
}
