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

	"github.com/golang/protobuf/ptypes"
	"github.com/relab/gorums/examples/storage/proto"
)

func StartServer(address string) (*proto.GorumsServer, string) {
	// listen on given address
	lis, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatalf("Failed to listen on '%s': %v\n", address, err)
	}

	// init server implementation
	storage := newStorageServer()
	storage.logger = log.New(os.Stderr, fmt.Sprintf("%s: ", lis.Addr()), log.Ltime|log.Lmicroseconds|log.Lmsgprefix)

	// create Gorums server
	srv := proto.NewGorumsServer()
	// register server implementation with Gorums server
	srv.RegisterStorageServer(storage)
	// handle requests on listener
	go func() {
		err := srv.Serve(lis)
		if err != nil {
			storage.logger.Fatalf("Server error: %v\n", err)
		}
	}()

	return srv, lis.Addr().String()
}

func RunServer(address string) {
	// catch signals in order to shut down gracefully
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	srv, addr := StartServer(address)

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
func (s *storageServer) ReadRPC(req *proto.ReadRequest, out chan<- *proto.ReadResponse) {
	resp := s.Read(req)
	out <- resp
}

// WriteRPC is an RPC handler
func (s *storageServer) WriteRPC(req *proto.WriteRequest, out chan<- *proto.WriteResponse) {
	resp := s.Write(req)
	out <- resp
}

// ReadQC is an RPC handler for a quorum call
func (s *storageServer) ReadQC(req *proto.ReadRequest, out chan<- *proto.ReadResponse) {
	resp := s.Read(req)
	out <- resp
}

// WriteQC is an RPC handler for a quorum call
func (s *storageServer) WriteQC(req *proto.WriteRequest, out chan<- *proto.WriteResponse) {
	resp := s.Write(req)
	out <- resp
}

// Read reads a value from storage
func (s *storageServer) Read(req *proto.ReadRequest) *proto.ReadResponse {
	s.logger.Printf("Read '%s'\n", req.GetKey())
	s.mut.RLock()
	defer s.mut.RUnlock()
	state, ok := s.storage[req.GetKey()]
	time, err := ptypes.TimestampProto(state.Time)
	if err != nil {
		s.logger.Printf("Failed to marshal time: %v\n", err)
	}
	return &proto.ReadResponse{OK: ok, Value: state.Value, Time: time}
}

// Write writes a new value to storage if it is newer than the old value
func (s *storageServer) Write(req *proto.WriteRequest) *proto.WriteResponse {
	s.logger.Printf("Write '%s' = '%s'\n", req.GetKey(), req.GetValue())
	s.mut.Lock()
	defer s.mut.Unlock()
	oldState, ok := s.storage[req.GetKey()]
	if ok && oldState.Time.After(req.GetTime().AsTime()) {
		return &proto.WriteResponse{New: false}
	}
	s.storage[req.GetKey()] = state{Value: req.GetValue(), Time: req.GetTime().AsTime()}
	return &proto.WriteResponse{New: true}
}
