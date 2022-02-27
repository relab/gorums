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

var replicaCount = flag.Int("replicaCount", 80, "number of replicas to create all-to-all communication")
var waitTime = flag.Int("waitTime", 2, "Seconds to wait before forming the configuration")
var wg sync.WaitGroup

func TestAlltoAllConfigurationStyle1(t *testing.T) {
	nodeMap := make(map[string]uint32)
	replicaList := make([]*replica, 0)
	for i := 1; i <= *replicaCount; i++ {
		address := fmt.Sprintf("%s:%d", "127.0.0.1", 50000+i)
		nodeMap[address] = uint32(i)
		replica := replica{
			address: address,
			id:      uint32(i),
		}
		replicaList = append(replicaList, &replica)
	}
	wg.Add(*replicaCount)
	for _, replica := range replicaList {
		go replica.startServerAndCreateConfig(nodeMap, t)
	}
	wg.Wait()
	log.Println("TestAlltoAllConfigurationStyle1 test completed")
}

func TestAlltoAllConfigurationStyle2(t *testing.T) {
	replicas := createReplicas()
	nodeMap := make(map[string]uint32)

	for _, replica := range replicas {
		nodeMap[replica.address] = replica.id
	}
	wg.Add(*replicaCount)
	for _, replica := range replicas {
		go replica.createConfiguration(nodeMap, t)
	}
	wg.Wait()
	log.Println("TestAlltoAllConfigurationStyle2 test completed")

}

func createReplicas() []*replica {
	replicas := make([]*replica, 0)
	for i := 1; i <= *replicaCount; i++ {
		lis, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil
		}
		replica := replica{
			address: lis.Addr().String(),
			id:      uint32(i),
			lis:     lis,
		}
		replicas = append(replicas, &replica)
	}
	return replicas
}

type qspec struct{}

func (q qspec) WriteQCQF(in *WriteRequest, replies map[uint32]*WriteResponse) (*WriteResponse, bool) {
	return &WriteResponse{New: true}, true
}

type replica struct {
	address string
	lis     net.Listener
	id      uint32
	server  *gorums.Server
}

func (r replica) WriteQC(ctx gorums.ServerCtx, request *WriteRequest) (response *WriteResponse, err error) {
	return &WriteResponse{New: true}, nil
}

func (r *replica) createConfiguration(nodeMap map[string]uint32, t *testing.T) {
	srv := gorums.NewServer()
	r.server = srv
	RegisterSampleServer(srv, r)
	go func() {
		err := srv.Serve(r.lis)
		if err != nil {
			log.Printf("Node %s failed to serve\n", r.address)
			t.Fail()
		}
	}()
	time.Sleep(time.Duration(*waitTime) * time.Second)
	mgr := NewManager(gorums.WithDialTimeout(1*time.Second),
		gorums.WithGrpcDialOptions(
			grpc.WithBlock(), // block until connections are made
			grpc.WithTransportCredentials(insecure.NewCredentials()), // disable TLS
		),
	)
	qspec := qspec{}
	_, err := mgr.NewConfiguration(qspec, gorums.WithNodeMap(nodeMap))
	if err != nil {
		log.Printf("unable to create the configuration Error: %v\n", err)
		t.Fail()
	}
	wg.Done()

}

func (r *replica) startListener(t *testing.T) {
	lis, err := net.Listen("tcp", r.address)
	if err != nil {
		log.Fatalf("Failed to listen on '%s': %v\n", r.address, err)
		t.Fail()
	}
	r.lis = lis
}

func (r *replica) startServerAndCreateConfig(nodeMap map[string]uint32, t *testing.T) {
	r.startListener(t)
	r.createConfiguration(nodeMap, t)
}

func (r *replica) stopServer() {
	r.server.Stop()
}
