package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"slices"
	"sync"
	"syscall"
	"time"

	"github.com/relab/gorums"
	pb "github.com/relab/gorums/examples/storage/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// runServer starts a storage server. peers[0] is the address to listen on;
// the remaining entries are the other cluster peers. The server waits until
// all peers have connected before logging the ready message.
func runServer(address string, peers []string, srvOpt gorums.ServerOption) error {
	if len(peers) == 0 {
		return fmt.Errorf("no peer addresses provided")
	}
	myID, peerList, err := peerConfig(address, peers)
	if err != nil {
		return err
	}
	sys, err := gorums.NewSystem(address,
		gorums.WithServerOptions(srvOpt, gorums.WithConfig(myID, peerList)),
		gorums.WithOutboundNodes(peerList),
		gorums.WithDialOptions(grpc.WithTransportCredentials(insecure.NewCredentials())),
	)
	if err != nil {
		return fmt.Errorf("failed to create system on %q: %w", address, err)
	}

	// catch signals in order to shut down gracefully
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	registerAndServe(sys, myID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := sys.WaitForConfig(ctx, func(cfg gorums.Configuration) bool {
		return cfg.Size() == len(peers)
	}); err != nil {
		return fmt.Errorf("peers did not connect in time: %w", err)
	}

	log.Printf("Started storage server on %s\n", sys.Addr())

	<-signals
	return sys.Stop()
}

// runLocalCluster starts four in-process servers for local testing.
// It returns the server addresses and a stop function. The caller must
// call stop when the cluster is no longer needed.
func runLocalCluster(srvOpts gorums.ServerOption) ([]string, func(), error) {
	dialOpts := gorums.WithDialOptions(grpc.WithTransportCredentials(insecure.NewCredentials()))
	systems, stop, err := gorums.NewLocalSystems(4, gorums.WithServerOptions(srvOpts), dialOpts)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create local systems: %w", err)
	}

	addrs := make([]string, len(systems))
	for i, sys := range systems {
		addrs[i] = sys.Addr()
		registerAndServe(sys, uint32(i+1))
	}

	// Wait for all systems to see each other before opening the client REPL.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, sys := range systems {
		if err := sys.WaitForConfig(ctx, func(cfg gorums.Configuration) bool {
			return cfg.Size() == len(systems)
		}); err != nil {
			stop()
			return nil, nil, fmt.Errorf("cluster failed to connect: %w", err)
		}
	}

	return addrs, stop, nil
}

// peerConfig returns the node ID and peer list for a server with the given
// address and peer addresses. The node ID is assumed to follow the ordering
// semantics of WithNodeList, where each node's ID is the 1-based index of
// its address in the sorted peer list. Sorting ensures deterministic node ID
// assignment regardless of the order of addresses in the input list.
// The server's own address must be included in the peer list.
// It returns an error if the server's address is not found in the peer list.
func peerConfig(address string, peers []string) (uint32, gorums.NodeListOption, error) {
	sorted := slices.Clone(peers)
	slices.Sort(sorted)
	idx := slices.Index(sorted, address)
	if idx < 0 {
		return 0, nil, fmt.Errorf("server address %q not found in -addrs list", address)
	}
	return uint32(idx + 1), gorums.WithNodeList(sorted), nil
}

// registerAndServe registers the storage service on sys and starts serving in
// a background goroutine. The server log output is labelled with the node ID.
func registerAndServe(sys *gorums.System, id uint32) {
	storage := newStorageServer(os.Stderr, fmt.Sprintf("node %d", id))
	sys.RegisterService(nil, func(srv *gorums.Server) {
		pb.RegisterStorageServer(srv, storage)
	})
	go func() {
		if err := sys.Serve(); err != nil {
			log.Printf("Server error: %v", err)
		}
	}()
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
func (s *storageServer) ReadCorrectable(_ gorums.ServerCtx, req *pb.ReadRequest, send func(response *pb.ReadResponse)) {
	resp, err := s.Read(req)
	if err != nil {
		s.logger.Printf("ReadCorrectable error: %v", err)
		return
	}
	// Note: in a real application, the server might send multiple updates.
	// For this storage example, we just send the current state once.
	send(resp)
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
