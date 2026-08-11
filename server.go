package gorums

import (
	"context"
	"fmt"
	"net"

	"github.com/relab/gorums/internal/stream"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// serverOptions contains configuration options for creating a new Server.
type serverOptions struct {
	recvBufferSize  uint
	sendBufferSize  uint
	grpcOpts        []grpc.ServerOption
	connectCallback func(context.Context)
	interceptors    []Interceptor
	// Peer management options
	myID             uint32
	peerNodes        NodeListOption      // Peers to track as they connect; set by WithPeers.
	onConfigChange   func(Configuration) // Callback registered via WithPeerChange.
	outboundNodes    NodeListOption      // Nodes this server calls; set by WithPeers.
	outboundDialOpts []DialOption
}

// ServerOption is used to change settings for the GorumsServer
type ServerOption func(*serverOptions)

// WithBufferSizes configures the send and receive buffer sizes for the server.
// The receiveSize is the capacity of the queue carrying finished handler
// responses to the goroutine that writes them back on the stream; it bounds how
// many requests one connection can have in flight. Its default is 0
// (unbuffered), which lets a connection read its next request only once the
// current handler's response has been picked up.
//
// The sendSize controls the capacity of the server's per-node send queue for
// outgoing peer messages in the reverse direction, with the same full-queue
// semantics as [WithSendBufferSize]: two-way requests fail fast, one-way
// requests and responses block. A sendSize of 0 selects [DefaultSendBufferSize].
// Larger values may increase throughput at the cost of higher latency.
func WithBufferSizes(receiveSize, sendSize uint) ServerOption {
	return func(o *serverOptions) {
		o.recvBufferSize = receiveSize
		o.sendBufferSize = sendSize
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

// WithPeers configures the server to both track and call a fixed set of peer
// servers. The myID parameter is this server's own node ID; it is always
// present in the peer [Configuration] so that quorum thresholds account for
// the local replica, and calls to it are served in-process without a network
// round-trip.
//
// The server builds the peer [Configuration] itself, available from
// [Server.PeerConfig], applying opts to the connections it establishes. To
// observe which peers are currently reachable, use [Server.Config].
//
// The returned option only records the peer set; the [NewServer] call that
// receives it panics if the node source is invalid, for example if it contains
// a duplicate or malformed address.
func WithPeers(myID uint32, nodes NodeListOption, opts ...DialOption) ServerOption {
	return func(o *serverOptions) {
		o.myID = myID
		o.peerNodes = nodes
		o.outboundNodes = nodes
		o.outboundDialOpts = append(o.outboundDialOpts, opts...)
	}
}

// WithPeerChange registers a callback invoked after each change to the peer
// [Configuration] (peer connect or disconnect). The callback runs while
// internal locks are held, so it must not call [Server.Config] or other
// blocking methods; use it only to signal or copy, not for long work.
func WithPeerChange(callback func(Configuration)) ServerOption {
	return func(o *serverOptions) {
		o.onConfigChange = callback
	}
}

// Server serves all ordering based RPCs using registered handlers.
type Server struct {
	srv          *stream.Server
	grpcServer   *grpc.Server
	handlers     map[string]Handler
	interceptors []Interceptor
	outbound     Configuration // peer config built by WithPeers; nil if unused
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
		if opt != nil {
			opt(&serverOpts)
		}
	}
	if serverOpts.sendBufferSize == 0 {
		serverOpts.sendBufferSize = DefaultSendBufferSize
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
		serverOpts.peerNodes,
		serverOpts.sendBufferSize,
		serverOpts.onConfigChange,
		s,
	)
	s.srv = stream.NewServer(serverOpts.recvBufferSize, serverOpts.connectCallback, s.inboundManager)
	stream.RegisterGorumsServer(s.grpcServer, s.srv)
	if serverOpts.outboundNodes != nil {
		cfg, err := s.newPeerConfig(serverOpts.outboundNodes, serverOpts.outboundDialOpts)
		if err != nil {
			panic(fmt.Sprintf("gorums: invalid peer configuration: %v", err))
		}
		s.outbound = cfg
	}
	return s
}

// newPeerConfig builds the outbound [Configuration] this server uses to call
// other servers. It installs the server as the back-channel request handler so
// the remote can dispatch requests back over the same connection.
func (s *Server) newPeerConfig(nodes NodeListOption, dialOpts []DialOption) (Configuration, error) {
	opts := append([]DialOption{withServer(s)}, dialOpts...)
	return NewConfig(nodes, opts...)
}

// PeerConfig returns the [Configuration] of the peers configured with
// [WithPeers], or nil if [WithPeers] was not used. Calls on the returned
// configuration reach the peers over connections this server establishes;
// calls on the local node are served in-process.
func (s *Server) PeerConfig() Configuration {
	return s.outbound
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
		srvCtx.SendMessage(MessageWithError(in, nil, status.Errorf(codes.Unimplemented, "no handler registered for method %s", reqMsg.GetMethod())))
		return
	}

	msg, err := unmarshalRequest(reqMsg)
	in := &Message{Msg: msg, Message: reqMsg}
	if err != nil {
		srvCtx.SendMessage(MessageWithError(in, nil, err))
		return
	}

	out, err := handler(srvCtx, in)
	// If there is no response and no error, we do not send anything back to the client.
	// This corresponds to a unidirectional message from client to server, where clients
	// are not expected to receive a response.
	if out == nil && err == nil {
		return
	}
	srvCtx.SendMessage(MessageWithError(in, out, err))
}

// Serve starts serving on the listener.
func (s *Server) Serve(listener net.Listener) error {
	return s.grpcServer.Serve(listener)
}

// GracefulStop waits for all RPCs to finish before stopping.
func (s *Server) GracefulStop() {
	s.grpcServer.GracefulStop()
}

// Stop stops the server immediately and releases the resources it owns,
// including the peer [Configuration] built by [WithPeers]. It does not use
// gRPC graceful stop, because one-way methods do not respond and would block
// indefinitely. Stop is safe to call more than once.
func (s *Server) Stop() {
	s.grpcServer.Stop()
	if s.outbound != nil {
		_ = s.outbound.Close()
	}
}

// compile-time assertion for interface compliance.
var _ stream.RequestHandler = (*Server)(nil)
