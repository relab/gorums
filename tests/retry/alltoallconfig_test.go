package retry

import (
	"flag"
	"fmt"
	"log"
	"net"
	sync "sync"
	"testing"
	"time"

	gorums "github.com/relab/gorums"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var replicaCount = flag.Int("replicaCount", 10, "number of replicas to create all-to-all communication")
var waitTime = flag.Int("waitTime", 1, "Seconds to wait before forming the configuration")
var wg sync.WaitGroup

func TestAlltoAllConfiguration(t *testing.T) {
	nodeMap := make(map[string]uint32)
	idMap := make(map[uint32]string)
	for i := 1; i <= *replicaCount; i++ {
		address := fmt.Sprintf("%s:%d", "127.0.0.1", 60000+i)
		nodeMap[address] = uint32(i)
		idMap[uint32(i)] = address
	}
	wg.Add(*replicaCount)
	for i := 1; i <= *replicaCount; i++ {
		go startServerAndCreateConfiguration(nodeMap, idMap[uint32(i)], t, *waitTime)
	}
	wg.Wait()
	log.Println("TestCompleted")
}

type qspec struct{}

func (q qspec) WriteQCQF(in *WriteRequest, replies map[uint32]*WriteResponse) (*WriteResponse, bool) {
	return &WriteResponse{New: true}, true
}

type replica struct{}

func (r replica) WriteQC(ctx gorums.ServerCtx, request *WriteRequest) (response *WriteResponse, err error) {
	return &WriteResponse{New: true}, nil
}

func startServerAndCreateConfiguration(nodeMap map[string]uint32,
	localAddress string, t *testing.T, waitSeconds int) {
	lis, err := net.Listen("tcp", localAddress)
	if err != nil {
		log.Fatalf("Failed to listen on '%s': %v\n", localAddress, err)
		t.Fail()
	}
	replica := replica{}
	srv := gorums.NewServer()
	RegisterSampleServer(srv, replica)
	go func() {
		err := srv.Serve(lis)
		if err != nil {
			log.Printf("Node %s failed to serve\n", localAddress)
			t.Fail()
		}
	}()
	time.Sleep(time.Duration(waitSeconds) * time.Second)
	mgr := NewManager(gorums.WithDialTimeout(1*time.Second),
		gorums.WithGrpcDialOptions(
			grpc.WithBlock(), // block until connections are made
			grpc.WithTransportCredentials(insecure.NewCredentials()), // disable TLS
		),
	)
	qspec := qspec{}
	_, err = mgr.NewConfiguration(qspec, gorums.WithNodeMap(nodeMap))
	if err != nil {
		log.Printf("unable to create the configuration Error: %v\n", err)
		t.Fail()
	}
	log.Printf("Configuration formed")
	wg.Done()
}
