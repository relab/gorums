package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/examples/interceptors"
	pb "github.com/relab/gorums/examples/storage/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
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
	srv := gorums.NewServer(gorums.WithInterceptors(
		interceptors.LoggingInterceptor("abc"),
		interceptors.LoggingSimpleInterceptor,
		interceptors.NoFooAllowedInterceptor[*pb.WriteRequest],
		interceptors.MetadataInterceptor,
	))
	// register server implementation with Gorums server
	pb.RegisterStorageServer(srv, storage)
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

// storageServer is an implementation of pb.Storage
type storageServer struct {
	storage map[string]state
	mut     sync.Mutex
	logger  *log.Logger
}

func newStorageServer() *storageServer {
	return &storageServer{
		storage: make(map[string]state),
	}
}

// ReadRPC is an RPC handler
func (s *storageServer) ReadRPC(_ gorums.ServerCtx, req *pb.ReadRequest) (resp *pb.ReadResponse, err error) {
	return s.Read(req)
}

// WriteRPC is an RPC handler
func (s *storageServer) WriteRPC(_ gorums.ServerCtx, req *pb.WriteRequest) (resp *pb.WriteResponse, err error) {
	return s.Write(req)
}

// ReadQC is an RPC handler for a quorum call
func (s *storageServer) ReadQC(_ gorums.ServerCtx, req *pb.ReadRequest) (resp *pb.ReadResponse, err error) {
	return s.Read(req)
}

// WriteQC is an RPC handler for a quorum call
func (s *storageServer) WriteQC(_ gorums.ServerCtx, req *pb.WriteRequest) (resp *pb.WriteResponse, err error) {
	return s.Write(req)
}

func (s *storageServer) WriteMulticast(_ gorums.ServerCtx, req *pb.WriteRequest) {
	_, err := s.Write(req)
	if err != nil {
		s.logger.Printf("Write error: %v", err)
	}
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
