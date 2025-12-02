package gorums

import (
	"log"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/metadata"
)

type managerOptions struct {
	grpcDialOpts []grpc.DialOption
	logger       *log.Logger
	noConnect    bool
	backoff      backoff.Config
	sendBuffer   uint
	metadata     metadata.MD
	perNodeMD    func(uint32) metadata.MD
	preConnect   func(stopServers func()) // test-only: called before connecting to nodes
}

func newManagerOptions() managerOptions {
	return managerOptions{
		backoff:    backoff.DefaultConfig,
		sendBuffer: 0,
	}
}

// ManagerOption provides a way to set different options on a new Manager.
type ManagerOption func(*managerOptions)

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

// -------------------------------------------------------------------------
// Testing-only options
// -------------------------------------------------------------------------

// WithNoConnect returns a ManagerOption which instructs the Manager not to
// connect to any of its nodes. Mainly used for testing purposes.
func WithNoConnect() ManagerOption {
	return func(o *managerOptions) {
		o.noConnect = true
	}
}

// WithTeardown returns a ManagerOption that registers a function to be called
// after servers are started but before nodes attempt to connect. The function
// receives a stopServers callback that can be used to stop the test servers.
//
// This is useful for testing error handling when servers are unavailable:
//
//	node := gorums.SetupNode(t, nil, gorums.WithTeardown(t, func(stopServers func()) {
//		stopServers()
//		time.Sleep(300 * time.Millisecond) // wait for server to fully stop
//	}))
//
// This option is intended for testing purposes only.
func WithTeardown(_ testing.TB, fn func(stopServers func())) ManagerOption {
	return func(o *managerOptions) {
		o.preConnect = fn
	}
}
