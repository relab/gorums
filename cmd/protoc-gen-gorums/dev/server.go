package dev

import (
	"context"
	"net"

	"github.com/relab/gorums/ordering"
	"google.golang.org/grpc"
)

// requestHandler is used to fetch a response message based on the request.
// A requestHandler should receive a message from the server, unmarshal it into
// the proper type for that Method's request type, call a user provided Handler,
// and return a marshaled result to the server.
type requestHandler func(*gorumsMessage, chan<- *gorumsMessage)

type orderingServer struct {
	handlers map[int32]requestHandler
	opts     *serverOptions
	ordering.UnimplementedGorumsServer
}

func newOrderingServer(opts *serverOptions) *orderingServer {
	s := &orderingServer{
		handlers: make(map[int32]requestHandler),
		opts:     opts,
	}
	return s
}

func (s *orderingServer) NodeStream(srv ordering.Gorums_NodeStreamServer) error {
	finished := make(chan *gorumsMessage, s.opts.buffer)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-finished:
				err := srv.SendMsg(msg)
				if err != nil {
					return
				}
			}
		}
	}()

	for {
		req := newGorumsMessage(false)
		err := srv.RecvMsg(req)
		if err != nil {
			return err
		}
		if handler, ok := s.handlers[req.metadata.MethodID]; ok {
			handler(req, finished)
		}
	}
}

type serverOptions struct {
	buffer   uint
	grpcOpts []grpc.ServerOption
}

// ServerOption is used to change settings for the GorumsServer
type ServerOption func(*serverOptions)

// WithServerBufferSize sets the buffer size for the server.
// A larger buffer may result in higher throughput at the cost of higher latency.
func WithServerBufferSize(size uint) ServerOption {
	return func(o *serverOptions) {
		o.buffer = size
	}
}

func WithGRPCServerOptions(opts ...grpc.ServerOption) ServerOption {
	return func(o *serverOptions) {
		o.grpcOpts = append(o.grpcOpts, opts...)
	}
}

// GorumsServer serves all ordering based RPCs using registered handlers.
type GorumsServer struct {
	srv        *orderingServer
	grpcServer *grpc.Server
}

// NewGorumsServer returns a new instance of GorumsServer.
func NewGorumsServer(opts ...ServerOption) *GorumsServer {
	var serverOpts serverOptions
	for _, opt := range opts {
		opt(&serverOpts)
	}
	serverOpts.grpcOpts = append(serverOpts.grpcOpts, grpc.CustomCodec(newGorumsCodec()))
	s := &GorumsServer{
		srv:        newOrderingServer(&serverOpts),
		grpcServer: grpc.NewServer(serverOpts.grpcOpts...),
	}
	ordering.RegisterGorumsServer(s.grpcServer, s.srv)
	return s
}

// Serve starts serving on the listener.
func (s *GorumsServer) Serve(listener net.Listener) error {
	return s.grpcServer.Serve(listener)
}

// GracefulStop waits for all RPCs to finish before stopping.
func (s *GorumsServer) GracefulStop() {
	s.grpcServer.GracefulStop()
}

// Stop stops the server immediately.
func (s *GorumsServer) Stop() {
	s.grpcServer.Stop()
}
