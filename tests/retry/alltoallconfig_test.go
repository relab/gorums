package retry

import (
	"flag"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/relab/gorums"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var replicaCount = flag.Int("replicas", 20, "number of replicas to create all-to-all communication")

func TestAllToAllConfigurationStyle2(t *testing.T) {
	replicas, err := createReplicas()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		for _, replica := range replicas {
			replica.stopServer()
		}
	}()
	nodeMap := make(map[string]uint32)
	for _, replica := range replicas {
		nodeMap[replica.address] = replica.id
	}
	for _, replica := range replicas {
		replica.createConfiguration(nodeMap)
	}
}

func TestAllToAllConfigurationStyle3(t *testing.T) {
	srvs := make([]*replica, *replicaCount)
	for i := range srvs {
		srvs[i] = &replica{}
	}
	addrs, closeServers := gorums.TestSetup(t, *replicaCount, func(i int) gorums.ServerIface {
		srv := gorums.NewServer()
		RegisterSampleServer(srv, srvs[i])
		return srv
	})
	nodeMap := make(map[string]uint32)
	for i, replica := range srvs {
		srvs[i].address = addrs[i]
		srvs[i].id = uint32(i)
		nodeMap[replica.address] = replica.id
	}
	for _, replica := range srvs {
		replica.createConfiguration(nodeMap)
	}

	teardown := func() {
		for _, replica := range srvs {
			replica.mgr.Close()
		}
		closeServers()
	}
	teardown()
}

func createReplicas() ([]*replica, error) {
	replicas := make([]*replica, 0)
	errChan := make(chan error, *replicaCount)
	defer close(errChan)
	for i := 1; i <= *replicaCount; i++ {
		lis, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, err
		}
		replica := replica{
			address: lis.Addr().String(),
			id:      uint32(i),
			lis:     lis,
			server:  gorums.NewServer(),
		}
		RegisterSampleServer(replica.server, replica)
		replicas = append(replicas, &replica)
		go func() {
			if err := replica.serve(); err != nil {
				errChan <- fmt.Errorf("failed to serve at %q: %w", replica.address, err)
			}
		}()
	}

	select {
	case err := <-errChan:
		return nil, err
	case <-time.After(1000 * time.Millisecond):
		// slept for a bit to allow replica serve goroutines to fail
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
	mgr     *Manager
}

func (r replica) WriteQC(ctx gorums.ServerCtx, request *WriteRequest) (response *WriteResponse, err error) {
	return &WriteResponse{New: true}, nil
}

func (r *replica) createConfiguration(nodeMap map[string]uint32) error {
	r.mgr = NewManager(gorums.WithDialTimeout(100*time.Millisecond),
		gorums.WithGrpcDialOptions(
			grpc.WithBlock(), // block until connections are made
			grpc.WithTransportCredentials(insecure.NewCredentials()), // disable TLS
		),
	)
	_, err := r.mgr.NewConfiguration(qspec{}, gorums.WithNodeMap(nodeMap))
	return err
}

func (r *replica) serve() error {
	return r.server.Serve(r.lis)
}

func (r *replica) stopServer() {
	r.mgr.Close()
	r.server.Stop()
}
