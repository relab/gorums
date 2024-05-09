package broadcast

import (
	"context"
	"fmt"
	net "net"
	"sync"
	"time"

	gorums "github.com/relab/gorums"
	grpc "google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var leader = "127.0.0.1:5000"

type response struct {
	respChan  chan int64
	messageID int64
}

type testServer struct {
	*Server
	mut            sync.Mutex
	leader         string
	addr           string
	peers          []string
	lis            net.Listener
	mgr            *Manager
	numMsg         map[string]int
	respChan       map[int64]response
	processingTime time.Duration
	val            int64
}

func newtestServer(addr string, srvAddresses []string, _ int) *testServer {
	// enable profiling
	//go func() {
	//	http.ListenAndServe(fmt.Sprintf(":1000%v", i), nil)
	//}()
	srv := testServer{
		Server:   NewServer(gorums.WithMetrics()),
		numMsg:   map[string]int{"BC": 0, "QC": 0, "QCB": 0, "QCM": 0, "M": 0, "BI": 0, "B": 0},
		respChan: make(map[int64]response),
		leader:   leader,
	}
	RegisterBroadcastServiceServer(srv.Server, &srv)
	srv.peers = srvAddresses
	srv.addr = addr
	if addr != leader {
		srv.processingTime = 100 * time.Millisecond
	}
	srv.mgr = NewManager(
		gorums.WithSendBufferSize(uint(2*len(srvAddresses))),
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
	return &srv
}

func (srv *testServer) start(lis net.Listener) {
	srv.Serve(lis)
}

func (srv *testServer) Stop() {
	srv.Server.Stop()
	srv.mgr.Close()
}

func (srv *testServer) QuorumCall(ctx gorums.ServerCtx, req *Request) (resp *Response, err error) {
	srv.mut.Lock()
	defer srv.mut.Unlock()
	srv.numMsg["QC"]++
	//slog.Warn("server received broadcast call")
	return &Response{Result: req.Value}, nil
}

func (srv *testServer) QuorumCallWithBroadcast(ctx gorums.ServerCtx, req *Request, broadcast *Broadcast) {
	srv.mut.Lock()
	defer srv.mut.Unlock()
	srv.numMsg["QCB"]++
	//slog.Warn("server received quorum call with broadcast")
	broadcast.BroadcastIntermediate(req)
}

func (srv *testServer) QuorumCallWithMulticast(ctx gorums.ServerCtx, req *Request) (resp *Response, err error) {
	done := make(chan int64)
	srv.mut.Lock()
	srv.numMsg["QCM"]++
	srv.respChan[req.Value] = response{
		messageID: req.Value,
		respChan:  done,
	}
	srv.mut.Unlock()
	//slog.Warn("server received quorum call with broadcast")
	srv.View.MulticastIntermediate(context.Background(), req, gorums.WithNoSendWaiting())
	ctx.Release()
	res := <-done
	return &Response{Result: res}, nil
}

func (srv *testServer) MulticastIntermediate(ctx gorums.ServerCtx, req *Request) {
	srv.mut.Lock()
	srv.numMsg["M"]++
	srv.mut.Unlock()
	ctx.Release()
	srv.View.Multicast(context.Background(), req, gorums.WithNoSendWaiting())
}

func (srv *testServer) Multicast(ctx gorums.ServerCtx, req *Request) {
	ctx.Release()
	srv.mut.Lock()
	defer srv.mut.Unlock()
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
	//md := broadcast.GetMetadata()
	//slog.Warn("server received broadcast call", "srv", srv.addr, "bID", md.BroadcastID)
	//time.Sleep(1 * time.Millisecond)
	//broadcast.SendToClient(&Response{
	//Result: req.Value,
	//}, nil)
	/*broadcast.SendToClient(&Response{
		Result: req.Value,
	}, nil)*/
	//time.Sleep(1 * time.Millisecond)
	broadcast.BroadcastIntermediate(req)
}

func (srv *testServer) BroadcastIntermediate(ctx gorums.ServerCtx, req *Request, broadcast *Broadcast) {
	//srv.mu.Lock()
	//srv.numMsg["BI"]++
	//srv.mu.Unlock()
	///md := broadcast.GetMetadata()
	///slog.Warn("server received broadcast intermediate", "srv", srv.addr, "bID", md.BroadcastID)
	//broadcast.SendToClient(&Response{
	//Result: req.Value,
	//}, nil)
	//time.Sleep(1 * time.Millisecond)
	//time.Sleep(1 * time.Millisecond)
	broadcast.Broadcast(req)
}

func (srv *testServer) Broadcast(ctx gorums.ServerCtx, req *Request, broadcast *Broadcast) {
	//srv.mu.Lock()
	//srv.numMsg["B"]++
	//srv.mu.Unlock()
	///md := broadcast.GetMetadata()
	///slog.Warn("server received broadcast", "srv", srv.addr, "bID", md.BroadcastID)
	//time.Sleep(1 * time.Millisecond)
	//time.Sleep(1 * time.Millisecond)
	broadcast.SendToClient(&Response{
		Result: req.Value,
	}, nil)
}

func (srv *testServer) BroadcastCallForward(ctx gorums.ServerCtx, req *Request, broadcast *Broadcast) {
	//srv.mu.Lock()
	//srv.numMsg["BC"]++
	//srv.mu.Unlock()
	//slog.Warn("server received broadcast call")
	if srv.addr != srv.leader {
		broadcast.Forward(req, srv.leader)
		return
	}
	broadcast.Broadcast(req)
}

func (srv *testServer) BroadcastCallTo(ctx gorums.ServerCtx, req *Request, broadcast *Broadcast) {
	//srv.mu.Lock()
	//srv.numMsg["BC"]++
	//srv.mu.Unlock()
	//slog.Warn("server received broadcast call")
	broadcast.To(srv.leader).BroadcastToResponse(req) // only broadcast to the leader
}

func (srv *testServer) BroadcastToResponse(ctx gorums.ServerCtx, req *Request, broadcast *Broadcast) {
	//srv.mu.Lock()
	//srv.numMsg["BC"]++
	//srv.mu.Unlock()
	//slog.Warn("server received broadcast call")
	// only broadcast to the leader
	broadcast.SendToClient(&Response{
		From: srv.addr,
	}, nil)
}

func (srv *testServer) Search(ctx gorums.ServerCtx, req *Request, broadcast *Broadcast) {
	select {
	case <-ctx.Done():
		//slog.Info("cancelled", "addr", srv.addr)
		broadcast.SendToClient(&Response{
			From:   srv.addr,
			Result: 0,
		}, nil)
	case <-time.After(srv.processingTime):
		//slog.Info("processed", "addr", srv.addr)
		broadcast.SendToClient(&Response{
			From:   srv.addr,
			Result: 1,
		}, nil)
	}
	broadcast.Cancel()
}

func (srv *testServer) LongRunningTask(ctx gorums.ServerCtx, req *Request, broadcast *Broadcast) {
	srv.mut.Lock()
	defer srv.mut.Unlock()
	select {
	case <-ctx.Done():
		srv.val = 1
	case <-time.After(2 * time.Second):
		srv.val = 0
	}
	broadcast.Done()
}

func (srv *testServer) GetVal(ctx gorums.ServerCtx, req *Request, broadcast *Broadcast) {
	srv.mut.Lock()
	defer srv.mut.Unlock()
	broadcast.SendToClient(&Response{
		From:   srv.addr,
		Result: srv.val,
	}, nil)
}

func (srv *testServer) GetMsgs() string {
	srv.mut.Lock()
	defer srv.mut.Unlock()
	res := "Srv " + srv.addr
	res += fmt.Sprintf(" -> QC: %d, QCB: %d, QCM: %d, M: %d, BC: %d, BI: %d, B: %d", srv.numMsg["QC"], srv.numMsg["QCB"], srv.numMsg["QCM"], srv.numMsg["M"], srv.numMsg["BC"], srv.numMsg["BI"], srv.numMsg["B"])
	return res
}

func (srv *testServer) GetNumMsgs() int {
	srv.mut.Lock()
	defer srv.mut.Unlock()
	return srv.numMsg["BC"] + srv.numMsg["B"] + srv.numMsg["BI"]
}
