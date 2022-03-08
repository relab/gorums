package all2all

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

func TestAllToAllConfiguration(t *testing.T) {
	replicas, err := createReplicas(*replicaCount)
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

// createReplicas returns a slice of replicas.
// The function waits for all serve goroutines to start and one additional
// second to allow the servers to start before returning.
func createReplicas(numReplicas int) ([]*replica, error) {
	replicas := make([]*replica, 0)
	errChan := make(chan error, numReplicas)
	startedChan := make(chan struct{}, numReplicas)
	for i := 1; i <= numReplicas; i++ {
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
			startedChan <- struct{}{}
			if err := replica.serve(); err != nil {
				errChan <- fmt.Errorf("failed to serve at %q: %w", replica.address, err)
			}
		}()
	}
	for range replicas {
		<-startedChan
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
	server  *gorums.Server // the replica's gRPC server
	mgr     *Manager       // the replica's Gorums manager (used as a client)
}

func (r replica) WriteQC(ctx gorums.ServerCtx, request *WriteRequest) (response *WriteResponse, err error) {
	return &WriteResponse{New: true}, nil
}

// createConfiguration creates a configuration for the replica, allowing
// this replica to communicate with the other replicas in the configuration.
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
