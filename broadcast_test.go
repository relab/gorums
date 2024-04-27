package gorums

/*import (
	"context"
	"fmt"
	"net"
	"os"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/relab/gorums/broadcast"
	"github.com/relab/gorums/ordering"
	"github.com/relab/gorums/tests/mock"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type mockRouter struct {
}

func (m *mockRouter) Send(broadcastID uint64, addr, method string, msg any) error {
	return nil
}
func (m *mockRouter) CreateConnection(addr string) {
}
func (m *mockRouter) AddAddr(id uint32, addr string) {
}
func (m *mockRouter) AddServerHandler(method string, handler broadcast.ServerHandler) {
}
func (m *mockRouter) AddClientHandler(method string, handler broadcast.ClientHandler) {
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

func setup[T testingInterface](t T) (func(ServerCtx, *Message, chan<- *Message), func()) {
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
	clientHandlerMock := func(broadcastID uint64, req protoreflect.ProtoMessage, cc *grpc.ClientConn, timeout time.Duration, opts ...grpc.CallOption) (any, error) {
		return nil, nil
	}
	srv.broadcastSrv.registerSendToClientHandler(clientHandlerName, clientHandlerMock)
	handler := BroadcastHandler(testHandler, srv)
	return handler, func() {
		srv.Stop()
	}
}

type requester struct {
	mut       sync.Mutex
	finished  chan *Message
	handler   func(ServerCtx, *Message, chan<- *Message)
	snowflake *broadcast.Snowflake
}

func newRequester(handler func(ServerCtx, *Message, chan<- *Message)) *requester {
	return &requester{
		finished:  make(chan *Message),
		handler:   handler,
		snowflake: broadcast.NewSnowflake("test"),
	}
}

func (r *requester) sendReq(val string) {
	req := newMessage(requestType)
	req.Message = &mock.Request{
		Val: val,
	}
	bMsg := &ordering.BroadcastMsg{
		BroadcastID:       r.snowflake.NewBroadcastID(),
		IsBroadcastClient: true,
	}
	req.Metadata.BroadcastMsg = bMsg
	ctx := context.Background()
	r.mut.Lock()
	r.handler(ServerCtx{Context: ctx, once: new(sync.Once), mut: &r.mut}, req, r.finished)
}
func TestBroadcastID(t *testing.T) {
	snowflake := broadcast.NewSnowflake("127.0.0.1")
	machineID := snowflake.MachineID
	timestampDistribution := make(map[uint32]int)
	shardDistribution := make(map[uint16]int)
	maxN := 262144
	for j := 1; j < 3*maxN; j++ {
		i := j % maxN
		broadcastID := snowflake.NewBroadcastID()
		timestamp, shard, m, n := broadcast.DecodeBroadcastID(broadcastID)
		if i != int(n) {
			t.Errorf("wrong sequence number. want: %v, got: %v", i, n)
		}
		if m >= 4096 {
			t.Errorf("machine ID cannot be higher than max. want: %v, got: %v", 4095, m)
		}
		if m != uint16(machineID) {
			t.Errorf("wrong machine ID. want: %v, got: %v", machineID, m)
		}
		if shard >= 16 {
			t.Errorf("cannot have higher shard than max. want: %v, got: %v", 15, shard)
		}
		if n >= uint32(maxN) {
			t.Errorf("sequence number cannot be higher than max. want: %v, got: %v", maxN, n)
		}
		timestampDistribution[timestamp]++
		shardDistribution[shard]++
		//fmt.Println(timestamp, shard, m, n)
	}
	for k, v := range timestampDistribution {
		if v > maxN {
			t.Errorf("cannot have more than maxN in a second. want: %v, got: %v", maxN, k)
		}
	}
	fmt.Println(timestampDistribution)
	fmt.Println(shardDistribution)
	//fmt.Printf("0b%08b\n", broadcastID)
	//if machineID != m {
	//t.Errorf("machineID is wrong. got: %v, want: %v", m, machineID)
	//}
}

func TestBroadcastFunc(t *testing.T) {
	handler, cleanup := setup(t)
	r := newRequester(handler)
	start := time.Now()
	r.sendReq("test")
	fmt.Println(time.Since(start))
	cleanup()
}

func BenchmarkBroadcastFunc2(b *testing.B) {
	handler, cleanup := setup(b)
	r := newRequester(handler)

	b.ResetTimer()
	b.Run("BroadcastFunc", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			r.sendReq(strconv.Itoa(i))
		}
	})
	b.StopTimer()
	cleanup()
}

func BenchmarkBroadcastFunc1(b *testing.B) {
	handler, cleanup := setup(b)
	r := newRequester(handler)
	numClients := 100

	b.ResetTimer()
	b.Run("BroadcastFunc", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			for c := 0; c < numClients; c++ {
				r.sendReq(strconv.Itoa(i))
			}
		}
	})
	b.StopTimer()
	cleanup()
}

func BenchmarkBroadcastFuncT(b *testing.B) {
	handler, cleanup := setup(b)
	r := newRequester(handler)
	numClients := 100

	stop, err := StartTrace("traceprofileBC")
	if err != nil {
		b.Error(err)
	}
	defer stop()
	cpuProfile, _ := os.Create("cpuprofileBC")
	pprof.StartCPUProfile(cpuProfile)
	b.ResetTimer()
	for c := 0; c < numClients; c++ {
		b.RunParallel(func(pb *testing.PB) {
			for i := 0; pb.Next(); i++ {
				r.sendReq(strconv.Itoa(i))
			}
		})
	}
	b.StopTimer()
	pprof.StopCPUProfile()
	cpuProfile.Close()
	cleanup()
}
func BenchmarkBroadcastFuncM(b *testing.B) {
	handler, cleanup := setup(b)
	r := newRequester(handler)
	numClients := 100

	stop, err := StartTrace("traceprofileBC")
	if err != nil {
		b.Error(err)
	}
	defer stop()
	memProfile, _ := os.Create("memprofileBC")
	runtime.GC()
	b.ResetTimer()
	for c := 0; c < numClients; c++ {
		b.RunParallel(func(pb *testing.PB) {
			for i := 0; pb.Next(); i++ {
				r.sendReq(strconv.Itoa(i))
			}
		})
	}
	b.StopTimer()
	pprof.WriteHeapProfile(memProfile)
	memProfile.Close()
	cleanup()
}
func BenchmarkBroadcastFuncC(b *testing.B) {
	handler, cleanup := setup(b)
	r := newRequester(handler)
	numClients := 100

	stop, err := StartTrace("traceprofileBC")
	if err != nil {
		b.Error(err)
	}
	defer stop()
	b.ResetTimer()
	for c := 0; c < numClients; c++ {
		b.RunParallel(func(pb *testing.PB) {
			for i := 0; pb.Next(); i++ {
				r.sendReq(strconv.Itoa(i))
			}
		})
	}
	b.StopTimer()
	cleanup()
}

func StartTrace(tracePath string) (stop func() error, err error) {
	traceFile, err := os.Create(tracePath)
	if err != nil {
		return nil, err
	}
	if err := trace.Start(traceFile); err != nil {
		return nil, err
	}
	return func() error {
		trace.Stop()
		err = traceFile.Close()
		if err != nil {
			return err
		}
		return nil
	}, nil
}
*/
