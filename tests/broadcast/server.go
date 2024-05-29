package broadcast

import (
	"context"
	"errors"
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
	err            error
	order          []string
}

func newtestServer(addr string, srvAddresses []string, _ int, withOrder ...bool) *testServer {
	var osrv *Server
	if len(withOrder) > 0 {
		osrv = NewServer(gorums.WithOrder(BroadcastServicePrePrepare, BroadcastServicePrepare, BroadcastServiceCommit))
	} else {
		osrv = NewServer()
	}
	srv := testServer{
		Server:   osrv,
		numMsg:   map[string]int{"BC": 0, "QC": 0, "QCB": 0, "QCM": 0, "M": 0, "BI": 0, "B": 0},
		respChan: make(map[int64]response),
		leader:   leader,
		order:    make([]string, 0),
	}
	RegisterBroadcastServiceServer(srv.Server, &srv)
	srv.peers = srvAddresses
	srv.addr = addr
	if addr != leader {
		srv.processingTime = 100 * time.Millisecond
	}
	srv.mgr = NewManager(
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
	broadcast.SendToClient(&Response{
		From: srv.addr,
	}, nil)
}

func (srv *testServer) Search(ctx gorums.ServerCtx, req *Request, broadcast *Broadcast) {
	// make sure the client req reaches all servers first.
	// this is because the cancellation only give weak
	// guarantees. Meaning, cancellations not yet related
	// to a broadcast request (e.g. because the client req has
	// not yet arrived) will be dropped.
	time.Sleep(1 * time.Millisecond)
	select {
	case <-ctx.Done():
		broadcast.SendToClient(&Response{
			From:   srv.addr,
			Result: 0,
		}, nil)
	case <-time.After(srv.processingTime):
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
	case <-time.After(5 * time.Second):
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

func (srv *testServer) Order(ctx gorums.ServerCtx, req *Request, broadcast *Broadcast) {
	broadcast.PrePrepare(&Request{})
}

func (srv *testServer) PrePrepare(ctx gorums.ServerCtx, req *Request, broadcast *Broadcast) {
	// this will cause the leader to be late to call broadcast.Prepare().
	// Hence, it will receive Prepare and Commit from the other servers
	// before calling Prepare. The order of received msgs will thus be
	// wrong and the msgs need to be stored temporarily.
	if srv.addr == srv.leader {
		time.Sleep(200 * time.Millisecond)
	}
	srv.mut.Lock()
	added := false
	for _, m := range srv.order {
		if m == "PrePrepare" {
			added = true
			break
		}
	}
	if !added {
		srv.order = append(srv.order, "PrePrepare")
	}
	srv.mut.Unlock()
	broadcast.Prepare(&Request{})
}

func (srv *testServer) Prepare(ctx gorums.ServerCtx, req *Request, broadcast *Broadcast) {
	srv.mut.Lock()
	if len(srv.order) <= 0 {
		broadcast.SendToClient(&Response{
			From:   srv.addr,
			Result: 1,
		}, errors.New("did not receive PrePrepare before Prepare"))
		srv.mut.Unlock()
		return
	}
	added := false
	for _, m := range srv.order {
		if m == "Prepare" {
			added = true
			break
		}
	}
	if !added {
		srv.order = append(srv.order, "Prepare")
	}
	srv.mut.Unlock()
	broadcast.Commit(&Request{})
}

func (srv *testServer) Commit(ctx gorums.ServerCtx, req *Request, broadcast *Broadcast) {
	srv.mut.Lock()
	if len(srv.order) <= 0 {
		broadcast.SendToClient(&Response{
			From:   srv.addr,
			Result: 2,
		}, errors.New("did not receive PrePrepare and Prepare before Commit"))
		srv.mut.Unlock()
		return
	}
	if len(srv.order) <= 1 {
		broadcast.SendToClient(&Response{
			From:   srv.addr,
			Result: 3,
		}, errors.New("did not receive Prepare before Commit"))
		srv.mut.Unlock()
		return
	}
	srv.mut.Unlock()
	broadcast.SendToClient(&Response{
		From:   srv.addr,
		Result: 0,
	}, nil)
}
