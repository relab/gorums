package gorums

import (
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/relab/gorums/ordering"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type broadcastServer struct {
	id              string
	addr            string
	peers           []string
	view            serverView
	mgr             *RawManager
	broadcastedMsgs map[string]map[string]bool
	handlers        map[string]broadcastFunc
	broadcastChan   chan *broadcastMsg
	responseChan    chan *responseMsg
	clientHandlers  map[string]func(addr, broadcastID string, req protoreflect.ProtoMessage, opts ...grpc.CallOption) (any, error)
	broadcaster     broadcaster
	timeout         time.Duration
	clientReqs      *RequestMap
	stopChan        chan struct{}
	async           bool
	logger          *slog.Logger
}

func newBroadcastServer(logger *slog.Logger) *broadcastServer {
	return &broadcastServer{
		id:              uuid.New().String(),
		peers:           make([]string, 0),
		broadcastedMsgs: make(map[string]map[string]bool),
		clientHandlers:  make(map[string]func(addr, broadcastID string, req protoreflect.ProtoMessage, opts ...grpc.CallOption) (any, error)),
		broadcastChan:   make(chan *broadcastMsg, 1000),
		handlers:        make(map[string]broadcastFunc),
		responseChan:    make(chan *responseMsg),
		clientReqs:      NewRequestMap(),
		stopChan:        make(chan struct{}, 0),
		logger:          logger,
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
		if handler, ok := srv.handlers[msg.method]; ok {
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

func (srv *broadcastServer) handle(response *responseMsg) {
	// REEVALUATE THIS:
	// If it is used in gossiping, then ok.
	// Otherwise, not ok. (e.g. when timeout)
	if !response.valid() {
		// the response is old and should have timed out, but may not due to scheduling.
		// the timeout msg should arrive soon.
		return
	}
	broadcastID := response.getBroadcastID()
	req, handled := srv.clientReqs.GetAndSetHandled(broadcastID)
	if handled {
		// this server has not received a request directly from a client
		// hence, the response should be ignored
		// already handled and can not be removed yet. It is possible to get duplicates.
		return
	}
	//select {
	//case <-req.ctx.Done():
	//	// client request has been cancelled by client
	//	log.Println("CLIENT REQUEST HAS BEEN CANCELLED")
	//	return
	//default:
	//}
	if handler, ok := srv.clientHandlers[req.metadata.BroadcastMsg.OriginMethod]; ok && req.metadata.BroadcastMsg.OriginAddr != "" {
		handler(req.metadata.BroadcastMsg.OriginAddr, broadcastID, response.getResponse())
	} else if req.metadata.BroadcastMsg.Sender == BroadcastClient {
		SendMessage(req.ctx, req.finished, WrapMessage(req.metadata, protoreflect.ProtoMessage(response.getResponse()), response.getError()))
	}
}

func (srv *broadcastServer) clientReturn(resp ResponseTypes, err error, metadata BroadcastMetadata) {
	srv.returnToClient(metadata.BroadcastID, resp, err)
}

func (srv *broadcastServer) returnToClient(broadcastID string, resp ResponseTypes, err error) {
	if !srv.alreadyReturnedToClient(broadcastID) {
		srv.responseChan <- newResponseMessage(resp, err, broadcastID, clientResponse, srv.timeout)
	}
}

//func (srv *broadcastServer) timeoutClientResponse(ctx ServerCtx, in *Message, finished chan<- *Message) {
//	time.Sleep(srv.timeout)
//	srv.responseChan <- newResponseMessage(protoreflect.ProtoMessage(nil), errors.New("server timed out"), in.Metadata.BroadcastMsg.GetBroadcastID(), timeout, srv.timeout)
//}

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
