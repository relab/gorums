package dev

import "github.com/relab/gorums"

func NewServer(opts ...gorums.ServerOption) *gorums.Server {
	return gorums.NewServer(orderingMethods, opts...)
}
