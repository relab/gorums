package gorums

import (
	"cmp"
	"context"
	"fmt"
	"log"
	"net"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/relab/gorums/internal/conn"
	"github.com/relab/gorums/internal/impl"
	"github.com/relab/gorums/internal/stream"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type (
	// Handler processes a request. It either sends its responses or returns an
	// error: a unary handler calls send once, a streaming handler calls send
	// once for each response, and a one-way handler never calls send. If the
	// handler returns an error before it calls send, the caller receives the
	// error as the call's reply. An error returned after send is a programming
	// error: the server discards it and logs it, see [WithServerLogger].
	Handler func(ctx ServerContext, in *Message, send func(proto.Message)) error
	// ServerInterceptor intercepts and may modify incoming requests and outgoing
	// responses of every call type. It receives the request and the send
	// function of the call, and calls next to continue the chain. To observe or
	// modify responses, it passes next a send function that wraps send. To
	// reject a request, it returns an error without calling next. An error it
	// returns after a response was sent is discarded and logged, as for a
	// [Handler].
	ServerInterceptor func(ctx ServerContext, in *Message, send func(proto.Message), next Handler) error
)

// Server serves Gorums calls on registered handlers, and can call its peers
// and connected clients.
type Server struct {
	srv          *stream.Server
	grpcServer   *grpc.Server
	handlers     map[string]Handler
	interceptors []ServerInterceptor
	logger       *log.Logger // logs programming errors in handlers; never nil
	im           *conn.InboundManager
	drainTimeout time.Duration // bounds GracefulStop; never zero

	mu         sync.Mutex   // guards lis
	lis        net.Listener // active listener; set by Serve, ListenAndServe, or NewLocalServers
	listenAddr string       // address recorded by WithAddr
	outbound   Config       // outbound config; nil if WithPeers was not used
	closePeers func()       // closes outbound's pool; nil if WithPeers was not used
}

// NodeID returns this server's own [Config] node ID, as configured with
// [WithPeers]. It returns 0 if [WithPeers] was not used.
func (s *Server) NodeID() ID {
	return s.im.NodeID()
}

// ConnectedPeers returns the subset of [Server.PeerConfig] whose peers are
// currently connected. It always includes the local node, and it equals
// [Server.PeerConfig] when all peers are connected. A returned configuration
// remains valid across connectivity changes.
//
// A peer is connected while the stream that this server uses to call it is
// open and accepted. Without [WithStreamDedup], this server opens that stream,
// and the peer accepts it. With it, the server with the lower ID opens the
// stream, and the other server accepts it.
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
		panic(err.Error())
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
	if serverOpts.recvBufferSize == 0 {
		serverOpts.recvBufferSize = conn.DefaultRecvBufferSize
	}
	if serverOpts.logger == nil {
		serverOpts.logger = log.Default()
	}
	verify, err := conn.NewPeerVerifier(serverOpts.peerNodes)
	if err != nil {
		return nil, err
	}
	// Allocate s first so it can serve as the self-node handler for the [conn.InboundManager].
	// HandleRequest only accesses s.handlers, s.interceptors, and s.logger, which are
	// set below before NewInboundManager is called, so the reference is safe to pass.
	s := &Server{
		grpcServer:   grpc.NewServer(serverOpts.grpcOpts...),
		handlers:     make(map[string]Handler),
		interceptors: serverOpts.interceptors,
		logger:       serverOpts.logger,
		listenAddr:   serverOpts.listenAddr,
		drainTimeout: cmp.Or(serverOpts.drainTimeout, defaultDrainTimeout),
	}
	im, err := conn.NewInboundManager(
		serverOpts.myID,
		serverOpts.peerNodes,
		serverOpts.sendBufferSize,
		serverOpts.recvBufferSize,
		serverOpts.onConfigChange,
		s,
		verify,
	)
	if err != nil {
		s.grpcServer.Stop()
		return nil, err
	}
	s.im = im
	s.srv = stream.NewServer(serverOpts.connectCallback, s.im)
	stream.RegisterGorumsServer(s.grpcServer, s.srv)
	if serverOpts.peerNodes != nil {
		cfg, closePeers, err := s.newPeerConfig(serverOpts.peerNodes, serverOpts.outboundDialOpts)
		if err != nil {
			s.grpcServer.Stop()
			return nil, err
		}
		s.outbound = cfg
		s.closePeers = closePeers
		s.im.SetPeerConfig(cfg)
	}
	return s, nil
}

// newPeerConfig builds the outbound [Config] this server uses to call other
// servers, and the function that closes its connection pool. It installs the
// server as the back-channel request handler so the remote can dispatch
// requests back over the same connection.
func (s *Server) newPeerConfig(nodes NodeSource, dialOpts []DialOption) (Config, func(), error) {
	opts := append([]DialOption{withServer(s)}, dialOpts...)
	return NewConfig(nodes, opts...)
}

// RegisterHandler registers a request handler for the specified method name.
// Generated registration functions use it; applications can also register
// handlers for back-channel calls.
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
		handler = func(ctx ServerContext, in *Message, send func(proto.Message)) error {
			return curr(ctx, in, send, next)
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
// For requests that reach the handler, send is invoked once for each response
// the handler sends. If the handler returns an error before it sends a
// response, send is invoked once with the error. An error returned after a
// response is discarded and logged with the server's logger.
//
// This is the "default interceptor"; it is the first and last handler in the chain.
// It calls release only when the handler or an interceptor calls
// [ServerContext.Release]. Otherwise the caller releases the request when
// HandleRequest returns, as [stream.RequestHandler] specifies, so the stream's
// next request can run on the same goroutine.
func (s *Server) HandleRequest(ctx context.Context, senderID ID, reqMsg *stream.Message, release func(), send func(*stream.Message)) {
	srvCtx := ServerContext{
		Context:  ctx,
		senderID: senderID,
		release:  release,
		srv:      s,
	}

	handler, ok := s.handlers[reqMsg.GetMethod()]
	if !ok {
		send(newResponse(reqMsg, nil, status.Errorf(codes.Unimplemented, "no handler registered for method %s", reqMsg.GetMethod())))
		return
	}

	msg, err := impl.UnmarshalRequest(reqMsg)
	if err != nil {
		send(newResponse(reqMsg, nil, err))
		return
	}

	call := &inboundCall{in: Message{Proto: msg, Message: reqMsg}}
	err = handler(srvCtx, &call.in, func(resp proto.Message) {
		call.sent.Store(true)
		send(newResponse(reqMsg, resp, nil))
	})
	if err == nil {
		return
	}
	if call.sent.Load() {
		s.logger.Printf("gorums: programming error: the handler for %s returned an error after it called send; the error is discarded: %v",
			reqMsg.GetMethod(), err)
		return
	}
	send(newResponse(reqMsg, nil, err))
}

// inboundCall holds the state of one request in [Server.HandleRequest]. It
// keeps the request and the sent flag in one allocation.
type inboundCall struct {
	in   Message
	sent atomic.Bool // set when the handler calls send
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

// GracefulStop stops the server after it has finished the requests it
// accepted, then unblocks [Server.WaitForPeers], [Server.WaitForClients], and
// [Server.WaitForAll] with [ErrStopped] and closes the peer [Config] built by
// [WithPeers].
//
// GracefulStop first refuses new streams and tells the peers on the streams
// it serves to stop sending. A peer then fails the requests it has not sent
// with [ErrServerDraining]. The server finishes every accepted handler,
// including one-way handlers and handlers that called [ServerContext.Release],
// and sends the replies before it closes the stream. A peer that does not
// implement this protocol is not waited for.
//
// Draining covers the streams that this server accepts. Calls to the local
// node and streams that this server dialed use the transport's own shutdown.
// The wait is bounded by [WithDrainTimeout]; when it ends, the server stops
// immediately as [Server.Stop] does.
func (s *Server) GracefulStop() {
	deadline := time.Now().Add(s.drainTimeout)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	if !s.srv.Drain(ctx) {
		s.Stop()
		return
	}
	forced := time.AfterFunc(time.Until(deadline), s.grpcServer.Stop)
	s.grpcServer.GracefulStop()
	forced.Stop()
	s.im.Close()
	if s.closePeers != nil {
		s.closePeers()
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
	if s.closePeers != nil {
		s.closePeers()
	}
}

// ServerContext is the context a Gorums server passes to a handler.
// Requests on one stream run one at a time until the handler calls
// [ServerContext.Release] or returns.
type ServerContext struct {
	context.Context
	senderID ID
	release  func()
	srv      *Server
}

// SenderID returns the ID of the node that sent the request, as this server
// identifies that node:
//   - a peer configured with [WithPeers] has its configured ID;
//   - a back-channel client has the ID that this server assigned to it, as in
//     [Server.ConnectedClients]; the ID changes when the client reconnects;
//   - a request on a connection that this server dialed has the dialed
//     node's ID;
//   - a request that this server sends to itself has this server's own ID.
//
// SenderID returns 0 for a regular client that announces no ID.
// A nonzero ID outside the server's peer configuration is an unknown peer ID.
// The server handles a stream that claims an unknown peer ID as follows:
//   - With TLS peer identities configured through [WithPeers], the server
//     refuses the stream before any handler runs.
//   - Without TLS peer identities, the server serves the stream as an untracked
//     client, and SenderID returns 0.
//
// The sender is the last hop: when a message travels through several nodes,
// SenderID identifies the node that forwarded it. The sender asserts its ID
// when it connects. A server whose peers declare [TLSIdentity] authenticates
// every peer, both on the streams that peers open and on the connections that
// it dials.
func (ctx *ServerContext) SenderID() ID {
	return ctx.senderID
}

// Release ends the ordered part of the handler and lets the next request on
// this handler's stream start. Code before Release runs in per-sender FIFO
// order: the requests of one stream pass it one at a time, in arrival order.
// Code after Release runs concurrently with the handlers of later requests, so
// it must protect shared state, for example with a mutex. A mutex gives mutual
// exclusion but not arrival order. A handler that does not call Release
// releases when it returns.
//
// To keep a nested call in FIFO order, dispatch it before Release and wait for
// it after Release, for example with [OnewayCall.Async] or [Call.Async];
// both send before they return. A call dispatched
// after Release may overtake the same call from the next request's handler. A
// reply that the handler waits for before Release arrives only while the
// stream's dispatch queue has room (see [WithBufferSizes]). It is safe to call
// Release multiple times.
func (ctx *ServerContext) Release() {
	if ctx.release != nil {
		ctx.release()
	}
}

// PeerConfig returns the [Config] of the peers configured with [WithPeers],
// or nil if [WithPeers] was not used. It is the same configuration as
// [Server.PeerConfig], so a handler can fan out calls to the server's peers.
// Calls on it follow the ordering rule of [ServerContext.Release]: dispatch a
// call before Release to keep it in FIFO order, and wait for its responses
// after Release, so that the next request on this stream can start meanwhile.
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

// Compile-time assertion: Server implements [stream.RequestHandler].
var _ stream.RequestHandler = (*Server)(nil)
