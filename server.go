package gorums

import (
	"context"
	"net"

	"github.com/relab/gorums/internal/stream"
	"google.golang.org/grpc"
)

// Type aliases for handler types that now live in internal/stream.
// These preserve backward compatibility for all existing code.
type (
	// Interceptor is a function that can intercept and modify incoming requests
	// and outgoing responses. It receives a ServerCtx, the incoming Message, and
	// a Handler representing the next element in the chain (either another
	// Interceptor or the actual server method). It returns a Message and an error.
	Interceptor = stream.Interceptor
	// Handler is a function that processes a request and returns a response.
	Handler = stream.Handler
	// ServerCtx is a context that is passed from the Gorums server to the handler.
	// It allows the handler to release its lock on the server, allowing the next
	// request to be processed. This happens automatically when the handler returns.
	ServerCtx = stream.ServerCtx
	// Message encapsulates the stream.Message and the actual proto.Message.
	Message = stream.Envelope
)

type serverOptions struct {
	buffer          uint
	grpcOpts        []grpc.ServerOption
	connectCallback func(context.Context)
	interceptors    []Interceptor
	inboundMgr      *InboundManager
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

// WithConnectCallback registers a callback function that will be called by the server
// whenever a node connects or reconnects to the server. This allows access to the node's
// stream context, which is passed to the callback function. The stream context can be
// used to extract the metadata and peer information, if available.
func WithConnectCallback(callback func(context.Context)) ServerOption {
	return func(so *serverOptions) {
		so.connectCallback = callback
	}
}

// WithInboundManager attaches an InboundManager to the server.
// When set, NodeStream calls AcceptPeer for every incoming stream,
// registering known peers and deferring their cleanup when the stream ends.
func WithInboundManager(im *InboundManager) ServerOption {
	return func(o *serverOptions) {
		o.inboundMgr = im
	}
}

// WithInterceptors registers server-side interceptors to run for every incoming request.
// Interceptors are executed for each registered handler. Interceptors may modify both
// the request and/or response messages, or perform additional actions before or after
// calling the next handler in the chain. Interceptors are executed in the order they are
// provided: the first element is executed first, and the last element calls the actual
// server method handler.
func WithInterceptors(i ...Interceptor) ServerOption {
	return func(opts *serverOptions) {
		opts.interceptors = append(opts.interceptors, i...)
	}
}

// Server serves all ordering based RPCs using registered handlers.
type Server struct {
	srv          *stream.Server
	grpcServer   *grpc.Server
	interceptors []Interceptor
}

// NewServer returns a new instance of [gorums.Server].
func NewServer(opts ...ServerOption) *Server {
	var serverOpts serverOptions
	for _, opt := range opts {
		opt(&serverOpts)
	}
	var acceptor stream.PeerAcceptor
	if serverOpts.inboundMgr != nil {
		acceptor = serverOpts.inboundMgr
	}
	s := &Server{
		srv:          stream.NewServer(serverOpts.buffer, acceptor, serverOpts.connectCallback),
		grpcServer:   grpc.NewServer(serverOpts.grpcOpts...),
		interceptors: serverOpts.interceptors,
	}
	stream.RegisterGorumsServer(s.grpcServer, s.srv)
	return s
}

// RegisterHandler registers a request handler for the specified method name.
//
// This function should only be used by generated code.
func (s *Server) RegisterHandler(method string, handler Handler) {
	s.srv.RegisterHandler(method, stream.ChainInterceptors(handler, s.interceptors...))
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
