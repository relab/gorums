package gorums_test

import (
	"net"
	"testing"
	"time"

	"github.com/relab/gorums"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type testBroadcastRequest struct{}

func (t *testBroadcastRequest) ProtoReflect() protoreflect.Message {
	return nil
}

type testBroadcastResponse struct{}

func (t *testBroadcastResponse) ProtoReflect() protoreflect.Message {
	return nil
}

type testBroadcastImpl interface {
	Broadcast(ctx gorums.ServerCtx, request *testBroadcastRequest, broadcast *testBroadcast)
}

type testBroadcastServer struct {
	*gorums.Server
	numMsgs int
}

type testBroadcast struct {
	*gorums.BroadcastStruct
	sp       *gorums.SpBroadcast
	metadata gorums.BroadcastMetadata
}

func configureHandlers(b *testBroadcast) func(bh gorums.BroadcastHandlerFunc, ch gorums.BroadcastReturnToClientHandlerFunc) {
	return func(bh gorums.BroadcastHandlerFunc, ch gorums.BroadcastReturnToClientHandlerFunc) {
		b.sp.BroadcastHandler = bh
		b.sp.ReturnToClientHandler = ch
	}
}

func configureMetadata(b *testBroadcast) func(metadata gorums.BroadcastMetadata) {
	return func(metadata gorums.BroadcastMetadata) {
		b.metadata = metadata
	}
}

func (b *testBroadcast) Broadcast(req *testBroadcastRequest, opts ...gorums.BroadcastOption) {
	data := gorums.NewBroadcastOptions()
	for _, opt := range opts {
		opt(&data)
	}
	b.sp.BroadcastHandler("broadcast", req, b.metadata, data)
}

// Returns a readonly struct of the metadata used in the broadcast.
//
// Note: Some of the data are equal across the cluster, such as BroadcastID.
// Other fields are local, such as SenderAddr.
func (b *testBroadcast) GetMetadata() gorums.BroadcastMetadata {
	return b.metadata
}

func (es *testBroadcastServer) Broadcast(ctx gorums.ServerCtx, request *testBroadcastRequest, broadcast *testBroadcast) {
	es.numMsgs++
	broadcast.Broadcast(&testBroadcastRequest{})
}

func createBroadcastServer(ownAddr string, srvAddrs []string) *testBroadcastServer {
	srv := &testBroadcastServer{
		Server:  gorums.NewServer(),
		numMsgs: 0,
	}
	srv.RegisterHandler("broadcast", gorums.BroadcastHandler(srv.Broadcast, srv.Server))

	lis, err := net.Listen("tcp", ownAddr)
	if err != nil {
		return nil
	}

	go func() { _ = srv.Serve(lis) }()
	//defer srv.Stop()

	srv.RegisterView(ownAddr, srvAddrs)
	srv.ListenForBroadcast()
	return srv
}

type testBroadcastConfiguration struct {
	gorums.RawConfiguration
	qspec *testBroadcastQSpec
	srv   *clientServerImpl
}

type clientServerImpl struct {
	*gorums.ClientServer
	grpcServer *grpc.Server
}

func (c *testBroadcastConfiguration) RegisterClientServer(lis net.Listener, opts ...grpc.ServerOption) error {
	srvImpl := &clientServerImpl{
		grpcServer: grpc.NewServer(opts...),
	}
	srv, err := gorums.NewClientServer(lis)
	if err != nil {
		return err
	}
	//srvImpl.grpcServer.RegisterService(&clientServer_ServiceDesc, srvImpl)
	go srvImpl.grpcServer.Serve(lis)
	srvImpl.ClientServer = srv
	c.srv = srvImpl
	return nil
}

type testBroadcastQSpec struct {
	quorumSize int
}

func newQSpec(qSize int) *testBroadcastQSpec {
	return &testBroadcastQSpec{
		quorumSize: qSize,
	}
}
func (qs *testBroadcastQSpec) BroadcastQF(reqs []*testBroadcastResponse) (*testBroadcastResponse, bool) {
	if len(reqs) < qs.quorumSize {
		return nil, false
	}
	return reqs[0], true
}

func getConfig(srvAddresses []string, numSrvs int) *testBroadcastConfiguration {
	mgr := gorums.NewRawManager(
		gorums.WithDialTimeout(time.Second),
		gorums.WithGrpcDialOptions(
			grpc.WithBlock(),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	)
	c := &testBroadcastConfiguration{}
	c.RawConfiguration, _ = gorums.NewRawConfiguration(mgr, gorums.WithNodeList(srvAddresses))
	c.qspec = newQSpec(numSrvs)
	//c.RegisterClientServer(gorums.WithListener("localhost:8080"))
	return c
}

func TestBroadcast(t *testing.T) {
	srv := gorums.NewServer()

	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}

	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

}
