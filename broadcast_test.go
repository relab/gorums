package gorums

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/relab/gorums/ordering"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type testBroadcastRequest struct {
	value string
}

func (t *testBroadcastRequest) ProtoReflect() protoreflect.Message {
	return nil
}

type testBroadcastResponse struct{}

func (t *testBroadcastResponse) ProtoReflect() protoreflect.Message {
	return nil
}

//	type testBroadcastImpl interface {
//		Broadcast(ctx gorums.ServerCtx, request *testBroadcastRequest, broadcast *testBroadcast)
//	}
//
//	type testBroadcastServer struct {
//		*gorums.Server
//		numMsgs int
//	}
//
//	func newTestBroadcastServer() *testBroadcastServer {
//		return &testBroadcastServer{
//			Server: gorums.NewServer(),
//		}
//	}
type testBroadcast struct {
	*BroadcastStruct
	sp       *SpBroadcast
	metadata BroadcastMetadata
}

func configureHandlers(b *testBroadcast) func(bh BroadcastHandlerFunc, ch BroadcastReturnToClientHandlerFunc) {
	return func(bh BroadcastHandlerFunc, ch BroadcastReturnToClientHandlerFunc) {
		b.sp.BroadcastHandler = bh
		b.sp.ReturnToClientHandler = ch
	}
}

func configureMetadata(b *testBroadcast) func(metadata BroadcastMetadata) {
	return func(metadata BroadcastMetadata) {
		b.metadata = metadata
	}
}

func (b *testBroadcast) Broadcast(req *testBroadcastRequest, opts ...BroadcastOption) {
	data := NewBroadcastOptions()
	for _, opt := range opts {
		opt(&data)
	}
	b.sp.BroadcastHandler("broadcast", req, b.metadata, data)
}

//
//// Returns a readonly struct of the metadata used in the broadcast.
////
//// Note: Some of the data are equal across the cluster, such as BroadcastID.
//// Other fields are local, such as SenderAddr.
//func (b *testBroadcast) GetMetadata() gorums.BroadcastMetadata {
//	return b.metadata
//}
//
//func (es *testBroadcastServer) Broadcast(ctx gorums.ServerCtx, request *testBroadcastRequest, broadcast *testBroadcast) {
//	es.numMsgs++
//	broadcast.Broadcast(&testBroadcastRequest{})
//}
//
//func createBroadcastServer(ownAddr string, srvAddrs []string) *testBroadcastServer {
//	srv := &testBroadcastServer{
//		Server:  gorums.NewServer(),
//		numMsgs: 0,
//	}
//	srv.RegisterHandler("broadcast", gorums.BroadcastHandler(srv.Broadcast, srv.Server))
//
//	lis, err := net.Listen("tcp", ownAddr)
//	if err != nil {
//		return nil
//	}
//
//	go func() { _ = srv.Serve(lis) }()
//	//defer srv.Stop()
//
//	srv.RegisterView(ownAddr, srvAddrs)
//	srv.ListenForBroadcast()
//	return srv
//}
//
//type testBroadcastConfiguration struct {
//	gorums.RawConfiguration
//	qspec *testBroadcastQSpec
//	srv   *clientServerImpl
//}
//
//type clientServerImpl struct {
//	*gorums.ClientServer
//	grpcServer *grpc.Server
//}
//
//func (c *testBroadcastConfiguration) RegisterClientServer(lis net.Listener, opts ...grpc.ServerOption) error {
//	srvImpl := &clientServerImpl{
//		grpcServer: grpc.NewServer(opts...),
//	}
//	srv, err := gorums.NewClientServer(lis)
//	if err != nil {
//		return err
//	}
//	//srvImpl.grpcServer.RegisterService(&clientServer_ServiceDesc, srvImpl)
//	go srvImpl.grpcServer.Serve(lis)
//	srvImpl.ClientServer = srv
//	c.srv = srvImpl
//	return nil
//}
//
//type testBroadcastQSpec struct {
//	quorumSize int
//}
//
//func newQSpec(qSize int) *testBroadcastQSpec {
//	return &testBroadcastQSpec{
//		quorumSize: qSize,
//	}
//}
//func (qs *testBroadcastQSpec) BroadcastQF(reqs []*testBroadcastResponse) (*testBroadcastResponse, bool) {
//	if len(reqs) < qs.quorumSize {
//		return nil, false
//	}
//	return reqs[0], true
//}
//
//func getConfig(srvAddresses []string, numSrvs int) *testBroadcastConfiguration {
//	mgr := gorums.NewRawManager(
//		gorums.WithDialTimeout(time.Second),
//		gorums.WithGrpcDialOptions(
//			grpc.WithBlock(),
//			grpc.WithTransportCredentials(insecure.NewCredentials()),
//		),
//	)
//	c := &testBroadcastConfiguration{}
//	c.RawConfiguration, _ = gorums.NewRawConfiguration(mgr, gorums.WithNodeList(srvAddresses))
//	c.qspec = newQSpec(numSrvs)
//	//c.RegisterClientServer(gorums.WithListener("localhost:8080"))
//	return c
//}

//func TestBroadcast(t *testing.T) {
//	srv := gorums.NewServer()
//
//	lis, err := net.Listen("tcp", ":0")
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	go func() { _ = srv.Serve(lis) }()
//	defer srv.Stop()
//
//}

type testBroadcastServer struct {
	*Server
	numMsgs int
	req     *testBroadcastRequest
}

func newTestBroadcastServer() *testBroadcastServer {
	srv := &testBroadcastServer{
		Server: NewServer(),
		req:    &testBroadcastRequest{},
	}
	b := &testBroadcast{
		BroadcastStruct: NewBroadcastStruct(),
		sp:              NewSpBroadcastStruct(),
	}
	srv.RegisterBroadcastStruct(b, configureHandlers(b), configureMetadata(b))
	return srv
}

func (srv *testBroadcastServer) Broadcast(ctx ServerCtx, request *testBroadcastRequest, broadcast *testBroadcast) {
	srv.numMsgs++
	srv.req = request
	//broadcast.Broadcast(&testBroadcastRequest{})
}

func createReq(val string) (ServerCtx, *Message, chan<- *Message) {
	var mut sync.Mutex
	mut.Lock()
	srvCtx := ServerCtx{
		Context: context.Background(),
		once:    new(sync.Once),
		mut:     &mut,
	}
	req := newMessage(requestType)
	req.Metadata = &ordering.Metadata{
		BroadcastMsg: &ordering.BroadcastMsg{
			BroadcastID: uuid.New().String(),
		},
	}
	req.Message = &testBroadcastRequest{
		value: val,
	}
	finished := make(chan *Message, 0)
	return srvCtx, req, finished
}

func TestBroadcastHandler(t *testing.T) {
	handlerName := "broadcast"

	// create a server
	srv := newTestBroadcastServer()
	// register the broadcast handler. Similar to proto option: broadcast
	srv.RegisterHandler(handlerName, BroadcastHandler(srv.Broadcast, srv.Server))

	// create a request
	vals := []string{"test1", "test2", "test3"}
	for _, val := range vals {
		srvCtx, req, finished := createReq(val)
		// call the server handler
		srv.srv.handlers[handlerName](srvCtx, req, finished)

		if srv.req.value != val {
			t.Errorf("request.value = %s, expected %s", srv.req.value, val)
		}
	}
}
