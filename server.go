package gorums

import (
	"context"
	"net"

	"github.com/relab/gorums/internal/stream"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// serverOptions contains configuration options for creating a new Server.
type serverOptions struct {
	buffer          uint
	grpcOpts        []grpc.ServerOption
	connectCallback func(context.Context)
	interceptors    []Interceptor
	// Peer management options
	myID           uint32
	peerOpt        NodeListOption
	peerSendBuffer uint
	onConfigChange func(Configuration)
}

// ServerOption is used to change settings for the GorumsServer
type ServerOption func(*serverOptions)

func (ServerOption) isOption() {}

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

// WithConfig configures the server to track a fixed set of peer servers.
// When a recognized peer connects, it is included in the [Configuration]
// returned by [Config]. The myID parameter is this server's own NodeID;
// it is always present in the [Config] so that quorum thresholds account
// for the local replica.
//
// The optional onChange callback is called after each change to the known
// peer [Configuration] (server peer connect or disconnect). It is invoked
// while any internal locks are held, so it must not call [Config] or other
// blocking methods. Use it only to signal or copy; do not perform long
// work inside the callback.
func WithConfig(myID uint32, opt NodeListOption, onChange ...func(Configuration)) ServerOption {
	return func(o *serverOptions) {
		o.myID = myID
		o.peerOpt = opt
		if len(onChange) > 0 {
			o.onConfigChange = onChange[0]
		}
	}
}

// WithPeerSendBufferSize sets the size of the per-node send buffer for channels
// created when inbound peers connect. A larger buffer may increase throughput
// for asynchronous call types at the cost of latency. The default is 0 (unbuffered).
func WithPeerSendBufferSize(size uint) ServerOption {
	return func(o *serverOptions) {
		o.peerSendBuffer = size
	}
}

// Server serves all ordering based RPCs using registered handlers.
type Server struct {
	srv          *stream.Server
	grpcServer   *grpc.Server
	handlers     map[string]Handler
	interceptors []Interceptor
	*inboundManager
}

// NewServer returns a new instance of [Server].
//
// The server tracks connected clients that are capable of receiving reverse-direction
// calls from the server; these clients are accessible via [ServerCtx.ClientConfig]
// and [Server.ClientConfig]. If [WithConfig] is provided, the server additionally
// tracks a fixed set of peer servers, which are accessible via [ServerCtx.Config]
// and [Server.Config].
//
// Panics on configuration errors (invalid addresses, duplicate nodes, etc.)
// since these are programmer errors detectable at startup.
func NewServer(opts ...ServerOption) *Server {
	var serverOpts serverOptions
	for _, opt := range opts {
		opt(&serverOpts)
	}
	// Allocate s first so it can serve as the selfHandler for the inboundManager.
	// HandleRequest only accesses s.handlers and s.interceptors, both of which are
	// set below before newInboundManager is called, so the reference is safe to pass.
	s := &Server{
		grpcServer:   grpc.NewServer(serverOpts.grpcOpts...),
		handlers:     make(map[string]Handler),
		interceptors: serverOpts.interceptors,
	}
	s.inboundManager = newInboundManager(
		serverOpts.myID,
		serverOpts.peerOpt,
		serverOpts.peerSendBuffer,
		serverOpts.onConfigChange,
		s,
	)
	s.srv = stream.NewServer(serverOpts.buffer, serverOpts.connectCallback, s.inboundManager)
	stream.RegisterGorumsServer(s.grpcServer, s.srv)
	return s
}

// RegisterHandler registers a request handler for the specified method name.
//
// This function should only be used by generated code.
func (s *Server) RegisterHandler(method string, handler Handler) {
	s.handlers[method] = chainInterceptors(handler, s.interceptors...)
}

// HandleRequest processes an incoming request from the stream, dispatching it
// to the appropriate registered handler. It serves as the bridge between the
// multiplexing in the stream package and the RPC logic in the gorums package.
//
// send is invoked in two infrastructure-level error cases regardless of call type:
// no handler is registered for the method, or the request cannot be unmarshaled.
// For requests that reach the handler: one-way handlers return nil, nil and send
// is not invoked; two-way handlers return a response which is delivered via send.
//
// This is the "default interceptor"; it is the first and last handler in the chain.
// It is responsible for releasing the mutex when the handler chain is done,
// unless already released by the handler itself, or an interceptor in the chain.
func (s *Server) HandleRequest(ctx context.Context, reqMsg *stream.Message, release func(), send func(*stream.Message)) {
	srvCtx := ServerCtx{
		Context: ctx,
		release: release,
		send:    send,
		srv:     s,
	}
	defer srvCtx.Release()

	handler, ok := s.handlers[reqMsg.GetMethod()]
	if !ok {
		in := &Message{Message: reqMsg}
		_ = srvCtx.SendMessage(MessageWithError(in, nil, status.Errorf(codes.Unimplemented, "no handler registered for method %s", reqMsg.GetMethod())))
		return
	}

	msg, err := unmarshalRequest(reqMsg)
	in := &Message{Msg: msg, Message: reqMsg}
	if err != nil {
		_ = srvCtx.SendMessage(MessageWithError(in, nil, err))
		return
	}

	out, err := handler(srvCtx, in)
	// If there is no response and no error, we do not send anything back to the client.
	// This corresponds to a unidirectional message from client to server, where clients
	// are not expected to receive a response.
	if out == nil && err == nil {
		return
	}
	_ = srvCtx.SendMessage(MessageWithError(in, out, err))
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

// compile-time assertion for interface compliance.
var _ stream.RequestHandler = (*Server)(nil)
