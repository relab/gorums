package gorums

import (
	"errors"
	"log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"

	"github.com/relab/gorums/internal/conn"
)

// DialOption provides a way to set different options on a new configuration.
type DialOption = conn.DialOption

// WithGRPCDialOptions returns a DialOption which sets any gRPC dial options
// the client should use when initially connecting to each node in its pool.
func WithGRPCDialOptions(opts ...grpc.DialOption) DialOption {
	return func(o *conn.DialOptions) {
		o.GRPCDialOptions = append(o.GRPCDialOptions, opts...)
	}
}

// WithTransportCredentials returns a DialOption that sets the transport
// credentials for the connection to each node, such as TLS credentials from
// [credentials.NewTLS]. They replace any credentials set with
// [WithGRPCDialOptions].
//
// Nodes that declare a [TLSIdentity] require this option. After each TLS
// handshake, and before the connection carries any stream, Gorums checks that
// the server's verified certificate names the dialed node. If the check
// fails, Gorums closes the connection, and calls to the node fail with
// codes.Unauthenticated.
func WithTransportCredentials(creds credentials.TransportCredentials) DialOption {
	return func(o *conn.DialOptions) {
		o.TransportCredentials = creds
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
// [WithDialBufferSizes] or [WithBufferSizes] is applied with a send size of 0.
const DefaultSendBufferSize = conn.DefaultSendBufferSize

// DefaultRecvBufferSize is the number of received requests per channel that
// can wait for the running handler when [WithDialBufferSizes] or
// [WithBufferSizes] is applied with a receive size of 0.
const DefaultRecvBufferSize = conn.DefaultRecvBufferSize

// WithDialBufferSizes sets the receive and send buffer sizes of each outbound
// channel, as [WithBufferSizes] does for a server.
//
// The recvSize is the number of requests received on the stream that can wait
// for the running handler. An outbound channel runs handlers for requests that
// the peer sends back on the stream, such as back-channel calls. Receiving from
// the stream waits while that many are queued. A recvSize of 0 selects
// [DefaultRecvBufferSize].
//
// The sendSize is the capacity of the per-node send queue. When the queue is
// full, two-way calls fail with [ErrSendQueueFull] and one-way calls wait for
// space. A sendSize of 0 selects [DefaultSendBufferSize].
func WithDialBufferSizes(recvSize, sendSize uint) DialOption {
	return func(o *conn.DialOptions) {
		if sendSize == 0 {
			sendSize = conn.DefaultSendBufferSize
		}
		if recvSize == 0 {
			recvSize = conn.DefaultRecvBufferSize
		}
		o.RecvBufferSize = recvSize
		o.SendBufferSize = sendSize
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
		o.InboundManager = srv.im
		o.Metadata = metadata.Join(o.Metadata, conn.MetadataWithNodeID(srv.NodeID()))
	}
}
