package gorums

import (
	"errors"
	"log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/metadata"

	"github.com/relab/gorums/internal/conn"
)

// DialOption provides a way to set different options on a new configuration.
type DialOption = conn.DialOption

// WithGRPCDialOptions returns a DialOption which sets any gRPC dial options
// the client should use when initially connecting to each node in its pool.
func WithGRPCDialOptions(opts ...grpc.DialOption) DialOption {
	return func(o *conn.DialOptions) {
		o.GRPCDialOpts = append(o.GRPCDialOpts, opts...)
	}
}

// WithLogger returns a DialOption which sets an optional error logger for
// the configuration.
func WithLogger(logger *log.Logger) DialOption {
	return func(o *conn.DialOptions) {
		o.Logger = logger
	}
}

// WithBackoff allows for changing the backoff delays used by Gorums.
func WithBackoff(backoff backoff.Config) DialOption {
	return func(o *conn.DialOptions) {
		o.Backoff = backoff
	}
}

// DefaultSendBufferSize is the per-node send queue capacity Gorums uses when
// [WithSendBufferSize] or [WithBufferSizes] is applied with size 0.
const DefaultSendBufferSize = conn.DefaultSendBufferSize

// WithSendBufferSize sets the capacity of the per-node send queue used by Gorums.
// When the queue toward a peer is full, two-way requests (RPC, quorum calls)
// fail fast with an Unavailable error (send queue full) so that quorum logic
// can count the peer as failed, while one-way requests (Unicast, Multicast)
// block until there is space, pacing the producer. The capacity is thus the
// backlog threshold at which a peer that stopped draining sends is treated as
// failed. Size 0 selects [DefaultSendBufferSize]; a larger value tolerates
// longer peer hiccups at the cost of memory and queueing latency.
func WithSendBufferSize(size uint) DialOption {
	return func(o *conn.DialOptions) {
		if size == 0 {
			size = conn.DefaultSendBufferSize
		}
		o.SendBuffer = size
	}
}

// WithMetadata returns a DialOption that merges md with any other metadata sent to
// each node during connection establishment.
// This metadata can be retrieved from the server-side method handlers.
func WithMetadata(md metadata.MD) DialOption {
	return func(o *conn.DialOptions) {
		o.Metadata = metadata.Join(o.Metadata, md)
	}
}

// WithBackChannel returns a [DialOption] that installs srv as the handler for
// calls the dialed servers send back over the configuration's connections.
// Each dialed server tracks the caller as an anonymous client and can reach
// its registered handlers through [ServerContext.ConnectedClients]. srv needs no
// listener; its handlers are served entirely over the connections the
// configuration dials.
//
// srv must not be configured with [WithPeers]; [NewConfig] returns an error
// otherwise. A server in a peer group already answers its peers' calls over
// the connections established by [WithPeers].
func WithBackChannel(srv *Server) DialOption {
	return func(o *conn.DialOptions) {
		if srv == nil {
			o.Err = errors.Join(o.Err, errors.New("gorums: WithBackChannel requires a non-nil server"))
			return
		}
		if srv.NodeID() != 0 {
			o.Err = errors.Join(o.Err, errors.New("gorums: WithBackChannel server must not be configured with WithPeers"))
			return
		}
		withServer(srv)(o)
	}
}

// withServer returns a [DialOption] that installs srv as the back-channel request
// handler and includes srv.NodeID() in the outgoing metadata, allowing the remote
// endpoint to route server-initiated requests back over the bidirectional connection.
// It is used by [Server.newPeerConfig] to build the outbound configuration for
// peers that call one another ([WithPeers]), and by [WithBackChannel] for clients.
//
// NodeID semantics:
//   - If srv.NodeID() == 0, the remote treats this connection as an anonymous
//     client and can perform back-channel calls via [ServerContext.ConnectedClients].
//   - If srv.NodeID() > 0, the remote treats this connection as a known peer
//     and tracks it in [Server.ConnectedPeers], as when servers call one
//     another.
func withServer(srv *Server) DialOption {
	return func(o *conn.DialOptions) {
		o.Handler = srv
		o.LocalNodeID = srv.NodeID()
		o.InboundMgr = srv.im
		o.Metadata = metadata.Join(o.Metadata, conn.MetadataWithNodeID(srv.NodeID()))
	}
}
