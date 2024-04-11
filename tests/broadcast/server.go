package broadcast

import (
	"context"
	"fmt"
	net "net"
	"sync"

	gorums "github.com/relab/gorums"
	grpc "google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type response struct {
	respChan  chan int64
	messageID int64
}

type testServer struct {
	*Server
	addr     string
	peers    []string
	lis      net.Listener
	mgr      *Manager
	numMsg   map[string]int
	mu       sync.Mutex
	respChan map[int64]response
}

func newtestServer(addr string, srvAddresses []string, _ int) *testServer {
	// enable profiling
	//go func() {
	//	http.ListenAndServe(fmt.Sprintf(":1000%v", i), nil)
	//}()
	srv := testServer{
		Server:   NewServer(),
		numMsg:   map[string]int{"BC": 0, "QC": 0, "QCB": 0, "QCM": 0, "M": 0, "BI": 0, "B": 0},
		respChan: make(map[int64]response),
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
	broadcast.BroadcastIntermediate(req)
}

func (srv *testServer) QuorumCallWithMulticast(ctx gorums.ServerCtx, req *Request) (resp *Response, err error) {
	done := make(chan int64)
	srv.mu.Lock()
	srv.numMsg["QCM"]++
	srv.respChan[req.Value] = response{
		messageID: req.Value,
		respChan:  done,
	}
	srv.mu.Unlock()
	//slog.Warn("server received quorum call with broadcast")
	srv.View.MulticastIntermediate(context.Background(), req, gorums.WithNoSendWaiting())
	ctx.Release()
	res := <-done
	return &Response{Result: res}, nil
}

func (srv *testServer) MulticastIntermediate(ctx gorums.ServerCtx, req *Request) {
	srv.mu.Lock()
	srv.numMsg["M"]++
	srv.mu.Unlock()
	ctx.Release()
	srv.View.Multicast(context.Background(), req, gorums.WithNoSendWaiting())
}

func (srv *testServer) Multicast(ctx gorums.ServerCtx, req *Request) {
	ctx.Release()
	srv.mu.Lock()
	defer srv.mu.Unlock()
	srv.numMsg["M"]++
	if response, ok := srv.respChan[req.Value]; ok {
		response.respChan <- req.Value
		close(response.respChan)
		delete(srv.respChan, req.Value)
	}
	//slog.Warn("server received quorum call with broadcast")
}

func (srv *testServer) BroadcastCall(ctx gorums.ServerCtx, req *Request, broadcast *Broadcast) {
	//srv.mu.Lock()
	//srv.numMsg["BC"]++
	//srv.mu.Unlock()
	//slog.Warn("server received broadcast call")
	broadcast.BroadcastIntermediate(req)
}

func (srv *testServer) BroadcastIntermediate(ctx gorums.ServerCtx, req *Request, broadcast *Broadcast) {
	//srv.mu.Lock()
	//srv.numMsg["BI"]++
	//srv.mu.Unlock()
	//slog.Warn("server received broadcast intermediate")
	broadcast.Broadcast(req)
}

func (srv *testServer) Broadcast(ctx gorums.ServerCtx, req *Request, broadcast *Broadcast) {
	//srv.mu.Lock()
	//srv.numMsg["B"]++
	//srv.mu.Unlock()
	//slog.Warn("server received broadcast")
	broadcast.SendToClient(&Response{
		Result: req.Value,
	}, nil)
}

func (srv *testServer) GetMsgs() string {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	res := "Srv " + srv.addr
	res += fmt.Sprintf(" -> QC: %d, QCB: %d, QCM: %d, M: %d, BC: %d, BI: %d, B: %d", srv.numMsg["QC"], srv.numMsg["QCB"], srv.numMsg["QCM"], srv.numMsg["M"], srv.numMsg["BC"], srv.numMsg["BI"], srv.numMsg["B"])
	return res
}

func (srv *testServer) GetNumMsgs() int {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	return srv.numMsg["BC"] + srv.numMsg["B"] + srv.numMsg["BI"]
}
