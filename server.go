package gorums

import (
	"context"
	"net"

	"github.com/relab/gorums/internal/stream"
	"google.golang.org/grpc"
)

// serverOptions contains configuration options for creating a new Server.
type serverOptions struct {
	buffer          uint
	grpcOpts        []grpc.ServerOption
	connectCallback func(context.Context)
	interceptors    []Interceptor
	// Peer management options (create InboundManager internally)
	myID            uint32
	peerOpt         NodeListOption
	clientPeers     bool
	peerSendBuffer  uint
	onConfigChange  func(Configuration)
	onClientsChange func(Configuration)
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

// WithConfig configures the server to track known peer servers identified by their
// NodeID metadata. When a server connects as a client with a recognized NodeID, the
// server registers the node and includes it in the [Configuration] returned by [Config].
// The myID parameter is this server's own NodeID; it is always present in the [Config]
// so that quorum thresholds account for the local replica.
// The optional onChange callback is called after each change to the known peer [Configuration]
// (server peer connect or disconnect). It is invoked while the manager's lock is held,
// so it must not call [Config] or other blocking methods.
// Use it only to signal or copy; do not perform long work inside the callback.
func WithConfig(myID uint32, opt NodeListOption, onChange ...func(Configuration)) ServerOption {
	return func(o *serverOptions) {
		o.myID = myID
		o.peerOpt = opt
		if len(onChange) > 0 {
			o.onConfigChange = onChange[0]
		}
	}
}

// WithClientConfig enables the server to accept unknown clients (those without
// a recognized NodeID). Each unknown client is assigned a sequential auto-generated
// ID and included in the [Configuration] returned by [ClientConfig]. Client nodes are
// removed when they disconnect. Can be combined with [WithConfig] for mixed mode.
// The optional onChange callback is called after each change to the [ClientConfig]
// (client peer connect or disconnect). It is invoked while the manager's lock is held,
// so it must not call [ClientConfig] or other blocking methods.
// Use it only to signal or copy; do not perform long work inside the callback.
func WithClientConfig(onChange ...func(Configuration)) ServerOption {
	return func(o *serverOptions) {
		o.clientPeers = true
		if len(onChange) > 0 {
			o.onClientsChange = onChange[0]
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
	inboundMgr   *InboundManager // nil if no peers configured
}

// NewServer returns a new instance of [Server].
// If [WithConfig] or [WithClientConfig] options are provided, an InboundManager is
// created internally to track connected peers.
// Panics on configuration errors (invalid addresses, duplicate nodes, etc.)
// since these are programmer errors detectable at startup.
func NewServer(opts ...ServerOption) *Server {
	var serverOpts serverOptions
	for _, opt := range opts {
		opt(&serverOpts)
	}
	var acceptor *InboundManager
	if serverOpts.peerOpt != nil || serverOpts.clientPeers {
		acceptor = newInboundManager(serverOpts.myID, serverOpts.peerOpt, serverOpts.peerSendBuffer, serverOpts.onConfigChange, serverOpts.onClientsChange, serverOpts.clientPeers)
	}
	s := &Server{
		grpcServer:   grpc.NewServer(serverOpts.grpcOpts...),
		handlers:     make(map[string]Handler),
		interceptors: serverOpts.interceptors,
		inboundMgr:   acceptor,
	}
	s.srv = stream.NewServer(serverOpts.buffer, acceptor, serverOpts.connectCallback, s)
	stream.RegisterGorumsServer(s.grpcServer, s.srv)
	return s
}

// Config returns a [Configuration] of all connected known peers, plus this node.
// The returned slice is replaced atomically on each connect/disconnect;
// retaining a reference to an old configuration is safe.
// Returns nil if no peer tracking is configured.
func (s *Server) Config() Configuration {
	if s.inboundMgr == nil {
		return nil
	}
	return s.inboundMgr.Config()
}

// ClientConfig returns a [Configuration] of all connected client peers.
// The returned slice is replaced atomically on each connect/disconnect;
// retaining a reference to an old value is safe.
// Returns nil if no peer tracking is configured.
func (s *Server) ClientConfig() Configuration {
	if s.inboundMgr == nil {
		return nil
	}
	return s.inboundMgr.ClientConfig()
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
		// No handler registered: we just ignore the message and release the lock.
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
