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
	peerVerifier     PeerVerifier // Checks peer identities on peer streams; set by WithPeerVerifier.
	hasPeerVerifier  bool         // Records that WithPeerVerifier was applied, so a nil verifier is reported.
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
// selects the default of 4096.
//
// The sendSize controls the capacity of the server's per-node send queue for
// outgoing peer messages, with the same full-queue semantics as
// [WithSendBufferSize]. Two-way requests fail fast when the queue is full.
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

// WithPeerVerifier makes the server check, with verify, the identity of the
// peers configured by [WithPeers] on every stream between them, in both
// directions:
//   - An inbound stream that claims a peer's node ID is checked against that
//     ID. A stream that fails the check, or that claims a node ID outside the
//     peer configuration, ends with codes.Unauthenticated before any connect
//     callback or handler runs. Streams that claim no peer node ID are not
//     checked.
//   - A stream that the server opens to a peer is checked against the dialed
//     node's ID before any request is sent on it. If the check fails, the
//     stream is closed, the waiting call fails with codes.Unauthenticated,
//     and the server tries again later, as for a failed connection.
//
// When the server refuses a stream that a peer opened, the refusal's message
// includes the text of verify's error, so the peer learns why. When a stream
// that the server opened fails the check, the text is in the error of the
// waiting call. The server runs verify once per stream, outside its internal
// locks. With WithPeerVerifier, [ServerContext.SenderID] is authenticated for
// every configured peer. [NewServer] panics if verify is nil or if the server
// has no [WithPeers]. Use [TLSPeerVerifier] to bind node IDs to mTLS
// certificates.
func WithPeerVerifier(verify PeerVerifier) ServerOption {
	return func(o *serverOptions) {
		o.peerVerifier = verify
		o.hasPeerVerifier = true
	}
}
