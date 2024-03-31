package broadcast

import (
	"context"
	fmt "fmt"
	"log/slog"
	net "net"
	"testing"
	"time"

	gorums "github.com/relab/gorums"
	grpc "google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type testServer struct {
	*Server
	addr  string
	peers []string
	lis   net.Listener
}

func (srv *testServer) BroadcastCall(ctx gorums.ServerCtx, req *Request, broadcast *Broadcast) {
	slog.Warn("server received req")
	broadcast.Broadcast(req)
}

func (srv *testServer) Broadcast(ctx gorums.ServerCtx, req *Request, broadcast *Broadcast) {
	broadcast.SendToClient(&Response{
		Result: req.Value,
	}, nil)
}

type testQSpec struct {
	quorumSize    int
	broadcastSize int
}

func newQSpec(qSize, broadcastSize int) QuorumSpec {
	return &testQSpec{
		quorumSize:    qSize,
		broadcastSize: broadcastSize,
	}
}

func (qs *testQSpec) BroadcastCallQF(replies []*Response) (*Response, bool) {
	if len(replies) >= qs.quorumSize {
		for _, resp := range replies {
			return resp, true
		}
	}
	return nil, false
}

func newtestServer(addr string, srvAddresses []string) *testServer {
	srv := testServer{
		Server: NewServer(),
	}
	RegisterBroadcastServiceServer(srv.Server, &srv)
	srv.peers = srvAddresses
	srv.addr = addr
	return &srv
}

func (srv *testServer) start(lis net.Listener) {
	mgr := NewManager(
		gorums.WithGrpcDialOptions(
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	)
	view, err := mgr.NewConfiguration(gorums.WithNodeList(srv.peers))
	if err != nil {
		panic(err)
	}
	srv.SetView(view)
	srv.Serve(lis)
}

func createSrvs(numSrvs int) ([]string, func(), error) {
	srvs := make([]*testServer, 0, numSrvs)
	srvAddrs := make([]string, numSrvs)
	for i := 0; i < numSrvs; i++ {
		srvAddrs[i] = fmt.Sprintf("127.0.0.1:500%v", i)
	}
	for _, addr := range srvAddrs {
		srv := newtestServer(addr, srvAddrs)
		lis, err := net.Listen("tcp4", srv.addr)
		if err != nil {
			return nil, nil, err
		}
		srv.lis = lis
		go srv.start(lis)
		srvs = append(srvs, srv)
	}
	return srvAddrs, func() {
		// stop the servers
		for _, srv := range srvs {
			srv.Stop()
			srv.lis.Close()
		}
	}, nil
}

func createClient(srvAddrs []string, listenAddr string) (*Configuration, func(), error) {
	mgr := NewManager(
		gorums.WithGrpcDialOptions(
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	)
	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, nil, err
	}
	err = mgr.AddClientServer(lis)
	if err != nil {
		return nil, nil, err
	}
	config, err := mgr.NewConfiguration(
		gorums.WithNodeList(srvAddrs),
		newQSpec(len(srvAddrs), len(srvAddrs)),
	)
	if err != nil {
		return nil, nil, err
	}
	return config, func() {
		mgr.Close()
	}, nil
}

func TestBroadcast(t *testing.T) {
	srvAddrs, srvCleanup, err := createSrvs(3)
	if err != nil {
		t.Error(err)
	}
	defer srvCleanup()

	config, clientCleanup, err := createClient(srvAddrs, "127.0.0.1:8080")
	if err != nil {
		t.Error(err)
	}
	defer clientCleanup()

	val := int64(1)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	resp, err := config.BroadcastCall(ctx, &Request{Value: val})
	if err != nil {
		t.Error(err)
	}
	if resp.GetResult() != val {
		t.Fatal("resp is wrong")
	}
}

func BenchmarkBroadcast(b *testing.B) {
	srvAddrs, srvCleanup, err := createSrvs(3)
	if err != nil {
		b.Error(err)
	}
	defer srvCleanup()

	config, clientCleanup, err := createClient(srvAddrs, "127.0.0.1:8080")
	if err != nil {
		b.Error(err)
	}
	defer clientCleanup()

	b.Run(fmt.Sprintf("BroadcastCall_%d", 1), func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			resp, err := config.BroadcastCall(context.Background(), &Request{Value: int64(i)})
			if err != nil {
				_ = resp.GetResult()
			}
		}
	})

}

/*import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/relab/gorums"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type testServer struct {
	*Server
}

type testQSpec struct {
	quorum int
}

func (q testQSpec) SliceUseReqQF(in *Request, replies []*Response) (*Response, bool) {
	if len(replies) < q.quorum {
		return nil, false
	}
	expected := in.GetValue()
	for _, reply := range replies {
		if expected != reply.GetResult() {
			return reply, true
		}
	}
	return replies[0], true
}

func (q testQSpec) SliceIgnoreReqQF(_ *Request, replies []*Response) (*Response, bool) {
	if len(replies) < q.quorum {
		return nil, false
	}
	expected := int64(requestValue)
	for _, reply := range replies {
		if expected != reply.GetResult() {
			return reply, true
		}
	}
	return replies[0], true
}

func (q testQSpec) SliceWithoutReqQF(replies []*Response) (*Response, bool) {
	if len(replies) < q.quorum {
		return nil, false
	}
	expected := int64(requestValue)
	for _, reply := range replies {
		if expected != reply.GetResult() {
			return reply, true
		}
	}
	return replies[0], true
}

func (q testQSpec) UseReqQF(in *Request, replies map[uint32]*Response) (*Response, bool) {
	if len(replies) < q.quorum {
		return nil, false
	}
	_ = in.GetValue()
	var reply *Response
	for _, r := range replies {
		reply = r
		break
	}
	return reply, true
}

func (q testQSpec) IgnoreReqQF(_ *Request, replies map[uint32]*Response) (*Response, bool) {
	if len(replies) < q.quorum {
		return nil, false
	}
	var reply *Response
	for _, r := range replies {
		reply = r
		break
	}
	return reply, true
}

func (q testQSpec) WithoutReqQF(replies map[uint32]*Response) (*Response, bool) {
	if len(replies) < q.quorum {
		return nil, false
	}
	var reply *Response
	for _, r := range replies {
		reply = r
		break
	}
	return reply, true
}

func BenchmarkQF(b *testing.B) {
	for n := 3; n < 20; n += 2 {
		quorum := n / 2
		mapSize := n << 1
		qspec := &testQSpec{quorum: quorum}
		request := &Request{Value: 1}

		b.Run(fmt.Sprintf("UseReq_%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				replies := make(map[uint32]*Response, mapSize)
				for j := 0; j < n; j++ {
					replies[uint32(j)] = &Response{Result: request.Value}
					resp, q := qspec.UseReqQF(request, replies)
					if q {
						_ = resp.GetResult()
					}
				}
			}
		})
		b.Run(fmt.Sprintf("IgnoreReq_%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				replies := make(map[uint32]*Response, mapSize)
				for j := 0; j < n; j++ {
					replies[uint32(j)] = &Response{Result: request.Value}
					resp, q := qspec.IgnoreReqQF(request, replies)
					if q {
						_ = resp.GetResult()
					}
				}
			}
		})
		b.Run(fmt.Sprintf("WithoutReq_%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				replies := make(map[uint32]*Response, mapSize)
				for j := 0; j < n; j++ {
					replies[uint32(j)] = &Response{Result: request.Value}
					resp, q := qspec.WithoutReqQF(replies)
					if q {
						_ = resp.GetResult()
					}
				}
			}
		})

		// Slice versions

		b.Run(fmt.Sprintf("SliceUseReq_%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				replies := make([]*Response, n)
				for j := 0; j < n; j++ {
					replies[uint32(j)] = &Response{Result: request.Value}
					resp, q := qspec.SliceUseReqQF(request, replies)
					if q {
						_ = resp.GetResult()
					}
				}
			}
		})
		b.Run(fmt.Sprintf("SliceIgnoreReq_%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				replies := make([]*Response, n)
				for j := 0; j < n; j++ {
					replies[uint32(j)] = &Response{Result: request.Value}
					resp, q := qspec.SliceIgnoreReqQF(request, replies)
					if q {
						_ = resp.GetResult()
					}
				}
			}
		})
		b.Run(fmt.Sprintf("SliceWithoutReq_%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				replies := make([]*Response, n)
				for j := 0; j < n; j++ {
					replies[uint32(j)] = &Response{Result: request.Value}
					resp, q := qspec.SliceWithoutReqQF(replies)
					if q {
						_ = resp.GetResult()
					}
				}
			}
		})
	}
}

type testSrv struct{}

func (s testSrv) UseReq(_ gorums.ServerCtx, req *Request) (resp *Response, err error) {
	return &Response{Result: req.GetValue()}, nil
}

func (s testSrv) IgnoreReq(_ gorums.ServerCtx, req *Request) (resp *Response, err error) {
	return &Response{Result: req.GetValue()}, nil
}

func BenchmarkFullStackQF(b *testing.B) {
	for n := 3; n < 20; n += 2 {
		_, stop := gorums.TestSetup(b, n, func(_ int) gorums.ServerIface {
			srv := gorums.NewServer()
			RegisterQuorumFunctionServer(srv, &testSrv{})
			return srv
		})
		mgr := NewManager(
			gorums.WithDialTimeout(10*time.Second),
			gorums.WithGrpcDialOptions(
				grpc.WithTransportCredentials(insecure.NewCredentials()),
			),
		)
		c, err := mgr.NewConfiguration(
			&testQSpec{quorum: n / 2},
			gorums.WithNodeList([]string{"127.0.0.1:9080", "127.0.0.1:9081", "127.0.0.1:9082"}), // dummy node list; won't actually be used in test
		)
		if err != nil {
			b.Fatal(err)
		}

		b.ReportAllocs()
		b.ResetTimer()

		b.Run(fmt.Sprintf("UseReq_%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				resp, err := c.UseReq(context.Background(), &Request{Value: int64(requestValue)})
				if err != nil {
					b.Fatalf("UseReq error: %v", err)
				}
				_ = resp.GetResult()
			}
		})
		b.Run(fmt.Sprintf("IgnoreReq_%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				resp, err := c.IgnoreReq(context.Background(), &Request{Value: int64(requestValue)})
				if err != nil {
					b.Fatalf("IgnoreReq error: %v", err)
				}
				_ = resp.GetResult()
			}
		})
		// close manager and stop gRPC servers;
		// must be done for each iteration
		mgr.Close()
		stop()
	}
}
*/
