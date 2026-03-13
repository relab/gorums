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

// splitOptions separates a slice of [Option]s into [ServerOption]s, [ManagerOption]s,
// and a single [NodeListOption]. It returns an error if more than one [NodeListOption]
// is provided or if an unsupported option type is encountered.
func splitOptions(opts []Option) (srvOpts []ServerOption, mgrOpts []ManagerOption, nodeListOpt NodeListOption, err error) {
	for _, opt := range opts {
		// A typed interface value (e.g. ManagerOption(nil)) is not equal to nil, so we need
		// to also check if the underlying value is actually nil to avoid panics.
		if opt == nil || reflect.ValueOf(opt).IsNil() {
			continue
		}
		switch o := opt.(type) {
		case ServerOption:
			srvOpts = append(srvOpts, o)
		case ManagerOption:
			mgrOpts = append(mgrOpts, o)
		case NodeListOption:
			if nodeListOpt != nil {
				return nil, nil, nil, fmt.Errorf("gorums: multiple NodeListOptions provided")
			}
			nodeListOpt = o
		default:
			return nil, nil, nil, fmt.Errorf("gorums: unsupported option type: %T", opt)
		}
	}
	return srvOpts, mgrOpts, nodeListOpt, nil
}
