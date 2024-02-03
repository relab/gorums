package gorums

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/relab/gorums/ordering"
	"google.golang.org/grpc/peer"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type broadcastServer struct {
	sync.RWMutex
	recievedFrom           map[uint64]map[string]map[string]bool
	broadcastedMsgs        map[string]map[string]bool
	returnedToClientMsgs   map[string]bool
	BroadcastChan          chan broadcastMsg
	methods                map[string]broadcastFunc
	responseChan           chan responseMsg
	mutex                  sync.RWMutex
	b                      broadcastStruct
	pendingClientResponses map[string]respType
	timeout                time.Duration
	clientReqs             map[string]*clientRequest
	config                 RawConfiguration
	middlewares            []func()
}

func newBroadcastServer() *broadcastServer {
	return &broadcastServer{
		recievedFrom:           make(map[uint64]map[string]map[string]bool),
		broadcastedMsgs:        make(map[string]map[string]bool),
		returnedToClientMsgs:   make(map[string]bool),
		BroadcastChan:          make(chan broadcastMsg, 1000),
		methods:                make(map[string]broadcastFunc),
		responseChan:           make(chan responseMsg),
		pendingClientResponses: make(map[string]respType),
		timeout:                5 * time.Second,
		clientReqs:             make(map[string]*clientRequest),
	}
}

func (srv *broadcastServer) setPendingClientResponse(broadcastID string) {
	srv.mutex.Lock()
	defer srv.mutex.Unlock()
	srv.pendingClientResponses[broadcastID] = unhandled
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

func (srv *broadcastServer) broadcast(broadcastMessage broadcastMsg) {
	//time.Sleep(5 * time.Second)
	// drop if ctx is cancelled? Or in run method?
	srv.BroadcastChan <- broadcastMessage
}

func (srv *broadcastServer) run() {
	for msg := range srv.BroadcastChan {
		//*srv.BroadcastID = msg.GetBroadcastID()
		//srv.c.StoreID(msgID-1)
		req := msg.getRequest()
		method := msg.getMethod()
		broadcastID := msg.getBroadcastID()
		ctx := context.Background()
		// reqCtx := msg.GetContext()
		// drop if ctx is cancelled? Or in broadcast method?
		// if another function is called in broadcast, the request needs to be converted
		//if convertFunc, ok := srv.conversions[method]; ok {
		//	convertedReq := convertFunc(ctx, req)
		//	srv.methods[method](ctx, convertedReq)
		//	continue
		//}
		srv.methods[method](ctx, req, broadcastID)
	}
}

func (srv *broadcastServer) handleClientResponses() {
	for response := range srv.responseChan {
		req, ok := srv.clientReqs[response.getBroadcastID()]
		if !ok {
			// this server has not received a request directly from a client
			// hence, the response should be ignored
			continue
		} else if req.status == unhandled {
			// first time it is handled
			req.status = response.getType()
		} else if req.status != done {
			// already handled, but got the other response type in the pair: clientResponse & timeout
			req.status = done
		} else {
			// already handled and can be removed
			continue
		}
		if !response.valid() {
			// the response is old and should have timed out, but may not due to scheduling.
			// the timeout msg should arrive soon.
			continue
		}
		SendMessage(req.ctx, req.finished, WrapMessage(req.metadata, protoreflect.ProtoMessage(response.getResponse()), response.getError()))
	}
}

func (srv *broadcastServer) runMiddleware() {
	for _, middleware := range srv.middlewares {
		middleware()
	}
}

func (srv *broadcastServer) timeoutClientResponse(ctx ServerCtx, in *Message, finished chan<- *Message) {
	time.After(srv.timeout)
	srv.responseChan <- newResponseMessage(protoreflect.ProtoMessage(nil), errors.New("server timed out"), in.Metadata.BroadcastMsg.GetBroadcastID(), timeout, srv.timeout)
}

func (srv *broadcastServer) alreadyReturnedToClient(broadcastID string, method string) bool {
	srv.mutex.Lock()
	defer srv.mutex.Unlock()
	returned, ok := srv.returnedToClientMsgs[broadcastID]
	if !ok {
		return true
	}
	return ok && returned
}

func (srv *broadcastServer) setReturnedToClient(broadcastID string, val bool) {
	srv.mutex.Lock()
	defer srv.mutex.Unlock()
	srv.returnedToClientMsgs[broadcastID] = val
}

func (srv *broadcastServer) addClientRequest(metadata *ordering.Metadata, ctx ServerCtx, finished chan<- *Message) {
	srv.setPendingClientResponse(metadata.BroadcastMsg.GetBroadcastID())
	srv.setReturnedToClient(metadata.BroadcastMsg.GetBroadcastID(), false)
	srv.mutex.Lock()
	defer srv.mutex.Unlock()
	srv.clientReqs[metadata.BroadcastMsg.GetBroadcastID()] = &clientRequest{
		id:       uuid.New().String(),
		ctx:      ctx,
		finished: finished,
		metadata: metadata,
		status:   unhandled,
	}
}

// QuorumCallData holds the message, destination nodes, method identifier,
// and other information necessary to perform the various quorum call types
// supported by Gorums.
//
// This struct should be used by generated code only.
type BroadcastCallData struct {
	Message     protoreflect.ProtoMessage
	Method      string
	Sender      string
	BroadcastID string // a unique identifier for the current message
}

// BroadcastCall performs a multicast call on the configuration.
//
// This method should be used by generated code only.
func (c RawConfiguration) BroadcastCall(ctx context.Context, d BroadcastCallData) {
	md := &ordering.Metadata{MessageID: c.getMsgID(), Method: d.Method, BroadcastMsg: &ordering.BroadcastMsg{
		Sender: d.Sender, BroadcastID: d.BroadcastID,
	}}

	for _, n := range c {
		msg := d.Message
		n.channel.enqueue(request{ctx: ctx, msg: &Message{Metadata: md, Message: msg}}, nil, false)
	}
}

type broadcastMsg interface {
	getFrom() string
	getRequest() requestTypes
	getMethod() string
	getContext() ServerCtx
	getBroadcastID() string
}

type broadcastMessage struct {
	from        string
	request     requestTypes
	method      string
	context     ServerCtx
	broadcastID string
}

func (b *broadcastMessage) getFrom() string {
	return b.from
}

func (b *broadcastMessage) getRequest() requestTypes {
	return b.request
}

func (b *broadcastMessage) getMethod() string {
	return b.method
}

func (b *broadcastMessage) getContext() ServerCtx {
	return b.context
}

func (b *broadcastMessage) getBroadcastID() string {
	return b.broadcastID
}

func newBroadcastMessage(ctx ServerCtx, req requestTypes, method string, broadcastID string) *broadcastMessage {
	p, _ := peer.FromContext(ctx)
	addr := p.Addr.String()
	return &broadcastMessage{
		from:        addr,
		request:     req,
		method:      method,
		context:     ctx,
		broadcastID: broadcastID,
	}
}
