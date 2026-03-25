package gorums

import (
	"fmt"
	"log"
	"reflect"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/metadata"

	"github.com/relab/gorums/internal/stream"
)

// Option is a marker interface for options to NewConfig.
type Option interface {
	isOption()
}

// DialOption provides a way to set different options on a new configuration.
type DialOption func(*dialOptions)

func (DialOption) isOption() {}

// ManagerOption is a deprecated alias for [DialOption].
//
// Deprecated: Use [DialOption] instead.
type ManagerOption = DialOption

type dialOptions struct {
	grpcDialOpts []grpc.DialOption
	logger       *log.Logger
	backoff      backoff.Config
	sendBuffer   uint
	metadata     metadata.MD
	perNodeMD    func(uint32) metadata.MD
	handler      stream.RequestHandler
	localNodeID  uint32 // if non-zero, skip setting handler on this node ID
}

func newDialOptions() dialOptions {
	return dialOptions{
		backoff:    backoff.DefaultConfig,
		sendBuffer: 0,
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

// WithSendBufferSize allows for changing the size of the send buffer used by Gorums.
// A larger buffer might achieve higher throughput for asynchronous calltypes, but at
// the cost of latency.
func WithSendBufferSize(size uint) DialOption {
	return func(o *dialOptions) {
		o.sendBuffer = size
	}
}

// WithMetadata returns a DialOption that sets the metadata that is sent to each node
// when the connection is initially established. This metadata can be retrieved from the
// server-side method handlers.
func WithMetadata(md metadata.MD) DialOption {
	return func(o *dialOptions) {
		o.metadata = md
	}
}

// WithPerNodeMetadata returns a DialOption that allows you to set metadata for each
// node individually.
func WithPerNodeMetadata(f func(uint32) metadata.MD) DialOption {
	return func(o *dialOptions) {
		o.perNodeMD = f
	}
}

// withRequestHandler returns a DialOption that sets the RequestHandler used to
// dispatch server-initiated requests arriving on the bidirectional back-channel,
// and records localID as this node's own NodeID. The localID is included in this
// node's outgoing metadata for each connection, enabling the server to identify
// this replica. The handler is not installed for the self-connection (if any) to
// avoid deadlocks in symmetric configurations.
func withRequestHandler(handler stream.RequestHandler, localID uint32) DialOption {
	return func(o *dialOptions) {
		o.handler = handler
		o.localNodeID = localID
		o.metadata = metadata.Join(o.metadata, metadataWithNodeID(localID))
	}
}

// WithServer returns a [DialOption] that installs srv as the back-channel request
// handler and includes srv.NodeID() in the outgoing metadata, allowing the remote
// endpoint to route server-initiated requests back over the bidirectional connection.
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
	return withRequestHandler(srv, srv.NodeID())
}

// splitOptions separates a slice of [Option]s into [ServerOption]s, [DialOption]s,
// and a single [NodeListOption]. It returns an error if more than one [NodeListOption]
// is provided or if an unsupported option type is encountered.
func splitOptions(opts []Option) (srvOpts []ServerOption, dialOpts []DialOption, nodeListOpt NodeListOption, err error) {
	for _, opt := range opts {
		// A typed interface value (e.g. DialOption(nil)) is not equal to nil, so we need
		// to also check if the underlying value is actually nil to avoid panics.
		if opt == nil || reflect.ValueOf(opt).IsNil() {
			continue
		}
		switch o := opt.(type) {
		case ServerOption:
			srvOpts = append(srvOpts, o)
		case DialOption:
			dialOpts = append(dialOpts, o)
		case NodeListOption:
			if nodeListOpt != nil {
				return nil, nil, nil, fmt.Errorf("gorums: multiple NodeListOptions provided")
			}
			nodeListOpt = o
		default:
			return nil, nil, nil, fmt.Errorf("gorums: unsupported option type: %T", opt)
		}
	}
	return srvOpts, dialOpts, nodeListOpt, nil
}
