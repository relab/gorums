package gorums

import (
	"context"
	"net"

	"github.com/relab/gorums/ordering"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// requestHandler is used to fetch a response message based on the request.
// A requestHandler should receive a message from the server, unmarshal it into
// the proper type for that Method's request type, call a user provided Handler,
// and return a marshaled result to the server.
type requestHandler func(context.Context, *Message, chan<- *Message)

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

// WrapMessage wraps the metadata, response and error status in a gorumsMessage
func WrapMessage(md *ordering.Metadata, resp protoreflect.ProtoMessage, err error) *Message {
	errStatus, ok := status.FromError(err)
	if !ok {
		errStatus = status.New(codes.Unknown, err.Error())
	}
	md.Status = errStatus.Proto()
	return &Message{Metadata: md, Message: resp}
}

// NodeStream handles a connection to a single client. The stream is aborted if there
// is any error with sending or receiving.
func (s *orderingServer) NodeStream(srv ordering.Gorums_NodeStreamServer) error {
	finished := make(chan *Message, s.opts.buffer)
	ctx := srv.Context()

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
		req := newGorumsMessage(gorumsRequestType)
		err := srv.RecvMsg(req)
		if err != nil {
			return err
		}
		if handler, ok := s.handlers[req.Metadata.MethodID]; ok {
			handler(ctx, req, finished)
		}
	}
}

type serverOptions struct {
	buffer   uint
	grpcOpts []grpc.ServerOption
}

// ServerOption is used to change settings for the GorumsServer
type ServerOption func(*serverOptions)

// WithReceiveBufferSize sets the buffer size for the server.
// A larger buffer may result in higher throughput at the cost of higher latency.
func WithReceiveBufferSize(size uint) ServerOption {
	return func(o *serverOptions) {
		o.buffer = size
	}
}

// WithGRPCServerOptions allows to set gRPC options for the server.
func WithGRPCServerOptions(opts ...grpc.ServerOption) ServerOption {
	return func(o *serverOptions) {
		o.grpcOpts = append(o.grpcOpts, opts...)
	}
}

// Server serves all ordering based RPCs using registered handlers.
type Server struct {
	srv        *orderingServer
	grpcServer *grpc.Server
}

// NewServer returns a new instance of GorumsServer.
// This function is intended for internal Gorums use.
// You should call `NewServer` in the generated code instead.
func NewServer(opts ...ServerOption) *Server {
	var serverOpts serverOptions
	for _, opt := range opts {
		opt(&serverOpts)
	}
	s := &Server{
		srv:        newOrderingServer(&serverOpts),
		grpcServer: grpc.NewServer(serverOpts.grpcOpts...),
	}
	ordering.RegisterGorumsServer(s.grpcServer, s.srv)
	return s
}

func (s *Server) RegisterHandler(methodID int32, handler requestHandler) {
	s.srv.handlers[methodID] = handler
}

// Serve starts serving on the listener.
func (s *Server) Serve(listener net.Listener) error {
	return s.grpcServer.Serve(listener)
}

// GracefulStop waits for all RPCs to finish before stopping.
func (s *Server) GracefulStop() {
	s.grpcServer.GracefulStop()
}

// Stop stops the server immediately.
func (s *Server) Stop() {
	s.grpcServer.Stop()
}
