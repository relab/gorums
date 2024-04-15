package gorums

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/relab/gorums/ordering"
	"github.com/relab/gorums/tests/mock"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type mockRouter struct {
}

func (m *mockRouter) Send(broadcastID, addr, method string, msg any) error {
	return nil
}
func (m *mockRouter) SendOrg(broadcastID string, data content, msg any) error {
	return nil
}
func (m *mockRouter) CreateConnection(addr string) {
}
func (m *mockRouter) AddAddr(id uint32, addr string) {
}
func (m *mockRouter) AddServerHandler(method string, handler serverHandler) {
}
func (m *mockRouter) AddClientHandler(method string, handler clientHandler) {
}

var newBroadcaster = func(BroadcastMetadata, *BroadcastOrchestrator) Broadcaster {
	return &testBroadcaster{}
}

type testResp struct{}
type testBroadcaster struct{}

func testHandler(ServerCtx, *mock.Request, *testBroadcaster) {}

const (
	clientHandlerName = "client"
	serverHandlerName = "test"
)

type testingInterface interface {
	Error(...any)
}

func setup[T testingInterface](t T) func(ServerCtx, *Message, chan<- *Message) {
	srv := NewServer()
	srv.broadcastSrv.router = &mockRouter{}
	srv.broadcastSrv.orchestrator = NewBroadcastOrchestrator(srv)
	srv.broadcastSrv.createBroadcaster = newBroadcaster
	srv.broadcastSrv.registerBroadcastFunc(serverHandlerName)
	listener, err := net.Listen("tcp", "127.0.0.1:5000")
	if err != nil {
		t.Error(err)
	}
	srv.broadcastSrv.addAddr(listener)
	listener.Close()
	var mut sync.Mutex
	mut.Lock()
	clientHandlerMock := func(broadcastID string, req protoreflect.ProtoMessage, cc *grpc.ClientConn, timeout time.Duration, opts ...grpc.CallOption) (any, error) {
		return nil, nil
	}
	srv.broadcastSrv.registerSendToClientHandler(clientHandlerName, clientHandlerMock)
	handler := BroadcastHandler2(testHandler, srv)
	return handler
}

type requester struct {
	mut      sync.Mutex
	finished chan *Message
	handler  func(ServerCtx, *Message, chan<- *Message)
}

func newRequester(handler func(ServerCtx, *Message, chan<- *Message)) *requester {
	return &requester{
		finished: make(chan *Message),
		handler:  handler,
	}
}

func (r *requester) sendReq(val string) {
	req := newMessage(requestType)
	req.Message = &mock.Request{
		Val: val,
	}
	bMsg := &ordering.BroadcastMsg{
		BroadcastID: uuid.NewString(),
		SenderType:  BroadcastClient,
	}
	req.Metadata.BroadcastMsg = bMsg
	ctx := context.Background()
	r.mut.Lock()
	r.handler(ServerCtx{Context: ctx, once: new(sync.Once), mut: &r.mut}, req, r.finished)
}
func TestBroadcastID(t *testing.T) {
	machineID := NewMachineID("127.0.0.1")
	id := NewBroadcastID(machineID, 2)
	fmt.Println(id)
	fmt.Printf("0b%08b\n", id)
}

func TestBroadcastFunc(t *testing.T) {
	r := newRequester(setup(t))
	start := time.Now()
	r.sendReq("test")
	fmt.Println(time.Since(start))
}

func BenchmarkBroadcastFunc2(b *testing.B) {
	r := newRequester(setup(b))

	b.ResetTimer()
	b.Run("BroadcastFunc", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			r.sendReq(strconv.Itoa(i))
		}
	})
}

func BenchmarkBroadcastFunc1(b *testing.B) {
	r := newRequester(setup(b))
	numClients := 100

	b.ResetTimer()
	b.Run("BroadcastFunc", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			for c := 0; c < numClients; c++ {
				r.sendReq(strconv.Itoa(i))
			}
		}
	})
}

func BenchmarkBroadcastFuncE(b *testing.B) {
	r := newRequester(setup(b))
	numClients := 100

	b.ResetTimer()
	for c := 0; c < numClients; c++ {
		b.RunParallel(func(pb *testing.PB) {
			for i := 0; pb.Next(); i++ {
				r.sendReq(strconv.Itoa(i))
			}
		})
	}
}
