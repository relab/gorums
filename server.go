package gorums

import (
	"context"
	"fmt"
	"net"
	"slices"
	"sync"

	"github.com/relab/gorums/internal/conn"
	"github.com/relab/gorums/internal/impl"
	"github.com/relab/gorums/internal/stream"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type (
	// Handler processes a request and returns a response.
	Handler func(ServerContext, *Message) (*Message, error)
	// ServerInterceptor intercepts and may modify incoming requests and outgoing responses.
	// It receives a ServerContext, the incoming Message, and a Handler representing
	// the next element in the chain. It returns a Message and an error.
	ServerInterceptor func(ServerContext, *Message, Handler) (*Message, error)
)

// Server serves Gorums calls on registered handlers, and can call its peers
// and connected clients.
type Server struct {
	srv          *stream.Server
	grpcServer   *grpc.Server
	handlers     map[string]Handler
	interceptors []ServerInterceptor
	im           *conn.InboundManager

	mu         sync.Mutex   // guards lis
	lis        net.Listener // active listener; set by Serve, ListenAndServe, or NewLocalServers
	listenAddr string       // address recorded by WithAddr
	outbound   Config       // outbound config; nil if WithPeers was not used
}

// NodeID returns this server's own [Config] node ID, as configured with
// [WithPeers]. It returns 0 if [WithPeers] was not used.
func (s *Server) NodeID() uint32 {
	return s.im.NodeID()
}

// ConnectedPeers returns the subset of [Server.PeerConfig] whose peers are
// currently reachable: the local node, every peer whose connection this
// server established, and, under [WithStreamDedup], every peer whose shared
// connection is attached. It equals [Server.PeerConfig] when all peers are
// connected, and always includes the local node, even before any peers have
// connected. A returned configuration remains valid across connectivity changes.
func (s *Server) ConnectedPeers() Config {
	return s.im.ConnectedPeers()
}

// ConnectedClients returns a [Config] of the clients currently connected to
// this server that can receive back-channel calls. An empty (non-nil)
// configuration is returned when no clients are connected. A returned
// configuration remains valid across connectivity changes.
func (s *Server) ConnectedClients() Config {
	return s.im.ConnectedClients()
}

// WaitForPeers blocks until cond returns true for the current connected-peer
// [Config] (the same view as [Server.ConnectedPeers]), or until ctx is
// cancelled or the server is stopped. cond runs while an internal lock is
// held, so it must not call [Server.ConnectedPeers] or other blocking
// methods; use it only to inspect the given [Config].
func (s *Server) WaitForPeers(ctx context.Context, cond func(Config) bool) error {
	return s.im.WaitForPeers(ctx, cond)
}

// WaitForClients blocks until cond returns true for the current
// connected-client [Config] (the same view as [Server.ConnectedClients]),
// or until ctx is cancelled or the server is stopped. cond runs while an
// internal lock is held, so it must not call [Server.ConnectedClients] or
// other blocking methods; use it only to inspect the given [Config].
func (s *Server) WaitForClients(ctx context.Context, cond func(Config) bool) error {
	return s.im.WaitForClients(ctx, cond)
}

// NewServer returns a new [Server].
//
// Every server tracks the clients that connect to it and can receive
// back-channel calls. These clients are available from [Server.ConnectedClients]
// and [ServerContext.ConnectedClients].
//
// [WithPeers] configures how this server tracks and calls other servers. A
// client that answers calls from the servers it dials instead passes its
// handler-only Server to [NewConfig] via the [WithBackChannel] dial option.
//
// NewServer allocates only in-memory state and does not bind a network
// listener. It panics on configuration errors, such as invalid addresses or
// duplicate nodes, since these are detectable at startup.
func NewServer(opts ...ServerOption) *Server {
	s, err := newServer(opts...)
	if err != nil {
		panic(fmt.Sprintf("gorums: invalid peer configuration: %v", err))
	}
	return s
}

// newServer builds the [Server] that [NewServer] returns, reporting an invalid
// peer configuration as an error instead of panicking.
func newServer(opts ...ServerOption) (*Server, error) {
	var serverOpts serverOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&serverOpts)
		}
	}
	if serverOpts.sendBufferSize == 0 {
		serverOpts.sendBufferSize = conn.DefaultSendBufferSize
	}
	// Allocate s first so it can serve as the self-node handler for the [conn.InboundManager].
	// HandleRequest only accesses s.handlers and s.interceptors, both of which are
	// set below before NewInboundManager is called, so the reference is safe to pass.
	s := &Server{
		grpcServer:   grpc.NewServer(serverOpts.grpcOpts...),
		handlers:     make(map[string]Handler),
		interceptors: serverOpts.interceptors,
		listenAddr:   serverOpts.listenAddr,
	}
	im, err := conn.NewInboundManager(
		serverOpts.myID,
		serverOpts.peerNodes,
		serverOpts.sendBufferSize,
		serverOpts.recvBufferSize,
		serverOpts.onConfigChange,
		s,
	)
	if err != nil {
		s.grpcServer.Stop()
		return nil, err
	}
	s.im = im
	s.srv = stream.NewServer(serverOpts.connectCallback, s.im)
	stream.RegisterGorumsServer(s.grpcServer, s.srv)
	if serverOpts.peerNodes != nil {
		cfg, err := s.newPeerConfig(serverOpts.peerNodes, serverOpts.outboundDialOpts)
		if err != nil {
			s.grpcServer.Stop()
			return nil, err
		}
		s.outbound = cfg
		s.im.SetPeerConfig(cfg)
	}
	return s, nil
}

// newPeerConfig builds the outbound [Config] this server uses to call other
// servers. It installs the server as the back-channel request handler so the
// remote can dispatch requests back over the same connection.
func (s *Server) newPeerConfig(nodes NodeSource, dialOpts []DialOption) (Config, error) {
	opts := append([]DialOption{withServer(s)}, dialOpts...)
	return NewConfig(nodes, opts...)
}

// RegisterHandler registers a request handler for the specified method name.
//
// This function should only be used by generated code.
func (s *Server) RegisterHandler(method string, handler Handler) {
	s.handlers[method] = chainInterceptors(handler, s.interceptors...)
}

// chainInterceptors composes the provided interceptors around the final Handler and
// returns a Handler that executes the chain. The execution order is the same as the
// order of the interceptors in the slice: the first element is executed first, and
// the last element calls the final handler (the server method).
func chainInterceptors(final Handler, interceptors ...ServerInterceptor) Handler {
	if len(interceptors) == 0 {
		return final
	}
	handler := final
	for _, curr := range slices.Backward(interceptors) {
		next := handler
		handler = func(ctx ServerContext, in *Message) (*Message, error) {
			return curr(ctx, in, next)
		}
	}
	return handler
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
// It calls release only when the handler or an interceptor calls
// [ServerContext.Release]. Otherwise the caller releases the request when
// HandleRequest returns, as [stream.RequestHandler] specifies, so the stream's
// next request can run on the same goroutine.
func (s *Server) HandleRequest(ctx context.Context, reqMsg *stream.Message, release func(), send func(*stream.Message)) {
	srvCtx := ServerContext{
		Context: ctx,
		release: release,
		send:    send,
		srv:     s,
	}

	handler, ok := s.handlers[reqMsg.GetMethod()]
	if !ok {
		in := &Message{Message: reqMsg}
		srvCtx.SendMessage(messageWithError(in, nil, status.Errorf(codes.Unimplemented, "no handler registered for method %s", reqMsg.GetMethod())))
		return
	}

	msg, err := impl.UnmarshalRequest(reqMsg)
	in := &Message{Proto: msg, Message: reqMsg}
	if err != nil {
		srvCtx.SendMessage(messageWithError(in, nil, err))
		return
	}

	out, err := handler(srvCtx, in)
	// If there is no response and no error, we do not send anything back to the client.
	// This corresponds to a unidirectional message from client to server, where clients
	// are not expected to receive a response.
	if out == nil && err == nil {
		return
	}
	srvCtx.SendMessage(messageWithError(in, out, err))
}

// Serve serves on the externally supplied listener and records it so that
// [Server.Addr] reports its address and [Server.Stop] closes it. The server
// takes lifecycle responsibility for the listener once Serve is called: Stop
// closes it even though gRPC also closes it when Serve returns.
func (s *Server) Serve(listener net.Listener) error {
	s.setListener(listener)
	return s.grpcServer.Serve(listener)
}

// ListenAndServe binds the address recorded by [WithAddr] and serves on
// it. When the server was created by [NewLocalServers], it serves on the
// preallocated listener instead. It returns a clear error if no listen address
// was configured, or the bind error if the address is invalid or cannot be
// bound. When the configured address uses port 0, [Server.Addr] reports the
// actual bound address after this method creates the listener.
func (s *Server) ListenAndServe() error {
	s.mu.Lock()
	lis := s.lis
	s.mu.Unlock()
	if lis == nil {
		if s.listenAddr == "" {
			return fmt.Errorf("gorums: ListenAndServe requires a listen address; use WithAddr")
		}
		var err error
		lis, err = net.Listen("tcp", s.listenAddr)
		if err != nil {
			return err
		}
		s.setListener(lis)
	}
	return s.grpcServer.Serve(lis)
}

// setListener records lis as the server's active listener.
func (s *Server) setListener(lis net.Listener) {
	s.mu.Lock()
	s.lis = lis
	s.mu.Unlock()
}

// Addr returns the bound listener address once the server has a listener.
// Before binding, it returns the address configured with [WithAddr].
// If neither exists, it returns the empty string.
func (s *Server) Addr() string {
	s.mu.Lock()
	lis := s.lis
	s.mu.Unlock()
	if lis != nil {
		return lis.Addr().String()
	}
	return s.listenAddr
}

// PeerConfig returns the [Config] of the peers configured with [WithPeers],
// or nil if [WithPeers] was not used. Calls on the returned [Config] reach
// the peers over connections this server establishes; calls on the local
// node are served in-process. With [WithStreamDedup], calls may fail with
// [ErrStreamDown] until the peers they target have connected. Call
// [Server.WaitForAll] first to wait for them.
func (s *Server) PeerConfig() Config {
	return s.outbound
}

// WaitForAll blocks until every peer is connected — that is, until
// [Server.ConnectedPeers] equals [Server.PeerConfig] — then returns the peer
// [Config].
//
// It waits only under [WithStreamDedup]. Without stream dedup, or without
// [WithPeers], it returns the current peer [Config] immediately (nil if
// none), since this server dials each of its peers itself.
//
// It returns an error without waiting if the server has no node ID, or if that
// node ID is not one of its configured peers.
func (s *Server) WaitForAll(ctx context.Context) (Config, error) {
	cfg := s.outbound
	if cfg == nil {
		return nil, nil
	}
	required, err := conn.WaitForAllRequired(cfg)
	if err != nil {
		return nil, err
	}
	if !required {
		return cfg, nil
	}
	if err := s.WaitForPeers(ctx, func(connected Config) bool {
		return connected.Size() == cfg.Size()
	}); err != nil {
		return nil, err
	}
	return cfg, nil
}

// GracefulStop waits for all RPCs to finish, then unblocks
// [Server.WaitForPeers], [Server.WaitForClients], and [Server.WaitForAll] with
// [ErrStopped] and closes the peer [Config] built by [WithPeers].
func (s *Server) GracefulStop() {
	s.grpcServer.GracefulStop()
	s.im.Close()
	if s.outbound != nil {
		_ = s.outbound.Close()
	}
}

// Stop stops the server immediately and releases its resources. It unblocks any
// [Server.WaitForPeers], [Server.WaitForClients], and [Server.WaitForAll]
// callers, stops the gRPC server, closes the listener owned by
// [Server.Serve], [Server.ListenAndServe], or [NewLocalServers], and closes the
// peer [Config]. It stops without waiting for in-flight RPCs, since one-way
// methods do not respond. Stop is safe to call before serving starts, and safe
// to call more than once.
func (s *Server) Stop() {
	// Unblock any WaitForPeers / WaitForClients / WaitForAll callers.
	s.im.Close()
	s.grpcServer.Stop()
	s.mu.Lock()
	lis := s.lis
	s.mu.Unlock()
	if lis != nil {
		_ = lis.Close()
	}
	if s.outbound != nil {
		_ = s.outbound.Close()
	}
}

// ServerContext is the context a Gorums server passes to a handler.
// Requests on one stream run one at a time until the handler calls
// [ServerContext.Release] or returns.
type ServerContext struct {
	context.Context
	release func()
	send    func(*stream.Message)
	srv     *Server
}

// Release lets the next request on this handler's stream start, concurrently
// with this handler. Replies to calls the handler makes arrive before and after
// Release, so the handler may call the peer that sent the request. Before
// Release, such a reply arrives only while the stream's dispatch queue has room
// (see [WithBufferSizes]); call Release first if the peer may fill that queue
// while the call is in flight. Call Release once the handler is done with state
// that requests must access one at a time. It is safe to call Release multiple
// times.
func (ctx *ServerContext) Release() {
	if ctx.release != nil {
		ctx.release()
	}
}

// SendMessage sends the given message to the client.
// If marshaling fails, the error is encoded into the response envelope
// and sent to the client; the stream is not closed.
//
// This function should only be used by generated code.
func (ctx *ServerContext) SendMessage(out *Message) {
	// If Proto is set, marshal it to payload before sending.
	if out.Proto != nil && len(out.GetPayload()) == 0 {
		payload, err := proto.Marshal(out.Proto)
		if err == nil {
			out.SetPayload(payload)
		} else {
			// Encode the marshal error into the response envelope; don't close the stream.
			out = messageWithError(nil, out, err)
		}
	}
	if ctx.send != nil {
		ctx.send(out.Message)
	}
}

// PeerConfig returns the [Config] of the peers configured with [WithPeers],
// or nil if [WithPeers] was not used. It is the same configuration as
// [Server.PeerConfig], so a handler can fan out calls to the server's peers.
// Call [ServerContext.Release] before invoking calls on it, so that the next
// request on this stream can start while the handler waits for responses.
func (ctx *ServerContext) PeerConfig() Config {
	if ctx.srv == nil {
		return nil
	}
	return ctx.srv.PeerConfig()
}

// ConnectedClients returns a [Config] of the clients currently connected to
// this server that can receive back-channel calls. It is the same
// configuration as [Server.ConnectedClients]. An empty (non-nil)
// configuration is returned when no clients are connected.
func (ctx *ServerContext) ConnectedClients() Config {
	if ctx.srv == nil {
		return nil
	}
	return ctx.srv.ConnectedClients()
}

// compile-time assertion for interface compliance.
var _ stream.RequestHandler = (*Server)(nil)
