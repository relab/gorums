package gorums

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/relab/gorums/ordering"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type broadcastServer struct {
	sync.RWMutex
	id              string
	addr            string
	broadcastedMsgs map[string]map[string]bool
	methods         map[string]broadcastFunc
	broadcastChan   chan *broadcastMsg
	responseChan    chan responseMsg
	clientHandlers  map[string]func(addr, broadcastID string, req protoreflect.ProtoMessage, opts ...grpc.CallOption) (any, error)
	bNew            iBroadcastStruct
	timeout         time.Duration
	clientReqs      *RequestMap
	view            serverView
	middlewares     []func(BroadcastMetadata) error
	stopChan        chan struct{}
	//mutex          sync.RWMutex
	//returnedToClientMsgs map[string]bool
	//clientReqs      map[string]*clientRequest
	//clientReqsMutex sync.Mutex
}

func newBroadcastServer() *broadcastServer {
	return &broadcastServer{
		id:              uuid.New().String(),
		broadcastedMsgs: make(map[string]map[string]bool),
		clientHandlers:  make(map[string]func(addr, broadcastID string, req protoreflect.ProtoMessage, opts ...grpc.CallOption) (any, error)),
		broadcastChan:   make(chan *broadcastMsg, 1000),
		methods:         make(map[string]broadcastFunc),
		responseChan:    make(chan responseMsg),
		clientReqs:      NewRequestMap(),
		middlewares:     make([]func(BroadcastMetadata) error, 0),
		stopChan:        make(chan struct{}, 0),
		//returnedToClientMsgs: make(map[string]bool),
		//clientReqs:    make(map[string]*clientRequest),
	}
}

func (srv *broadcastServer) alreadyBroadcasted(broadcastID string, method string) bool {
	_, ok := srv.broadcastedMsgs[broadcastID]
	if !ok {
		srv.broadcastedMsgs[broadcastID] = make(map[string]bool)
	}
	broadcasted, ok := srv.broadcastedMsgs[broadcastID][method]
	if !ok {
		srv.broadcastedMsgs[broadcastID][method] = true
	}
	return ok && broadcasted
}

func (srv *broadcastServer) run() {
	for msg := range srv.broadcastChan {
		if handler, ok := srv.methods[msg.method]; ok {
			handler(msg.ctx, msg.request, msg.metadata, msg.srvAddrs)
		}
		msg.setFinished()
	}
}

func (srv *broadcastServer) handleClientResponses() {
	for response := range srv.responseChan {
		srv.handle(response)
	}
}

func (srv *broadcastServer) handle(response responseMsg) {
	broadcastID := response.getBroadcastID()
	req, handled := srv.clientReqs.GetSet(broadcastID)
	if handled {
		// this server has not received a request directly from a client
		// hence, the response should be ignored
		// already handled and can not be removed yet. It is possible to get duplicates.
		return
	}
	select {
	case <-req.ctx.Done():
		// client request has been cancelled by client
		log.Println("CLIENT REQUEST HAS BEEN CANCELLED")
		return
	default:
	}
	if !response.valid() {
		// the response is old and should have timed out, but may not due to scheduling.
		// the timeout msg should arrive soon.
		log.Println("NOT VALID")
		return
	}
	if handler, ok := srv.clientHandlers[req.metadata.BroadcastMsg.OriginMethod]; ok && req.metadata.BroadcastMsg.OriginAddr != "" {
		handler(req.metadata.BroadcastMsg.OriginAddr, broadcastID, response.getResponse())
	} else if req.metadata.BroadcastMsg.Sender == BroadcastClient {
		SendMessage(req.ctx, req.finished, WrapMessage(req.metadata, protoreflect.ProtoMessage(response.getResponse()), response.getError()))
	}
}

func (srv *broadcastServer) runMiddleware(metadata BroadcastMetadata) error {
	for _, middleware := range srv.middlewares {
		err := middleware(metadata)
		if err != nil {
			return err
		}
	}
	return nil
}

func (srv *broadcastServer) clientReturn(resp ResponseTypes, err error, metadata BroadcastMetadata) {
	srv.returnToClient(metadata.BroadcastID, resp, err)
}

func (srv *broadcastServer) returnToClient(broadcastID string, resp ResponseTypes, err error) {
	srv.Lock()
	defer srv.Unlock()
	if !srv.alreadyReturnedToClient(broadcastID) {
		srv.responseChan <- newResponseMessage(resp, err, broadcastID, clientResponse, srv.timeout)
	}
}

func (srv *broadcastServer) timeoutClientResponse(ctx ServerCtx, in *Message, finished chan<- *Message) {
	time.Sleep(srv.timeout)
	srv.responseChan <- newResponseMessage(protoreflect.ProtoMessage(nil), errors.New("server timed out"), in.Metadata.BroadcastMsg.GetBroadcastID(), timeout, srv.timeout)
}

func (srv *broadcastServer) alreadyReturnedToClient(broadcastID string) bool {
	req, ok := srv.clientReqs.Get(broadcastID)
	return ok && req.handled
}

func (srv *broadcastServer) addClientRequest(metadata *ordering.Metadata, ctx ServerCtx, finished chan<- *Message) {
	srv.clientReqs.Add(metadata.BroadcastMsg.GetBroadcastID(), clientRequest{
		id:       uuid.New().String(),
		ctx:      ctx,
		finished: finished,
		metadata: metadata,
		status:   unhandled,
		handled:  false,
	})
}

// broadcastCallData holds the message, destination nodes, method identifier,
// and other information necessary to perform the various quorum call types
// supported by Gorums.
//
// This struct should be used by generated code only.
type broadcastCallData struct {
	Message         protoreflect.ProtoMessage
	Method          string
	BroadcastID     string // a unique identifier for the current broadcast request
	Sender          string
	SenderID        string
	SenderAddr      string
	OriginID        string
	OriginAddr      string
	OriginMethod    string
	ServerAddresses []string
}

func (bcd *broadcastCallData) inServerAddresses(addr string) bool {
	if len(bcd.ServerAddresses) <= 0 {
		return true
	}
	for _, srvAddr := range bcd.ServerAddresses {
		if addr == srvAddr {
			return true
		}
	}
	return false
}

// broadcastCall performs a multicast call on the configuration.
//
// This method should be used by generated code only.
func (c RawConfiguration) broadcastCall(ctx context.Context, d broadcastCallData) {
	md := &ordering.Metadata{MessageID: c.getMsgID(), Method: d.Method, BroadcastMsg: &ordering.BroadcastMsg{
		Sender:       d.Sender,
		BroadcastID:  d.BroadcastID,
		SenderID:     d.SenderID,
		SenderAddr:   d.SenderAddr,
		OriginID:     d.OriginID,
		OriginAddr:   d.OriginAddr,
		OriginMethod: d.OriginMethod,
	}}
	o := getCallOptions(E_Broadcast, nil)

	replyChan := make(chan response, len(c))
	sentMsgs := 0
	for _, n := range c {
		if !d.inServerAddresses(n.addr) {
			continue
		}
		if !n.connected {
			if n.connect(n.mgr) != nil {
				continue
			} else {
				n.connected = true
			}
		}
		sentMsgs++
		msg := d.Message
		go n.channel.enqueue(request{ctx: ctx, msg: &Message{Metadata: md, Message: msg}, opts: o}, replyChan, false)
	}

	// wait until all have requests have been sent
	for sentMsgs > 0 {
		<-replyChan
		sentMsgs--
	}
}

type broadcastMsg struct {
	from        string
	request     RequestTypes
	method      string
	metadata    BroadcastMetadata
	broadcastID string
	srvAddrs    []string
	finished    chan<- struct{}
	ctx         context.Context
}

func (b *broadcastMsg) setFinished() {
	close(b.finished)
}

func newBroadcastMessage(metadata BroadcastMetadata, req RequestTypes, method, broadcastID string, srvAddrs []string, finished chan<- struct{}) *broadcastMsg {
	return &broadcastMsg{
		request:     req,
		method:      method,
		metadata:    metadata,
		broadcastID: broadcastID,
		srvAddrs:    srvAddrs,
		finished:    finished,
		ctx:         context.WithValue(context.Background(), BroadcastID, metadata.BroadcastID),
	}
}
