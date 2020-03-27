package dev

import (
	"log"
	"time"

	"google.golang.org/grpc"
	backoff "google.golang.org/grpc/backoff"
)

type managerOptions struct {
	grpcDialOpts    []grpc.DialOption
	nodeDialTimeout time.Duration
	logger          *log.Logger
	noConnect       bool
	trace           bool
	backoff         backoff.Config
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
		o.grpcDialOpts = opts
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

// WithTracing controls whether to trace qourum calls for this Manager instance
// using the golang.org/x/net/trace package. Tracing is currently only supported
// for regular quorum calls.
func WithTracing() ManagerOption {
	return func(o *managerOptions) {
		o.trace = true
	}
}

// WithBackoff allows for changing the backoff delays used by Gorums.
func WithBackoff(backoff backoff.Config) ManagerOption {
	return func(o *managerOptions) {
		o.backoff = backoff
	}
}
