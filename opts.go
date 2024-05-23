package gorums

import (
	"log"
	"time"

	"github.com/relab/gorums/broadcast"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/metadata"
)

type managerOptions struct {
	grpcDialOpts    []grpc.DialOption
	nodeDialTimeout time.Duration
	logger          *log.Logger
	noConnect       bool
	backoff         backoff.Config
	sendBuffer      uint
	metadata        metadata.MD
	perNodeMD       func(uint32) metadata.MD
	publicKey       string
	machineID       uint64
}

func newManagerOptions() managerOptions {
	return managerOptions{
		backoff:         backoff.DefaultConfig,
		sendBuffer:      50,
		nodeDialTimeout: 50 * time.Millisecond,
		// Provide an illegal machineID to avoid unintentional collisions.
		// 0 is a valid MachineID and should not be used as default.
		machineID: uint64(broadcast.MaxMachineID) + 1,
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

// WithLogger returns a ManagerOption which sets an optional error logger for
// the Manager.
func WithLogger(logger *log.Logger) ManagerOption {
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

func WithPublicKey(publicKey string) ManagerOption {
	return func(o *managerOptions) {
		o.publicKey = publicKey
	}
}

func WithMachineID(id uint64) ManagerOption {
	return func(o *managerOptions) {
		o.machineID = id
	}
}
