package retry

import (
	"flag"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/relab/gorums"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	replicaCount = flag.Int("replicaCount", 80, "number of replicas to create all-to-all communication")
	waitTime     = flag.Int("waitTime", 2, "Seconds to wait before forming the configuration")
)

func TestAllToAllConfigurationStyle1(t *testing.T) {
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
	g := new(errgroup.Group)
	for _, replica := range replicaList {
		replica := replica
		g.Go(func() error {
			err := replica.startListener()
			if err != nil {
				return err
			}
			return replica.createConfiguration(nodeMap)
		})
	}
	if err := g.Wait(); err != nil {
		t.Fatal(err)
	}
	t.Log("Successful TestAllToAllConfigurationStyle1 completion")
}

func TestAllToAllConfigurationStyle2(t *testing.T) {
	replicas, err := createReplicas()
	if err != nil {
		t.Fatal(err)
	}
	nodeMap := make(map[string]uint32)
	for _, replica := range replicas {
		nodeMap[replica.address] = replica.id
	}
	g := new(errgroup.Group)
	for _, replica := range replicas {
		replica := replica
		g.Go(func() error { return replica.createConfiguration(nodeMap) })
	}
	if err := g.Wait(); err != nil {
		t.Fatal(err)
	}
	t.Log("Successful TestAllToAllConfigurationStyle2 completion")
}

func createReplicas() ([]*replica, error) {
	replicas := make([]*replica, 0)
	for i := 1; i <= *replicaCount; i++ {
		lis, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, err
		}
		replica := replica{
			address: lis.Addr().String(),
			id:      uint32(i),
			lis:     lis,
		}
		replicas = append(replicas, &replica)
	}
	return replicas, nil
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

func (r *replica) createConfiguration(nodeMap map[string]uint32) error {
	srv := gorums.NewServer()
	r.server = srv
	RegisterSampleServer(srv, r)
	errChan := make(chan error)
	go func() {
		if err := srv.Serve(r.lis); err != nil {
			errChan <- fmt.Errorf("failed to serve at %q: %w", r.address, err)
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
		return err
	}
	close(errChan)
	// since errChan is closed, this should either return an error or nil
	return <-errChan
}

func (r *replica) startListener() error {
	lis, err := net.Listen("tcp", r.address)
	if err != nil {
		return fmt.Errorf("failed to listen at %q: %w", r.address, err)
	}
	r.lis = lis
	return nil
}

func (r *replica) stopServer() {
	r.server.Stop()
}