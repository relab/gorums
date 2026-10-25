package main

import (
	"fmt"

	"github.com/relab/gorums"
	"github.com/relab/gorums/examples/storage/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func runClient(addresses []string) error {
	if len(addresses) < 1 {
		return fmt.Errorf("no server addresses provided")
	}
	cfg, closeFn, err := gorums.NewConfig(
		gorums.WithNodeList(addresses),
		gorums.WithGRPCDialOptions(grpc.WithTransportCredentials(insecure.NewCredentials())),
	)
	if err != nil {
		return err
	}
	defer closeFn()
	return runRepl(cfg)
}

// newestValue returns the ReadQC reply with the most recent timestamp. Use it
// with [gorums.Responses.Aggregate].
func newestValue(seq gorums.ResponseSeq[*proto.ReadResponse]) (*proto.ReadResponse, error) {
	var newest *proto.ReadResponse
	for resp := range seq {
		if resp.Err != nil {
			continue
		}
		if newest == nil || resp.Value.GetTime().AsTime().After(newest.GetTime().AsTime()) {
			newest = resp.Value
		}
	}
	if newest == nil {
		return nil, gorums.ErrIncomplete
	}
	return newest, nil
}

// numUpdated returns a function for [gorums.Responses.Aggregate] that
// reports, for a WriteQC call to size nodes, whether a majority of the nodes
// updated their value.
func numUpdated(size int) func(gorums.ResponseSeq[*proto.WriteResponse]) (*proto.WriteResponse, error) {
	return func(seq gorums.ResponseSeq[*proto.WriteResponse]) (*proto.WriteResponse, error) {
		var count int
		for resp := range seq.IgnoreErrors() {
			if resp.Value.GetNew() {
				count++
			}
		}
		// We need a majority of nodes to have updated the value
		return proto.WriteResponse_builder{New: count > size/2}.Build(), nil
	}
}
