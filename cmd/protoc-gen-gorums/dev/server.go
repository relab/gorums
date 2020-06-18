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
type requestHandler func(*ordering.Message) *ordering.Message

type orderingServer struct {
	handlers map[int32]requestHandler
	opts     serverOptions
}

func newOrderingServer(opts []ServerOption) *orderingServer {
	s := &orderingServer{
		handlers: make(map[int32]requestHandler),
	}
	for _, opt := range opts {
		opt(&s.opts)
	}
	return s
}

func (s *orderingServer) registerHandler(methodID int32, handler requestHandler) {
	s.handlers[methodID] = handler
}

func (s *orderingServer) NodeStream(srv ordering.Gorums_NodeStreamServer) error {
	finished := make(chan *ordering.Message, s.opts.buffer)
	ordered := make(chan struct {
		msg  *ordering.Message
		info methodInfo
	}, s.opts.buffer)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handleMsg := func(req *ordering.Message, info methodInfo) {
		if handler, ok := s.handlers[req.GetMethodID()]; ok {
			if info.oneway {
				handler(req)
			} else {
				finished <- handler(req)
			}
		}
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-finished:
				err := srv.Send(msg)
				if err != nil {
					return
				}
			}
		}
	}()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case req := <-ordered:
				handleMsg(req.msg, req.info)
			}
		}
	}()

	for {
		req, err := srv.Recv()
		if err != nil {
			return err
		}

		if info, ok := orderingMethods[req.MethodID]; ok {
			if info.concurrent {
				go handleMsg(req, info)
			} else {
				ordered <- struct {
					msg  *ordering.Message
					info methodInfo
				}{req, info}
			}
		}
	}
}

type serverOptions struct {
	buffer uint
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

// GorumsServer serves all ordering based RPCs using registered handlers.
type GorumsServer struct {
	srv        *orderingServer
	grpcServer *grpc.Server
}

// NewGorumsServer returns a new instance of GorumsServer.
func NewGorumsServer(opts ...ServerOption) *GorumsServer {
	s := &GorumsServer{
		srv:        newOrderingServer(opts),
		grpcServer: grpc.NewServer(),
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
