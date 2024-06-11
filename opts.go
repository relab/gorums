package gorums

import (
	"log/slog"
	"time"

	"github.com/relab/gorums/authentication"
	"github.com/relab/gorums/broadcast"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/metadata"
)

type managerOptions struct {
	grpcDialOpts    []grpc.DialOption
	nodeDialTimeout time.Duration
	logger          *slog.Logger
	noConnect       bool
	backoff         backoff.Config
	sendBuffer      uint
	metadata        metadata.MD
	perNodeMD       func(uint32) metadata.MD
	machineID       uint64                        // used for generating SnowflakeIDs
	maxSendRetries  int                           // number of times we try to resend a failed msg
	maxConnRetries  int                           // number of times we try to reconnect (in the background) to a node
	auth            *authentication.EllipticCurve // used when authenticating msgs
}

func newManagerOptions() managerOptions {
	return managerOptions{
		backoff:         backoff.DefaultConfig,
		sendBuffer:      100,
		nodeDialTimeout: 50 * time.Millisecond,
		// Provide an illegal machineID to avoid unintentional collisions.
		// 0 is a valid MachineID and should not be used as default.
		machineID:      uint64(broadcast.MaxMachineID) + 1,
		maxSendRetries: 0,
		maxConnRetries: -1, // no limit
	}
}

// ManagerOption provides a way to set different options on a new Manager.
type ManagerOption func(*managerOptions)

// WithDialTimeout returns a ManagerOption which is used to set the dial
// context timeout to be used when initially connecting to each node in its pool.
func WithDialTimeout(timeout time.Duration) ManagerOption {
	return func(o *managerOptions) {
		o.nodeDialTimeout = timeout
	}
}

// WithGrpcDialOptions returns a ManagerOption which sets any gRPC dial options
// the Manager should use when initially connecting to each node in its pool.
func WithGrpcDialOptions(opts ...grpc.DialOption) ManagerOption {
	return func(o *managerOptions) {
		o.grpcDialOpts = append(o.grpcDialOpts, opts...)
	}
}

// WithLogger returns a ManagerOption which sets an optional structured logger for
// the Manager. This will log events regarding creation of nodes and transmission
// of messages.
func WithLogger(logger *slog.Logger) ManagerOption {
	return func(o *managerOptions) {
		o.logger = logger
	}
}

// WithNoConnect returns a ManagerOption which instructs the Manager not to
// connect to any of its nodes. Mainly used for testing purposes.
func WithNoConnect() ManagerOption {
	return func(o *managerOptions) {
		o.noConnect = true
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

// WithAuthentication returns a ManagerOptions that enables digital signatures for msgs.
func WithAuthentication(auth *authentication.EllipticCurve) ManagerOption {
	return func(o *managerOptions) {
		o.auth = auth
	}
}

// WithMachineID returns a ManagerOption that allows you to set a unique ID for the client.
// This ID will be embedded in broadcast request sent from the client, making the requests
// trackable by the whole cluster. A random ID will be generated if not set. This can cause
// collisions if there are many clients. MinID = 0 and MaxID = 4095.
func WithMachineID(id uint64) ManagerOption {
	return func(o *managerOptions) {
		o.machineID = id
	}
}

// WithSendRetries returns a ManagerOption that allows you to specify how many times the node
// will try to send a message. The message will be dropped if it fails to send the message
// more than the specified number of times.
func WithSendRetries(maxRetries int) ManagerOption {
	return func(o *managerOptions) {
		o.maxSendRetries = maxRetries
	}
}

// WithConnRetries returns a ManagerOption that allows you to specify how many times the node
// will try to reconnect to a node. Default: no limit but it will follow a backoff strategy.
func WithConnRetries(maxRetries int) ManagerOption {
	return func(o *managerOptions) {
		o.maxConnRetries = maxRetries
	}
}
