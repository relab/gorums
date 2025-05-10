package main

import (
	"errors"
	"log"

	"github.com/relab/gorums"
	"github.com/relab/gorums/examples/storage/proto"
	pb "github.com/relab/gorums/examples/storage/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func runClient(addresses []string) error {
	if len(addresses) < 1 {
		log.Fatalln("No addresses provided!")
	}

	// init gorums manager
	mgr := proto.NewManager(
		gorums.WithGrpcDialOptions(
			grpc.WithTransportCredentials(insecure.NewCredentials()), // disable TLS
		),
	)
	// create configuration containing all nodes
	cfg, err := mgr.NewConfiguration(gorums.WithNodeList(addresses))
	if err != nil {
		log.Fatal(err)
	}

	return Repl(mgr, cfg)
}

// newestValue returns the reply that had the most recent timestamp
func newestValueOfTwo(v1, v2 *proto.ReadResponse) *proto.ReadResponse {
	if v1 == nil {
		return v2
	} else if v2 == nil {
		return v1
	}
	if v1.GetTime().AsTime().After(v2.GetTime().AsTime()) {
		return v1
	} else {
		return v2
	}
}

// numUpdated returns the number of replicas that updated their value
func isUpdated(r *proto.WriteResponse) bool {
	return r.GetNew()
}

func writeQF(replies gorums.Responses[*pb.WriteResponse], cfgSize int) (*pb.WriteResponse, error) {
	replyCount := int(0)
	updated := int(0)
	for response := range replies.IgnoreErrors() {
		replyCount++
		if isUpdated(response.Msg) {
			updated++
		}
		if updated > cfgSize/2 {
			return proto.WriteResponse_builder{New: true}.Build(), nil
		}
		// if all replicas have responded, there must have been another write before ours
		// that had a newer timestamp
		if replyCount == cfgSize {
			return proto.WriteResponse_builder{New: false}.Build(), nil
		}
	}
	return nil, errors.New("storage.writeqc: incomplete response")
}

func readQF(replies gorums.Responses[*pb.ReadResponse], quorum int) (*pb.ReadResponse, error) {
	var newest *pb.ReadResponse
	replyCount := int(0)
	for reply := range replies.IgnoreErrors() {
		newest = newestValueOfTwo(newest, reply.Msg)
		replyCount++
		if replyCount <= quorum {
			continue
		}
		return newest, nil
	}
	return nil, errors.New("storage.readqc: quorum not found")
}
