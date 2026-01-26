package main

import (
	"log"

	"github.com/relab/gorums"
	"github.com/relab/gorums/examples/storage/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func runClient(addresses []string) error {
	if len(addresses) < 1 {
		log.Fatalln("No addresses provided!")
	}

	// init gorums manager
	mgr := proto.NewManager(
		gorums.WithDialOptions(
			grpc.WithTransportCredentials(insecure.NewCredentials()), // disable TLS
		),
	)
	// create configuration containing all nodes
	cfg, err := proto.NewConfiguration(mgr, gorums.WithNodeList(addresses))
	if err != nil {
		log.Fatal(err)
	}

	return Repl(mgr, cfg)
}

// newestValue processes responses from a ReadQC call and returns the reply
// with the most recent timestamp.
func newestValue(responses *gorums.Responses[proto.NodeID, *proto.ReadResponse]) (*proto.ReadResponse, error) {
	var newest *proto.ReadResponse
	for resp := range responses.Seq() {
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

// numUpdated processes responses from a WriteQC call and returns true if
// a majority of nodes updated their value.
func numUpdated(responses *gorums.Responses[proto.NodeID, *proto.WriteResponse]) (*proto.WriteResponse, error) {
	var count int
	size := responses.Size()
	for resp := range responses.Seq() {
		if resp.Err != nil {
			continue
		}
		if resp.Value.GetNew() {
			count++
		}
	}
	// We need a majority of nodes to have updated the value
	if count > size/2 {
		return proto.WriteResponse_builder{New: true}.Build(), nil
	}
	return proto.WriteResponse_builder{New: false}.Build(), nil
}
