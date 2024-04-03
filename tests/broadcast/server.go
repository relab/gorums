package broadcast

import (
	"fmt"
	net "net"
	"sync"

	gorums "github.com/relab/gorums"
	grpc "google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type testServer struct {
	*Server
	addr   string
	peers  []string
	lis    net.Listener
	mgr    *Manager
	numMsg map[string]int
	mu     sync.Mutex
}

func newtestServer(addr string, srvAddresses []string, i int) *testServer {
	// enable profiling
	//go func() {
	//	http.ListenAndServe(fmt.Sprintf(":1000%v", i), nil)
	//}()
	srv := testServer{
		Server: NewServer(),
		numMsg: map[string]int{"BC": 0, "QC": 0, "QCB": 0, "B": 0},
	}
	RegisterBroadcastServiceServer(srv.Server, &srv)
	srv.peers = srvAddresses
	srv.addr = addr
	return &srv
}

func (srv *testServer) start(lis net.Listener) {
	srv.mgr = NewManager(
		gorums.WithPublicKey("server"),
		gorums.WithGrpcDialOptions(
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	)
	view, err := srv.mgr.NewConfiguration(gorums.WithNodeList(srv.peers))
	if err != nil {
		panic(err)
	}
	srv.SetView(view)
	srv.Serve(lis)
}

func (srv *testServer) Stop() {
	srv.Server.Stop()
	srv.mgr.Close()
}

func (srv *testServer) QuorumCall(ctx gorums.ServerCtx, req *Request) (resp *Response, err error) {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	srv.numMsg["QC"]++
	//slog.Warn("server received broadcast call")
	return &Response{Result: req.Value}, nil
}

func (srv *testServer) QuorumCallWithBroadcast(ctx gorums.ServerCtx, req *Request, broadcast *Broadcast) {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	srv.numMsg["QCB"]++
	//slog.Warn("server received quorum call with broadcast")
	broadcast.Broadcast(req)
}

func (srv *testServer) BroadcastCall(ctx gorums.ServerCtx, req *Request, broadcast *Broadcast) {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	srv.numMsg["BC"]++
	//slog.Warn("server received broadcast call")
	broadcast.Broadcast(req)
}

func (srv *testServer) Broadcast(ctx gorums.ServerCtx, req *Request, broadcast *Broadcast) {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	srv.numMsg["B"]++
	//slog.Warn("server received broadcast")
	broadcast.SendToClient(&Response{
		Result: req.Value,
	}, nil)
}

func (srv *testServer) GetMsgs() string {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	res := "Srv " + srv.addr
	res += fmt.Sprintf(" -> QC: %d, QCB: %d, BC: %d, B: %d", srv.numMsg["QC"], srv.numMsg["QCB"], srv.numMsg["BC"], srv.numMsg["B"])
	return res
}
