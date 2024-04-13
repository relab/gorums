package broadcast

/*import (
	"context"
	"testing"
	"time"

	"github.com/relab/gorums/ordering"
	"github.com/relab/gorums/tests/mock"
	"google.golang.org/grpc/encoding"
)

type broadcaster struct {
}

type orchestrator struct {
}

type mockBroadcastSrv struct{}

func (mockBroadcastSrv) Test(ctx ServerCtx, _ *mock.Request, broadcast *broadcaster) {}

func dummyBroadcastSrv(int) ServerIface {
	mockSrv := &mockBroadcastSrv{}
	srv := NewServer()
	srv.RegisterHandler(handlerName, BroadcastHandler(mockSrv.Test, srv))
	return srv
}

var msgs = []*mock.Request{
	{
		Val: "val1",
	},
	{
		Val: "val2",
	},
	{
		Val: "val3",
	},
	{
		Val: "val4",
	},
	{
		Val: "val5",
	},
}

func TestBroadcast(t *testing.T) {
	if encoding.GetCodec(ContentSubtype) == nil {
		encoding.RegisterCodec(NewCodec())
	}
	// wait to start the server
	addrs, stopSrvs := TestSetup(t, 3, dummyBroadcastSrv)
	defer stopSrvs()

	mgr := dummyMgr()
	defer mgr.Close()
	for _, addr := range addrs {
		node, err := NewRawNode(addr)
		if err != nil {
			t.Fatal(err)
		}
		// a proper connection should NOT be esablished here because server is not started
		node.connect(mgr)
	}

	config, err := NewRawConfiguration(mgr, WithNodeList(addrs))
	if err != nil {
		t.Fatal(err)
	}

	for _, msg := range msgs {
		d := broadcastCallData{
			Message:         in,
			Method:          method,
			BroadcastID:     req.metadata.BroadcastMsg.BroadcastID,
			SenderType:      BroadcastServer,
			SenderID:        srv.id,
			SenderAddr:      srv.addr,
			OriginAddr:      req.metadata.BroadcastMsg.OriginAddr,
			OriginMethod:    req.metadata.BroadcastMsg.OriginMethod,
			ServerAddresses: options.ServerAddresses,
		}
		config.broadcastCall(context.Background(), d)
	}

	// send first message when server is down
	replyChan1 := make(chan response, 1)
	go func() {
		md := &ordering.Metadata{MessageID: 1, Method: handlerName}
		req := request{ctx: context.Background(), msg: &Message{Metadata: md, Message: &mock.Request{}}}
		node.channel.enqueue(req, replyChan1, false)
	}()

	// check response: should be error because server is down
	select {
	case resp := <-replyChan1:
		if resp.err == nil {
			t.Fatal("should have received an error")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("deadlock: impossible to enqueue messages to the node")
	}

	// send second message when server is up
	replyChan2 := make(chan response, 1)
	go func() {
		md := &ordering.Metadata{MessageID: 2, Method: handlerName}
		req := request{ctx: context.Background(), msg: &Message{Metadata: md, Message: &mock.Request{}}, opts: getCallOptions(E_Multicast, nil)}
		node.channel.enqueue(req, replyChan2, false)
	}()

	// check response: error should be nil because server is up
	select {
	case resp := <-replyChan2:
		if resp.err != nil {
			t.Fatal(resp.err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("deadlock: impossible to enqueue messages to the node")
	}

	// stop the server
	stopServer()

	// send third message when server has been previously up but is now down
	replyChan3 := make(chan response, 1)
	go func() {
		md := &ordering.Metadata{MessageID: 3, Method: handlerName}
		req := request{ctx: context.Background(), msg: &Message{Metadata: md, Message: &mock.Request{}}}
		node.channel.enqueue(req, replyChan3, false)
	}()

	// check response: should be error because server is down
	select {
	case resp3 := <-replyChan3:
		if resp3.err == nil {
			t.Fatal("should have received an error", resp3.msg)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("deadlock: impossible to enqueue messages to the node")
	}
}

/*import (
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
	*Broadcaster
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
	b.sp.BroadcastHandler("Broadcast", req, b.metadata, data)
}

func (b *testBroadcast) CanBroadcast(req *testBroadcastRequest, opts ...BroadcastOption) {
	data := NewBroadcastOptions()
	for _, opt := range opts {
		opt(&data)
	}
	b.sp.BroadcastHandler("CanBroadcast", req, b.metadata, data)
}

func (b *testBroadcast) SendToClient(resp protoreflect.ProtoMessage, err error) {
	b.sp.ReturnToClientHandler(resp, err, b.metadata)
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

func (srv *testBroadcastServer) SendToClient(resp protoreflect.ProtoMessage, err error, broadcastID string) {
	srv.RetToClient(resp, err, broadcastID)
}

func newTestBroadcastServer() *testBroadcastServer {
	srv := &testBroadcastServer{
		Server: NewServer(),
		req:    &testBroadcastRequest{},
	}
	b := &testBroadcast{
		Broadcaster: NewBroadcaster(),
		sp:          NewSpBroadcastStruct(),
	}
	srv.RegisterBroadcastStruct(b, configureHandlers(b), configureMetadata(b))
	return srv
}

func (srv *testBroadcastServer) Broadcast(ctx ServerCtx, request *testBroadcastRequest, broadcast *testBroadcast) {
	srv.numMsgs++
	srv.req = request
	//broadcast.Broadcast(&testBroadcastRequest{})
}

func (srv *testBroadcastServer) CanBroadcast(ctx ServerCtx, request *testBroadcastRequest, broadcast *testBroadcast) {
	srv.numMsgs++
	srv.req = request
	go broadcast.Broadcast(&testBroadcastRequest{})
}

func (srv *testBroadcastServer) SendToClientBroadcast(ctx ServerCtx, request *testBroadcastRequest, broadcast *testBroadcast) {
	srv.numMsgs++
	srv.req = request
	go broadcast.SendToClient(&testBroadcastRequest{}, nil)
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
	handlerName := "Broadcast"

	// create a server
	srv := newTestBroadcastServer()
	// register the broadcast handler. Similar to proto option: broadcast
	srv.RegisterHandler(handlerName, BroadcastHandler(srv.Broadcast, srv.Server))

	vals := []string{"test1", "test2", "test3"}
	for _, val := range vals {
		// create the request
		srvCtx, req, finished := createReq(val)
		// call the server handler
		srv.srv.handlers[handlerName](srvCtx, req, finished)

		if srv.req.value != val {
			t.Errorf("request.value = %s, expected %s", srv.req.value, val)
		}
	}
}

func TestCanBroadcastHandler(t *testing.T) {
	handlerName := "CanBroadcast"

	// create a server
	srv := newTestBroadcastServer()
	// register the broadcast handler. Similar to proto option: broadcast
	srv.RegisterHandler(handlerName, BroadcastHandler(srv.CanBroadcast, srv.Server))

	vals := []string{"test1", "test2", "test3"}
	for _, val := range vals {
		// create the request
		srvCtx, req, finished := createReq(val)
		// call the server handler
		srv.srv.handlers[handlerName](srvCtx, req, finished)
		broadcastMsg := <-srv.broadcastSrv.broadcastChan

		if broadcastMsg == nil {
			t.Errorf("broadcastMsg should not be nil")
			continue
		}
		if broadcastMsg.broadcastID != req.Metadata.BroadcastMsg.BroadcastID {
			t.Errorf("broadcastID = %v, expected %s", broadcastMsg.broadcastID, req.Metadata.BroadcastMsg.BroadcastID)
		}
	}
}

func TestBroadcastSendToClient(t *testing.T) {
	handlerName := "SendToClientBroadcast"

	// create a server
	srv := newTestBroadcastServer()
	// register the broadcast handler. Similar to proto option: broadcast
	srv.RegisterHandler(handlerName, BroadcastHandler(srv.SendToClientBroadcast, srv.Server))

	vals := []string{"test1", "test2", "test3"}
	for _, val := range vals {
		// create the request
		srvCtx, req, finished := createReq(val)
		// call the server handler
		srv.srv.handlers[handlerName](srvCtx, req, finished)
		responseMsg := <-srv.broadcastSrv.responseChan

		if responseMsg == nil {
			t.Errorf("responseMsg should not be nil")
			continue
		}
		if responseMsg.getBroadcastID() != req.Metadata.BroadcastMsg.BroadcastID {
			t.Errorf("broadcastID = %v, expected %s", responseMsg.getBroadcastID(), req.Metadata.BroadcastMsg.BroadcastID)
		}
	}
}
*/
