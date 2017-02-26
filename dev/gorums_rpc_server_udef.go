package dev

import (
	"context"
	"sync"
)

// GorumsRPCBench represents a single State register used for benchmarking.
type GorumsRPCBench struct {
	sync.RWMutex
	state Reply
}

// NewGorumsRPCBench returns a new register benchmark server.
func NewGorumsRPCBench() *GorumsRPCBench {
	return &GorumsRPCBench{}
}

// ReadCorrectable yadayada
func (r *GorumsRPCBench) ReadCorrectable(ctx context.Context, rq *ReadReq) (*Reply, error) {
	r.RLock()
	defer r.RUnlock()
	return &Reply{Value: r.state.Value, Timestamp: r.state.Timestamp}, nil
}
