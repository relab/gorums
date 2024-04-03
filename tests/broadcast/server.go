package broadcast

import (
	net "net"

	gorums "github.com/relab/gorums"
	grpc "google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type testServer struct {
	*Server
	addr  string
	peers []string
	lis   net.Listener
	mgr   *Manager
}

func newtestServer(addr string, srvAddresses []string, i int) *testServer {
	// enable profiling
	//go func() {
	//	http.ListenAndServe(fmt.Sprintf(":1000%v", i), nil)
	//}()
	srv := testServer{
		Server: NewServer(),
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
	//slog.Warn("server received broadcast call")
	return &Response{Result: req.Value}, nil
}

func (srv *testServer) QuorumCallWithBroadcast(ctx gorums.ServerCtx, req *Request, broadcast *Broadcast) {
	//slog.Warn("server received quorum call with broadcast")
	broadcast.Broadcast(req)
}

func (srv *testServer) BroadcastCall(ctx gorums.ServerCtx, req *Request, broadcast *Broadcast) {
	//slog.Warn("server received broadcast call")
	broadcast.Broadcast(req)
}

func (srv *testServer) Broadcast(ctx gorums.ServerCtx, req *Request, broadcast *Broadcast) {
	//slog.Warn("server received broadcast")
	broadcast.SendToClient(&Response{
		Result: req.Value,
	}, nil)
}
