package gorums

import (
	"log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/metadata"

	"github.com/relab/gorums/internal/stream"
)

// Option is a marker interface for options to NewConfig.
type Option interface {
	isOption()
}

// ManagerOption provides a way to set different options on a new Manager.
type ManagerOption func(*managerOptions)

func (ManagerOption) isOption() {}

type managerOptions struct {
	grpcDialOpts []grpc.DialOption
	logger       *log.Logger
	backoff      backoff.Config
	sendBuffer   uint
	metadata     metadata.MD
	perNodeMD    func(uint32) metadata.MD
	handler      stream.RequestHandler
	localNodeID  uint32 // if non-zero, skip setting handler on this node ID
}

func newManagerOptions() managerOptions {
	return managerOptions{
		backoff:    backoff.DefaultConfig,
		sendBuffer: 0,
	}
}

// WithDialOptions returns a ManagerOption which sets any gRPC dial options
// the Manager should use when initially connecting to each node in its pool.
func WithDialOptions(opts ...grpc.DialOption) ManagerOption {
	return func(o *managerOptions) {
		o.grpcDialOpts = append(o.grpcDialOpts, opts...)
	}
}

// WithLogger returns a ManagerOption which sets an optional error logger for
// the Manager.
func WithLogger(logger *log.Logger) ManagerOption {
	return func(o *managerOptions) {
		o.logger = logger
	}
}

// WithBackoff allows for changing the backoff delays used by Gorums.
func WithBackoff(backoff backoff.Config) ManagerOption {
	return func(o *managerOptions) {
		o.backoff = backoff
	}
}

// WithSendBufferSize allows for changing the size of the send buffer used by Gorums.
// A larger buffer might achieve higher throughput for asynchronous calltypes, but at
// the cost of latency.
func WithSendBufferSize(size uint) ManagerOption {
	return func(o *managerOptions) {
		o.sendBuffer = size
	}
}

// WithMetadata returns a ManagerOption that sets the metadata that is sent to each node
// when the connection is initially established. This metadata can be retrieved from the
// server-side method handlers.
func WithMetadata(md metadata.MD) ManagerOption {
	return func(o *managerOptions) {
		o.metadata = md
	}
}

// WithPerNodeMetadata returns a ManagerOption that allows you to set metadata for each
// node individually.
func WithPerNodeMetadata(f func(uint32) metadata.MD) ManagerOption {
	return func(o *managerOptions) {
		o.perNodeMD = f
	}
}

// WithNodeID returns a ManagerOption that automatically includes this client's
// node ID in the outgoing metadata. This is required for symmetric configurations
// because the server needs to identify the connecting client.
func WithNodeID(id uint32) ManagerOption {
	return func(o *managerOptions) {
		o.metadata = metadata.Join(o.metadata, metadataWithNodeID(id))
	}
}

// withRequestHandler returns a ManagerOption that sets the RequestHandler used to
// dispatch server-initiated requests arriving on the bidirectional back-channel,
// and records localID as this node's own NodeID. The localID is included in this
// node's outgoing metadata for each connection, enabling the server to identify
// this replica. The handler is not installed for the self-connection (if any) to
// avoid deadlocks in symmetric configurations.
func withRequestHandler(handler stream.RequestHandler, localID uint32) ManagerOption {
	return func(o *managerOptions) {
		o.handler = handler
		o.localNodeID = localID
		o.metadata = metadata.Join(o.metadata, metadataWithNodeID(localID))
	}
}

// requestHandlerFor returns the RequestHandler for the given node ID, or nil if no
// handler is configured or if id is this node's own local ID (self-connection).
func (o *managerOptions) requestHandlerFor(id uint32) stream.RequestHandler {
	if o.handler == nil || id == o.localNodeID {
		return nil
	}
	return o.handler
}
