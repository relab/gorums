package gorums

import (
	"context"

	"github.com/relab/gorums/internal/conn"
	"google.golang.org/grpc"
)

// serverOptions contains configuration options for creating a new Server.
type serverOptions struct {
	recvBufferSize  uint
	sendBufferSize  uint
	grpcOpts        []grpc.ServerOption
	connectCallback func(context.Context)
	interceptors    []ServerInterceptor
	// Peer management options
	myID             ID
	peerNodes        NodeSource   // Peers this server tracks and calls; set by WithPeers.
	onConfigChange   func(Config) // Callback registered via [WithPeerChange]; invoked after each connected-peer config change.
	listenAddr       string       // Listener address recorded by WithAddr; bound by ListenAndServe.
	outboundDialOpts []DialOption
}

// ServerOption configures a [Server].
type ServerOption func(*serverOptions)

// WithBufferSizes configures the send and receive buffer sizes for the server.
// The receiveSize is the number of requests received on each inbound stream
// that can wait for the running handler; receiving from the stream waits while
// that many are queued. Replies on that stream wait as well, so a handler that
// waits for a reply from the sending peer before [ServerContext.Release] waits
// until its context ends if the peer fills the queue first. Dispatching the
// call before Release and waiting after it avoids this. A receiveSize of 0
// selects [DefaultRecvBufferSize].
//
// The sendSize controls the capacity of the server's per-node send queue for
// outgoing peer messages, with the same full-queue semantics as
// [WithDialBufferSizes]. Two-way requests fail fast when the queue is full.
// One-way requests wait for space. A reply never waits: it fails fast, or is
// dropped when it has no channel to report the error on. A sendSize of 0
// selects [DefaultSendBufferSize]. Larger values may increase throughput at
// the cost of higher latency.
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

// WithServerInterceptors registers server-side interceptors to run for every incoming request.
// Interceptors are executed for each registered handler. Interceptors may modify both
// the request and/or response messages, or perform additional actions before or after
// calling the next handler in the chain. Interceptors are executed in the order they are
// provided: the first element is executed first, and the last element calls the actual
// server method handler.
func WithServerInterceptors(i ...ServerInterceptor) ServerOption {
	return func(opts *serverOptions) {
		opts.interceptors = append(opts.interceptors, i...)
	}
}

// WithPeers configures the fixed set of peer servers for this server.
// myID is this server's own node ID and must be an entry in nodes.
//
// The peers become available from [Server.PeerConfig], and to handlers from
// [ServerContext.PeerConfig], backed by connections this server establishes
// using the given dial options. This is the configuration to invoke calls on.
// It includes the local node, so quorum thresholds count the local replica;
// calls to it are served in-process without a network round-trip. To observe
// which peers are currently connected, use [Server.ConnectedPeers].
//
// The returned option only records the peer set; the [NewServer] call that
// receives it panics if the node source is invalid, for example if it
// contains a duplicate or malformed address.
//
// If any node supplied through [WithNodes] implements [TLSIdentity], the server
// authenticates every peer, in both directions, before callbacks or handlers
// run. Every node must then supply a unique identity that a certificate SAN can
// match; see [TLSIdentity]. The servers must also use mutual TLS with the
// cluster's CA:
//   - Set the server's credentials with grpc.Creds in [WithGRPCServerOptions].
//     They must present this replica's certificate and verify client
//     certificates.
//   - Set the dialer's credentials in opts with [WithTransportCredentials].
//     They must present the same certificate. [NewServer] panics without them.
//   - Make each replica's certificate valid for both client and server
//     authentication.
//
// The server checks the certificate on each inbound stream that claims a peer
// ID, and on each connection that it dials to a peer. It does not check
// clients that claim no peer ID. If a check fails, the server refuses the
// stream or closes the connection. The dialer's calls then fail with
// codes.Unauthenticated, and its [Node.LastErr] reports the reason. The dialer
// waits a growing delay before each new attempt.
func WithPeers(myID ID, nodes NodeSource, opts ...DialOption) ServerOption {
	return func(o *serverOptions) {
		o.myID = myID
		o.peerNodes = nodes
		o.outboundDialOpts = append(o.outboundDialOpts, opts...)
	}
}

// WithPeerChange registers a callback invoked after each change to the
// connected-peer [Config] (peer connect or disconnect). The callback runs
// while internal locks are held, so it must not call [Server.ConnectedPeers]
// or other blocking methods; use it only to signal or copy, not for long work.
func WithPeerChange(callback func(Config)) ServerOption {
	return func(o *serverOptions) {
		o.onConfigChange = callback
	}
}

// WithAddr records the address that [Server.ListenAndServe] binds.
// It only stores the address; nothing is resolved or bound until
// [Server.ListenAndServe] is called.
func WithAddr(addr string) ServerOption {
	return func(o *serverOptions) {
		o.listenAddr = addr
	}
}

// WithStreamDedup makes each pair of peers share a single connection for
// calls in both directions instead of using one connection per direction.
// It applies only to the peer [Config] built by [WithPeers].
//
// The lower-ID peer of each pair owns the shared connection: it dials the
// higher-ID peer and re-establishes the connection whenever it drops. The
// higher-ID peer never dials; its calls are sent over the connection its
// peer opened, and fail with [ErrStreamDown] while that connection is down.
// Call [Server.WaitForAll] once at startup to wait until the shared
// connections are established before issuing calls.
func WithStreamDedup() ServerOption {
	return func(o *serverOptions) {
		o.outboundDialOpts = append(o.outboundDialOpts, conn.WithStreamDedup())
	}
}
