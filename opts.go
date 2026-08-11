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
	grpcDialOpts  []grpc.DialOption
	logger        *log.Logger
	backoff       backoff.Config
	sendBuffer    uint
	metadata      metadata.MD
	handler       stream.RequestHandler
	localNodeID   uint32          // if non-zero, skip setting handler on this node ID
	inboundMgr    *inboundManager // set by WithServer; enables eager reconnect for symmetric nodes
	srvOpts       []ServerOption  // applied only by NewSystem / NewLocalSystems
	outboundNodes NodeListOption  // applied only by NewSystem
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

// WithServer returns a [DialOption] that installs srv as the back-channel request
// handler and includes srv.NodeID() in the outgoing metadata, allowing the remote
// endpoint to route server-initiated requests back over the bidirectional connection.
// This option is intended for use in symmetric peer configurations, where each node
// is both a client and a server. It will panic if srv is nil.
//
// NodeID semantics:
//   - If srv.NodeID() == 0, the remote will typically treat this connection as an
//     anonymous client and track reverse-direction calls via [ServerCtx.ClientConfig].
//   - If srv.NodeID() > 0, the remote will treat this connection as a known peer
//     and route requests via [ServerCtx.Config], as in symmetric peer configurations
//     (e.g., outbound configs between replicas).
func WithServer(srv *Server) DialOption {
	if srv == nil {
		panic("gorums: WithServer called with nil server")
	}
	return func(o *dialOptions) {
		o.handler = srv
		o.localNodeID = srv.NodeID()
		o.inboundMgr = srv.inboundManager
		o.metadata = metadata.Join(o.metadata, metadataWithNodeID(srv.NodeID()))
	}
}

// WithServerOptions bundles [ServerOption]s into a [DialOption] for use with
// [NewSystem] and [NewLocalSystems]. It has no effect when passed to [NewConfig].
// Nil options are silently ignored.
func WithServerOptions(opts ...ServerOption) DialOption {
	return func(o *dialOptions) {
		for _, opt := range opts {
			if opt != nil {
				o.srvOpts = append(o.srvOpts, opt)
			}
		}
	}
}

// WithOutboundNodes wraps a [NodeListOption] as a [DialOption] for use with
// [NewSystem], instructing it to create an outbound [Configuration] for the given
// peers. It has no effect when passed to [NewConfig] or [NewLocalSystems].
// A nil opt is a no-op.
func WithOutboundNodes(opt NodeListOption) DialOption {
	return func(o *dialOptions) {
		if opt != nil {
			o.outboundNodes = opt
		}
	}
}
