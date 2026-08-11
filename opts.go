package gorums

import (
	"log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/metadata"

	"github.com/relab/gorums/internal/stream"
)

// DialOption provides a way to set different options on a new configuration.
type DialOption func(*dialOptions)

type dialOptions struct {
	grpcDialOpts []grpc.DialOption
	logger       *log.Logger
	backoff      backoff.Config
	sendBuffer   uint
	metadata     metadata.MD
	handler      stream.RequestHandler
	localNodeID  uint32          // if non-zero, skip setting handler on this node ID
	inboundMgr   *inboundManager // set by WithBackChannel; enables eager reconnect for symmetric nodes
}

// DefaultSendBufferSize is the per-node send queue capacity used when no
// explicit size is configured. It is both the backlog threshold at which a peer
// that stopped draining sends is treated as failed, since a full queue fails
// two-way requests fast with [ErrSendQueueFull], and the depth to which one-way
// calls dispatched asynchronously can pipeline. The queue is a buffered
// channel, so each node allocates the full capacity whether or not traffic
// flows.
const DefaultSendBufferSize = 4096

func newDialOptions() dialOptions {
	return dialOptions{
		backoff:    backoff.DefaultConfig,
		sendBuffer: DefaultSendBufferSize,
	}
}

// WithDialOptions returns a DialOption which sets any gRPC dial options
// the client should use when initially connecting to each node in its pool.
func WithDialOptions(opts ...grpc.DialOption) DialOption {
	return func(o *dialOptions) {
		o.grpcDialOpts = append(o.grpcDialOpts, opts...)
	}
}

// WithLogger returns a DialOption which sets an optional error logger for
// the configuration.
func WithLogger(logger *log.Logger) DialOption {
	return func(o *dialOptions) {
		o.logger = logger
	}
}

// WithBackoff allows for changing the backoff delays used by Gorums.
func WithBackoff(backoff backoff.Config) DialOption {
	return func(o *dialOptions) {
		o.backoff = backoff
	}
}

// WithSendBufferSize sets the per-node send queue capacity. A larger buffer
// may achieve higher throughput for asynchronous call types, at the cost of
// latency. Size 0 selects [DefaultSendBufferSize]: capacity 0 is not viable
// under the full-queue fail-fast semantics, since every two-way request
// enqueued while the sender is busy would fail.
func WithSendBufferSize(size uint) DialOption {
	return func(o *dialOptions) {
		if size == 0 {
			size = DefaultSendBufferSize
		}
		o.sendBuffer = size
	}
}

// WithMetadata returns a DialOption that merges md with any other metadata sent to
// each node during connection establishment.
// This metadata can be retrieved from the server-side method handlers.
func WithMetadata(md metadata.MD) DialOption {
	return func(o *dialOptions) {
		o.metadata = metadata.Join(o.metadata, md)
	}
}

// WithBackChannel returns a [DialOption] that installs srv as the back-channel
// request handler and includes srv.NodeID() in the outgoing metadata, allowing
// the remote endpoint to route server-initiated requests back over the
// bidirectional connection. Use it for a client that must accept calls from the
// servers it dials. It panics if srv is nil.
//
// A server that calls its own peers does not need this option: [WithPeers]
// installs the back channel on the peer [Config] it builds.
//
// NodeID semantics:
//   - If srv.NodeID() == 0, the remote treats this connection as an anonymous
//     client and tracks reverse-direction calls via [ServerCtx.ClientConfig].
//   - If srv.NodeID() > 0, the remote treats this connection as a known peer
//     and routes requests via [ServerCtx.Config].
func WithBackChannel(srv *Server) DialOption {
	if srv == nil {
		panic("gorums: WithBackChannel called with nil server")
	}
	return withServer(srv)
}

// withServer is WithBackChannel without the nil check, for the server's own
// peer configuration, where the server is known to be non-nil.
func withServer(srv *Server) DialOption {
	return func(o *dialOptions) {
		o.handler = srv
		o.localNodeID = srv.NodeID()
		o.inboundMgr = srv.inboundManager
		o.metadata = metadata.Join(o.metadata, metadataWithNodeID(srv.NodeID()))
	}
}
