package dev

import (
	"log"

	"google.golang.org/grpc"
)

type managerOptions struct {
	// Used by every generated implementation
	grpcDialOpts []grpc.DialOption
	logger       *log.Logger
	noConnect    bool
	selfAddr     string
	selfID       uint32

	// Generated for each specific implementation
	readqf  ReadQuorumFn
	writeqf WriteQuorumFn
}

// WithReadQuorumFunc returns a ManagerOption that sets a cumstom
// ReadQuorumFunc.
func WithReadQuorumFunc(f ReadQuorumFn) ManagerOption {
	return func(o *managerOptions) {
		o.readqf = f
	}
}

// WithWriteQuorumFunc returns a ManagerOption that sets a cumstom
// WirteQuorumFunc.
func WithWriteQuorumFunc(f WriteQuorumFn) ManagerOption {
	return func(o *managerOptions) {
		o.writeqf = f
	}
}
