package gorums

import (
	"log/slog"
	"net"
	"sync"

	"github.com/google/uuid"
	"github.com/relab/gorums/ordering"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type broadcastServer struct {
	propertiesMutex sync.Mutex
	viewMutex       sync.RWMutex
	id              string
	addr            string
	view            RawConfiguration
	broadcastedMsgs map[string]map[string]bool
	handlers        map[string]broadcastFunc
	broadcastChan   chan *broadcastMsg
	responseChan    chan *responseMsg
	clientHandlers  map[string]func(addr, broadcastID string, req protoreflect.ProtoMessage, opts ...grpc.CallOption) (any, error)
	//broadcaster     Ibroadcaster
	broadcaster  func(m BroadcastMetadata, o *BroadcastOrchestrator) Ibroadcaster
	orchestrator *BroadcastOrchestrator
	clientReqs   *RequestMap
	stopChan     chan struct{}
	logger       *slog.Logger
	started      bool
}

func newBroadcastServer(logger *slog.Logger) *broadcastServer {
	return &broadcastServer{
		id:              uuid.New().String(),
		broadcastedMsgs: make(map[string]map[string]bool),
		clientHandlers:  make(map[string]func(addr, broadcastID string, req protoreflect.ProtoMessage, opts ...grpc.CallOption) (any, error)),
		broadcastChan:   make(chan *broadcastMsg, 1000),
		handlers:        make(map[string]broadcastFunc),
		responseChan:    make(chan *responseMsg),
		clientReqs:      NewRequestMap(),
		stopChan:        nil,
		logger:          logger,
		started:         false,
	}
}

func (srv *broadcastServer) start() {
	srv.propertiesMutex.Lock()
	defer srv.propertiesMutex.Unlock()
	// the goroutines should only be created if they don't exist yet.
	// it is possible to change view after the server has been started
	// and thus we need to check for this to prevent starting these
	// goroutines again.
	// Also create new stopchan if it does not already exist or if the
	// server has been stopped.
	if !srv.started {
		srv.stopChan = make(chan struct{})
		go srv.run()
		go srv.handleClientResponses()
	}
	srv.started = true
}

func (srv *broadcastServer) stop() {
	srv.propertiesMutex.Lock()
	defer srv.propertiesMutex.Unlock()
	if srv.stopChan != nil {
		close(srv.stopChan)
	}
	srv.started = false
}

func (srv *broadcastServer) addAddr(lis net.Listener) {
	srv.propertiesMutex.Lock()
	defer srv.propertiesMutex.Unlock()
	srv.addr = lis.Addr().String()
}

func (srv *broadcastServer) alreadyBroadcasted(broadcastID string, method string) bool {
	srv.propertiesMutex.Lock()
	defer srv.propertiesMutex.Unlock()
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
	for {
		select {
		case msg := <-srv.broadcastChan:
			go srv.handleBroadcast(msg)
		case <-srv.stopChan:
			return
		}
	}
}

func (srv *broadcastServer) handleBroadcast(msg *broadcastMsg) {
	// set the message as handled when returning from the method
	defer msg.setFinished()
	// lock to prevent view change mid execution of a broadcast request
	srv.viewMutex.RLock()
	if broadcastCall, ok := srv.handlers[msg.method]; ok {
		// ignore if the client request is no longer valid
		req, ok := srv.clientReqs.Get(msg.broadcastID)
		if !ok {
			return
		}
		// it runs an interceptor prior to broadcastCall, hence a different signature.
		// see (srv *broadcastServer) registerBroadcastFunc(method string).
		broadcastCall(msg.ctx, msg.request, req, msg.options)
	}
	srv.viewMutex.RUnlock()
}

func (srv *broadcastServer) handleClientResponses() {
	go srv.clientReqs.cleanup()
	for {
		select {
		case response := <-srv.responseChan:
			srv.handle(response)
		case <-srv.stopChan:
			srv.clientReqs.Stop()
			return
		}
	}
}

func (srv *broadcastServer) handle(response *responseMsg) {
	/*if !response.valid() {
		// REEVALUATE THIS:
		// - A response message will always be valid because it is
		// 	 initiated by the implementer
		// - There is no utility for this method regarding gossiping.
		//   This method should be located in the broadcast function.
		return
	}*/
	broadcastID := response.getBroadcastID()
	req, valid := srv.clientReqs.GetIfValid(broadcastID)
	if !valid {
		// this server has not received a request directly from a client
		// hence, the response should be ignored
		// already handled and can not be removed yet. It is possible to get duplicates.
		return
	}
	// the request is handled and can thus be removed from
	// client requests when returning.
	defer srv.clientReqs.Remove(response.broadcastID)
	// there is no need to respond to the client if
	// the request has been cancelled
	select {
	case <-req.ctx.Done():
		// client request has been cancelled
		return
	default:
	}
	if handler, ok := srv.clientHandlers[req.metadata.BroadcastMsg.OriginMethod]; ok && req.metadata.BroadcastMsg.OriginAddr != "" {
		handler(req.metadata.BroadcastMsg.OriginAddr, broadcastID, response.getResponse())
	} else if req.metadata.BroadcastMsg.SenderType == BroadcastClient {
		SendMessage(req.ctx, req.finished, WrapMessage(req.metadata, protoreflect.ProtoMessage(response.getResponse()), response.getError()))
	}
}

func (srv *broadcastServer) sendToClient(broadcastID string, resp ResponseTypes, err error) {
	srv.responseChan <- newResponseMessage(resp, err, broadcastID)
}

func (srv *broadcastServer) addClientRequest(metadata *ordering.Metadata, ctx ServerCtx, finished chan<- *Message) (count uint64, err error) {
	return srv.clientReqs.Add(metadata.BroadcastMsg.GetBroadcastID(), clientRequest{
		id:       uuid.New().String(),
		ctx:      ctx,
		finished: finished,
		metadata: metadata,
		doneChan: make(chan struct{}),
	})
}
