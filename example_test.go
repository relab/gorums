package gorums_test

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/cmd/protoc-gen-gorums/dev"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// resultServer implements the QuorumCall method of a generated service by
// replying with a fixed result. The example calls no other method.
type resultServer struct {
	dev.ZorumsServiceServer
	result int64
}

func (s resultServer) QuorumCall(gorums.ServerContext, *dev.Request) (*dev.Response, error) {
	return dev.Response_builder{Result: s.result}.Build(), nil
}

// majorityResult is a custom quorum function. It returns the result that a
// majority of the nodes replied, or [gorums.ErrIncomplete] if no result can
// reach a majority. It stops reading responses as soon as one result has a
// majority.
func majorityResult(responses *gorums.Responses[*dev.Response]) (int64, error) {
	quorum := responses.Size()/2 + 1
	votes := make(map[int64]int)
	for resp := range responses.Results().IgnoreErrors() {
		result := resp.Value.GetResult()
		votes[result]++
		if votes[result] >= quorum {
			return result, nil
		}
	}
	return 0, gorums.ErrIncomplete
}

// This example passes the responses of a generated quorum call to a custom
// quorum function. Two of the three servers reply 7, so 7 has a majority.
func ExampleResponses_customAggregation() {
	var addrs []string
	var servers []*gorums.Server
	defer func() {
		for _, srv := range servers {
			srv.Stop()
		}
	}()
	for _, result := range []int64{7, 9, 7} {
		lis, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			log.Fatal(err)
		}
		srv := gorums.NewServer()
		dev.RegisterZorumsServiceServer(srv, resultServer{result: result})
		go func() { _ = srv.Serve(lis) }()
		servers = append(servers, srv)
		addrs = append(addrs, lis.Addr().String())
	}

	cfg, err := gorums.NewConfig(
		gorums.WithNodeList(addrs),
		gorums.WithGRPCDialOptions(grpc.WithTransportCredentials(insecure.NewCredentials())),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer cfg.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := majorityResult(dev.QuorumCall(cfg.Context(ctx), &dev.Request{}).Responses)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("majority result:", result)
	// Output: majority result: 7
}
