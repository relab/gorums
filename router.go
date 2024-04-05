package gorums

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/relab/gorums/ordering"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type content interface {
	Validate() error
	getCtx() context.Context
	getFinished() chan<- *Message
	getMetadata() *ordering.Metadata
	isFromClient() bool
	//getMethod() string
	getClientHandlerName() string
	getOriginAddr() string
	getOriginMethod() string
	shouldWaitForClient() bool
	canBeRouted() bool
	isDone() bool
	update(content) error
	getDur() time.Duration
	getProcessingTime() time.Duration
	getResponse() *reply
	setResponse(*reply)
}

type responseData struct {
	mut       sync.Mutex
	messageID uint64
	method    string
	finished  chan<- *Message
	response  *reply
}

func (r *responseData) getMessageID() uint64 {
	r.mut.Lock()
	defer r.mut.Unlock()
	return r.messageID
}

func (r *responseData) getMethod() string {
	r.mut.Lock()
	defer r.mut.Unlock()
	return r.method
}

func (r *responseData) getResponse() *reply {
	r.mut.Lock()
	defer r.mut.Unlock()
	return r.response
}

func (r *responseData) getFinished() chan<- *Message {
	r.mut.Lock()
	defer r.mut.Unlock()
	return r.finished
}

func (r *responseData) addMessageID(messageID uint64) {
	r.mut.Lock()
	defer r.mut.Unlock()
	r.messageID = messageID
}

func (r *responseData) addResponse(resp *reply) {
	r.mut.Lock()
	defer r.mut.Unlock()
	r.response = resp
}

func (r *responseData) addFinished(finished chan<- *Message) {
	r.mut.Lock()
	defer r.mut.Unlock()
	r.finished = finished
}

type reqContent struct {
	ctx          context.Context
	cancelFunc   context.CancelFunc
	client       *responseData
	senderType   string
	originAddr   string
	originMethod string
	methods      []string
	timestamp    time.Time
	reqTS        time.Time
	doneChan     chan struct{}
	sent         bool
}

func (c reqContent) Validate() error {
	if c.senderType != BroadcastClient && c.senderType != BroadcastServer {
		return errors.New(fmt.Sprintf("senderType must be either %s or %s", BroadcastServer, BroadcastClient))
	}
	return nil
}

func (c reqContent) getCtx() context.Context {
	return c.ctx
}

func (c reqContent) getFinished() chan<- *Message {
	if c.client == nil {
		return nil
	}
	return c.client.finished
}

func (c reqContent) getMetadata() *ordering.Metadata {
	if c.client == nil {
		return nil
	}
	return &ordering.Metadata{
		MessageID: c.client.getMessageID(),
		Method:    c.client.getMethod(),
	}
}

func (c reqContent) isFromClient() bool {
	return c.senderType == BroadcastClient
}

func (c reqContent) getClientHandlerName() string {
	return c.originMethod
}

func (c reqContent) getOriginAddr() string {
	return c.originAddr
}

func (c reqContent) getOriginMethod() string {
	return c.originMethod
}

func (c reqContent) shouldWaitForClient() bool {
	return c.sent
}

func (c reqContent) canBeRouted() bool {
	return c.sent && c.senderType == BroadcastClient
}

func (c reqContent) getDur() time.Duration {
	return time.Since(c.timestamp)
}

func (c reqContent) getProcessingTime() time.Duration {
	return time.Since(c.reqTS)
}

func (c reqContent) isDone() bool {
	select {
	case <-c.ctx.Done():
		return true
	default:
	}
	return false
}

func (c reqContent) update(new content) error {
	if c.ctx.Err() != nil {
		return c.ctx.Err()
	}
	newReq, ok := new.(reqContent)
	if !ok {
		return errors.New("could not convert to correct type")
	}
	c.reqTS = time.Now()
	if c.originAddr == "" && newReq.originAddr != "" {
		c.originAddr = newReq.originAddr
	}
	if c.originMethod == "" && newReq.originMethod != "" {
		c.originMethod = newReq.originMethod
	}
	if c.senderType == BroadcastServer && newReq.senderType == BroadcastClient {
		c.senderType = BroadcastClient
		if newReq.client != nil && newReq.client.getFinished() != nil {
			var resp *reply
			// check if a response has been added.
			// can happen if the server has executed
			// SendToClient before receiving the client
			// request directly from the client.
			if c.client != nil {
				resp = c.client.getResponse()
			}
			// add the response to new if resp is non-nil
			// and update the current data
			if resp != nil {
				newReq.client.addResponse(resp)
			}
			c.client = newReq.client
		}
	}
	return nil
}

func (c reqContent) getResponse() *reply {
	if c.client != nil {
		return c.client.getResponse()
	}
	return nil
}

func (c reqContent) setResponse(response *reply) {
	if c.client == nil {
		c.client = &responseData{}
	}
	c.client.addResponse(response)
}

type BroadcastRouter struct {
	id             uint32
	addr           string
	serverHandlers map[string]serverHandler // handlers on other servers
	clientHandlers map[string]clientHandler // handlers on client servers
	doneChan       chan struct{}
}

func newBroadcastRouter() *BroadcastRouter {
	return &BroadcastRouter{
		serverHandlers: make(map[string]serverHandler),
		clientHandlers: make(map[string]clientHandler),
		doneChan:       make(chan struct{}),
	}
}

func (r *BroadcastRouter) addAddr(id uint32, addr string) {
	r.id = id
	r.addr = addr
}

func (r *BroadcastRouter) addServerHandler(method string, handler serverHandler) {
	r.serverHandlers[method] = handler
}

func (r *BroadcastRouter) addClientHandler(method string, handler clientHandler) {
	r.clientHandlers[method] = handler
}

func (r *BroadcastRouter) send(broadcastID string, data content, req any) error {
	switch val := req.(type) {
	case *reply:
		return r.routeClientReply(broadcastID, data, val)
	case *broadcastMsg:
		return r.routeBroadcast(broadcastID, data, val)
	}
	return errors.New("wrong req type")
}

func (r *BroadcastRouter) routeBroadcast(broadcastID string, data content, msg *broadcastMsg) error {
	if handler, ok := r.serverHandlers[msg.method]; ok {
		// it runs an interceptor prior to broadcastCall, hence a different signature.
		// see (srv *broadcastServer) registerBroadcastFunc(method string).
		handler(msg.ctx, msg.request, broadcastID, data.getOriginAddr(), data.getOriginMethod(), msg.options, r.id, r.addr)
		slog.Debug("broadcast took", "time", data.getProcessingTime())
		return nil
	}
	return errors.New("not found")
}

func (r *BroadcastRouter) routeClientReply(broadcastID string, data content, resp *reply) error {
	// the client has initiated a broadcast call and the reply should be sent as an RPC
	if handler, ok := r.clientHandlers[data.getClientHandlerName()]; ok && data.getOriginAddr() != "" {
		handler(data.getOriginAddr(), broadcastID, resp.getResponse())
		slog.Debug("return took", "time", data.getProcessingTime())
		return nil
	}
	// there is a direct connection to the client, e.g. from a QuorumCall
	if data.isFromClient() {
		msg := WrapMessage(data.getMetadata(), protoreflect.ProtoMessage(resp.getResponse()), resp.getError())
		SendMessage(data.getCtx(), data.getFinished(), msg)
		slog.Debug("return took", "time", data.getProcessingTime())
		return nil
	}
	// the server can receive a broadcast from another server before a client sends a direct message.
	// it should thus wait for a potential message from the client. otherwise, it should be removed.
	return errors.New("not routed")
}

type BroadcastState struct {
	mut      sync.RWMutex
	msgs     map[string]content
	doneChan chan struct{}
}

func newBroadcastStorage() *BroadcastState {
	return &BroadcastState{
		msgs:     make(map[string]content),
		doneChan: make(chan struct{}),
	}
}

func (s *BroadcastState) newData(ctx context.Context, senderType, originAddr, originMethod string, messageID uint64, method string, finished chan<- *Message) (content, error) {
	var client *responseData
	if senderType == BroadcastClient {
		client = &responseData{
			messageID: messageID,
			method:    method,
			finished:  finished,
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	data := reqContent{
		ctx:          ctx,
		cancelFunc:   cancel,
		senderType:   senderType,
		originAddr:   originAddr,
		originMethod: originMethod,
		client:       client,
		timestamp:    time.Now(),
		reqTS:        time.Now(),
		sent:         false,
	}
	return data, nil
}

func (s *BroadcastState) addOrUpdate(broadcastID string, msg content) error {
	s.mut.Lock()
	defer s.mut.Unlock()
	defer slog.Debug("added broadcast", "broadcastID", broadcastID)
	if old, ok := s.msgs[broadcastID]; ok {
		// update old with new
		err := old.update(msg)
		if err != nil {
			return err
		}
		// data is not a pointer and must thus be saved
		s.msgs[broadcastID] = old
		return nil
	}
	s.msgs[broadcastID] = msg
	return nil
}

func (s *BroadcastState) add(broadcastID string, msg content) error {
	s.mut.Lock()
	defer s.mut.Unlock()
	if _, ok := s.msgs[broadcastID]; ok {
		return errors.New("already added")
	}
	s.msgs[broadcastID] = msg
	return nil
}

func (s *BroadcastState) update(broadcastID string, new content) error {
	s.mut.Lock()
	defer s.mut.Unlock()
	if old, ok := s.msgs[broadcastID]; ok {
		// update old with new
		err := old.update(new)
		if err != nil {
			return err
		}
		// data is not a pointer and must thus be saved
		s.msgs[broadcastID] = old
		return nil
	}
	return errors.New("not found")
}

func (s *BroadcastState) isValid(broadcastID string) bool {
	s.mut.RLock()
	defer s.mut.RUnlock()
	msg, ok := s.msgs[broadcastID]
	if !ok {
		return false
	}
	// the request is done if the server has replied to the client
	return !msg.shouldWaitForClient()
}

func (s *BroadcastState) get(broadcastID string) (content, error) {
	s.mut.RLock()
	defer s.mut.RUnlock()
	if msg, ok := s.msgs[broadcastID]; ok {
		if msg.shouldWaitForClient() {
			return nil, errors.New("not valid")
		}
		return msg, nil
	}
	return nil, errors.New("not found")
}

func (s *BroadcastState) remove(broadcastID string) error {
	s.mut.Lock()
	defer s.mut.Unlock()
	_, ok := s.msgs[broadcastID]
	if !ok {
		return errors.New("not found")
	}
	//if msg.getDur() > 15*time.Millisecond {
	//slog.Info("duration", "time", msg.getDur())
	//}
	delete(s.msgs, broadcastID)
	return nil
}

func (s *BroadcastState) setShouldWaitForClient(broadcastID string, response *reply) {
	s.mut.Lock()
	defer s.mut.Unlock()
	msg, ok := s.msgs[broadcastID]
	if !ok {
		return
	}
	msg.setResponse(response)
}

func (s *BroadcastState) prune() error {
	s.msgs = make(map[string]content)
	s.doneChan = make(chan struct{})
	return nil
}
