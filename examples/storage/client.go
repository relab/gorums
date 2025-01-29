package main

import (
	"log"
	"time"

	"github.com/relab/gorums"
	pb "github.com/relab/gorums/examples/storage/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func runClient(addresses []string) {
	if len(addresses) < 1 {
		log.Fatalln("No addresses provided!")
	}

	// init gorums manager
	mgr := pb.NewManager(
		gorums.WithDialTimeout(1*time.Second),
		gorums.WithGrpcDialOptions(
			grpc.WithBlock(), // block until connections are made
			grpc.WithTransportCredentials(insecure.NewCredentials()), // disable TLS
		),
	)
	// create configuration containing all nodes
	cfg, err := mgr.NewConfiguration(&qspec{cfgSize: len(addresses)}, gorums.WithNodeList(addresses))
	if err != nil {
		log.Fatal(err)
	}

	Repl(mgr, cfg)
}

type qspec struct {
	cfgSize int
}

// ReadQCQF is the quorum function for the ReadQC
// ordered quorum call method. The in parameter is the request object
// supplied to the ReadQC method at call time, and may or may not
// be used by the quorum function. If the in parameter is not needed
// you should implement your quorum function with '_ *ReadRequest'.
func (q qspec) ReadQCQF(_ *pb.ReadRequest, replies map[uint32]*pb.ReadResponse) (*pb.ReadResponse, bool) {
	// wait until at least half of the replicas have responded
	if len(replies) <= q.cfgSize/2 {
		return nil, false
	}
	// return the value with the most recent timestamp
	return newestValue(replies), true
}

// WriteQCQF is the quorum function for the WriteQC
// ordered quorum call method. The in parameter is the request object
// supplied to the WriteQC method at call time, and may or may not
// be used by the quorum function. If the in parameter is not needed
// you should implement your quorum function with '_ *WriteRequest'.
func (q qspec) WriteQCQF(in *pb.WriteRequest, replies map[uint32]*pb.WriteResponse) (*pb.WriteResponse, bool) {
	// wait until at least half of the replicas have responded and have updated their value
	if numUpdated(replies) > q.cfgSize/2 {
		return pb.WriteResponse_builder{New: true}.Build(), true
	}
	// if all replicas have responded, there must have been another write before ours
	// that had a newer timestamp
	if len(replies) == q.cfgSize {
		return pb.WriteResponse_builder{New: false}.Build(), true
	}
	return nil, false
}

// newestValue returns the reply that had the most recent timestamp
func newestValue(values map[uint32]*pb.ReadResponse) *pb.ReadResponse {
	if len(values) < 1 {
		return nil
	}
	var newest *pb.ReadResponse
	for _, v := range values {
		if v.GetTime().AsTime().After(newest.GetTime().AsTime()) {
			newest = v
		}
	}
	return newest
}

// numUpdated returns the number of replicas that updated their value
func numUpdated(replies map[uint32]*pb.WriteResponse) int {
	count := 0
	for _, r := range replies {
		if r.GetNew() {
			count++
		}
	}
	return count
}
