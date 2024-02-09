package gorums

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/relab/gorums/ordering"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type broadcastServer struct {
	sync.RWMutex
	recievedFrom         map[uint64]map[string]map[string]bool
	broadcastedMsgs      map[string]map[string]bool
	returnedToClientMsgs map[string]bool
	BroadcastChan        chan broadcastMsg
	methods              map[string]broadcastFunc
	responseChan         chan responseMsg
	mutex                sync.RWMutex
	b                    broadcastStruct
	timeout              time.Duration
	clientReqs           map[string]*clientRequest
	clientReqsMutex      sync.Mutex
	config               RawConfiguration
	middlewares          []func(BroadcastCtx) error
	addr                 string
	id                   string
	publicKey            string
	privateKey           string
}

func newBroadcastServer() *broadcastServer {
	return &broadcastServer{
		id:                   uuid.New().String(),
		recievedFrom:         make(map[uint64]map[string]map[string]bool),
		broadcastedMsgs:      make(map[string]map[string]bool),
		returnedToClientMsgs: make(map[string]bool),
		BroadcastChan:        make(chan broadcastMsg, 1000),
		methods:              make(map[string]broadcastFunc),
		responseChan:         make(chan responseMsg),
		clientReqs:           make(map[string]*clientRequest),
		middlewares:          make([]func(BroadcastCtx) error, 0),
		publicKey:            "publicKey",
		privateKey:           "privateKey",
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
		srvAddrs := msg.getSrvAddrs()
		//broadcastID := msg.getBroadcastID()
		//ctx := context.Background()
		bCtx := msg.getContext()
		ctx := context.Background()
		// reqCtx := msg.GetContext()
		// drop if ctx is cancelled? Or in broadcast method?
		// if another function is called in broadcast, the request needs to be converted
		//if convertFunc, ok := srv.conversions[method]; ok {
		//	convertedReq := convertFunc(ctx, req)
		//	srv.methods[method](ctx, convertedReq)
		//	continue
		//}
		srv.methods[method](ctx, req, bCtx, srvAddrs)
	}
}

func (srv *broadcastServer) handleClientResponses() {
	for response := range srv.responseChan {
		srv.handle(response)
	}
}

func (srv *broadcastServer) handle(response responseMsg) {
	srv.clientReqsMutex.Lock()
	defer srv.clientReqsMutex.Unlock()
	req, ok := srv.clientReqs[response.getBroadcastID()]
	if !ok {
		// this server has not received a request directly from a client
		// hence, the response should be ignored
		return
	} else if req.status == unhandled {
		// first time it is handled
		req.status = response.getType()
	} else if req.status == clientResponse || req.status == timeout {
		// already handled, but got the other response type in the pair: clientResponse & timeout
		// or a duplicate
		req.status = done
		return
	} else {
		// already handled and can be removed
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
		return
	}
	SendMessage(req.ctx, req.finished, WrapMessage(req.metadata, protoreflect.ProtoMessage(response.getResponse()), response.getError()))
}

func (srv *broadcastServer) runMiddleware(ctx BroadcastCtx) error {
	for _, middleware := range srv.middlewares {
		err := middleware(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

func (srv *broadcastServer) timeoutClientResponse(ctx ServerCtx, in *Message, finished chan<- *Message) {
	time.Sleep(srv.timeout)
	srv.responseChan <- newResponseMessage(protoreflect.ProtoMessage(nil), errors.New("server timed out"), in.Metadata.BroadcastMsg.GetBroadcastID(), timeout, srv.timeout)
}

func (srv *broadcastServer) alreadyReturnedToClient(broadcastID string) bool {
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
	srv.setReturnedToClient(metadata.BroadcastMsg.GetBroadcastID(), false)
	srv.clientReqsMutex.Lock()
	defer srv.clientReqsMutex.Unlock()
	srv.clientReqs[metadata.BroadcastMsg.GetBroadcastID()] = &clientRequest{
		id:       uuid.New().String(),
		ctx:      ctx,
		finished: finished,
		metadata: metadata,
		status:   unhandled,
	}
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
	PublicKey       string
	Signature       string
	MAC             string
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
		Sender:      d.Sender,
		BroadcastID: d.BroadcastID,
		SenderID:    d.SenderID,
		SenderAddr:  d.SenderAddr,
		OriginID:    d.OriginID,
		OriginAddr:  d.OriginAddr,
		PublicKey:   d.PublicKey,
		Signature:   d.Signature,
		MAC:         d.MAC,
	}}

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
		msg := d.Message
		go n.channel.enqueue(request{ctx: ctx, msg: &Message{Metadata: md, Message: msg}}, nil, false)
	}
}

type broadcastMsg interface {
	getFrom() string
	getRequest() requestTypes
	getMethod() string
	getContext() BroadcastCtx
	getBroadcastID() string
	getSrvAddrs() []string
}

type broadcastMessage struct {
	from        string
	request     requestTypes
	method      string
	context     BroadcastCtx
	broadcastID string
	srvAddrs    []string
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

func (b *broadcastMessage) getContext() BroadcastCtx {
	return b.context
}

func (b *broadcastMessage) getBroadcastID() string {
	return b.broadcastID
}

func (b *broadcastMessage) getSrvAddrs() []string {
	return b.srvAddrs
}

func newBroadcastMessage(ctx BroadcastCtx, req requestTypes, method, broadcastID, from string, srvAddrs []string) *broadcastMessage {
	return &broadcastMessage{
		from:        from,
		request:     req,
		method:      method,
		context:     ctx,
		broadcastID: broadcastID,
		srvAddrs:    srvAddrs,
	}
}
