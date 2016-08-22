package dev

import (
	"log"

	"google.golang.org/grpc"
)

type managerOptions struct {
	grpcDialOpts []grpc.DialOption
	logger       *log.Logger
	noConnect    bool
	selfAddr     string
	selfID       uint32
	certPaths    map[string]string
}

// ManagerOption provides a way to set different options on a new Manager.
type ManagerOption func(*managerOptions)

// WithGrpcDialOptions returns a ManagerOption which sets any gRPC dial options
// the Manager should use when initially connecting to each node in its
// pool.
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

// WithSelfAddr returns a ManagerOption which instructs the Manager not to connect
// to the node with network address addr. The address must be present in the
// list of node addresses provided to the Manager.
func WithSelfAddr(addr string) ManagerOption {
	return func(o *managerOptions) {
		o.selfAddr = addr
	}
}

// WithSelfID returns a ManagerOption which instructs the Manager not to
// connect to the node with the given id. The node must be present in the list
// of node addresses provided to the Manager.
func WithSelfID(id uint32) ManagerOption {
	return func(o *managerOptions) {
		o.selfID = id
	}
}

func WithCredentials(certPaths map[string]string) ManagerOption {
	return func(o *managerOptions) {
		o.certPaths = certPaths
	}
}
