package gorums

import (
	"cmp"
	"log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/metadata"
)

type managerOptions[idType cmp.Ordered] struct {
	grpcDialOpts []grpc.DialOption
	logger       *log.Logger
	noConnect    bool
	backoff      backoff.Config
	sendBuffer   uint
	metadata     metadata.MD
	perNodeMD    func(idType) metadata.MD
}

func newManagerOptions[idType cmp.Ordered]() managerOptions[idType] {
	return managerOptions[idType]{
		backoff:    backoff.DefaultConfig,
		sendBuffer: 0,
	}
}

// ManagerOption provides a way to set different options on a new Manager.
type ManagerOption[idType cmp.Ordered] func(*managerOptions[idType])

// WithGrpcDialOptions returns a ManagerOption which sets any gRPC dial options
// the Manager should use when initially connecting to each node in its pool.
func WithGrpcDialOptions[idType cmp.Ordered](opts ...grpc.DialOption) ManagerOption[idType] {
	return func(o *managerOptions[idType]) {
		o.grpcDialOpts = append(o.grpcDialOpts, opts...)
	}
}

// WithLogger returns a ManagerOption which sets an optional error logger for
// the Manager.
func WithLogger[idType cmp.Ordered](logger *log.Logger) ManagerOption[idType] {
	return func(o *managerOptions[idType]) {
		o.logger = logger
	}
}

// WithNoConnect returns a ManagerOption which instructs the Manager not to
// connect to any of its nodes. Mainly used for testing purposes.
func WithNoConnect[idType cmp.Ordered]() ManagerOption[idType] {
	return func(o *managerOptions[idType]) {
		o.noConnect = true
	}
}

// WithBackoff allows for changing the backoff delays used by Gorums.
func WithBackoff[idType cmp.Ordered](backoff backoff.Config) ManagerOption[idType] {
	return func(o *managerOptions[idType]) {
		o.backoff = backoff
	}
}

// WithSendBufferSize allows for changing the size of the send buffer used by Gorums.
// A larger buffer might achieve higher throughput for asynchronous calltypes, but at
// the cost of latency.
func WithSendBufferSize[idType cmp.Ordered](size uint) ManagerOption[idType] {
	return func(o *managerOptions[idType]) {
		o.sendBuffer = size
	}
}

// WithMetadata returns a ManagerOption that sets the metadata that is sent to each node
// when the connection is initially established. This metadata can be retrieved from the
// server-side method handlers.
func WithMetadata[idType cmp.Ordered](md metadata.MD) ManagerOption[idType] {
	return func(o *managerOptions[idType]) {
		o.metadata = md
	}
}

// WithPerNodeMetadata returns a ManagerOption that allows you to set metadata for each
// node individually.
func WithPerNodeMetadata[idType cmp.Ordered](f func(idType) metadata.MD) ManagerOption[idType] {
	return func(o *managerOptions[idType]) {
		o.perNodeMD = f
	}
}
