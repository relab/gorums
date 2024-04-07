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

//type content interface {
//	getCtx() context.Context
//	getFinished() chan<- *Message
//	getMetadata() *ordering.Metadata
//	isFromClient() bool
//	getClientHandlerName() string
//	getOriginAddr() string
//	getOriginMethod() string
//	shouldWaitForClient() bool
//	canBeRouted() bool
//	isDone() bool
//	update(content) error
//	getDur() time.Duration
//	getProcessingTime() time.Duration
//	getResponse() *reply
//	setResponse(*reply) error
//}

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

type content struct {
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

func (c *content) getCtx() context.Context {
	return c.ctx
}

func (c *content) getFinished() chan<- *Message {
	if c.client == nil {
		return nil
	}
	return c.client.finished
}

func (c *content) getMetadata() *ordering.Metadata {
	if c.client == nil {
		return nil
	}
	return &ordering.Metadata{
		MessageID: c.client.getMessageID(),
		Method:    c.client.getMethod(),
	}
}

func (c *content) isFromClient() bool {
	return c.senderType == BroadcastClient
}

func (c *content) getClientHandlerName() string {
	return c.originMethod
}

func (c *content) getOriginAddr() string {
	return c.originAddr
}

func (c *content) getOriginMethod() string {
	return c.originMethod
}

func (c *content) shouldWaitForClient() bool {
	return c.sent
}

func (c *content) canBeRouted() bool {
	return c.sent && c.senderType == BroadcastClient
}

func (c *content) getDur() time.Duration {
	return time.Since(c.timestamp)
}

func (c *content) getProcessingTime() time.Duration {
	return time.Since(c.reqTS)
}

func (c *content) isDone() bool {
	select {
	case <-c.ctx.Done():
		return true
	default:
	}
	return false
}

func (old *content) update(new *content) error {
	if old.ctx.Err() != nil {
		return old.ctx.Err()
	}
	//newReq, ok := new.(*reqContent)
	//if !ok {
	//	return errors.New("could not convert to correct type")
	//}
	old.reqTS = time.Now()
	if old.originAddr == "" && new.originAddr != "" {
		old.originAddr = new.originAddr
	}
	if old.originMethod == "" && new.originMethod != "" {
		old.originMethod = new.originMethod
	}
	if old.senderType == BroadcastServer && new.senderType == BroadcastClient {
		old.senderType = BroadcastClient
		if new.client != nil && new.client.getFinished() != nil {
			//slog.Info("client arrived later", "old", c.sent, "new", new.shouldWaitForClient())
			var resp *reply
			// check if a response has been added.
			// can happen if the server has executed
			// SendToClient before receiving the client
			// request directly from the client.
			if old.client != nil {
				resp = old.client.getResponse()
			}
			// add the response to new if resp is non-nil
			// and update the current data
			if resp != nil {
				new.client.addResponse(resp)
			}
			old.client = new.client
			//slog.Info("it happened", "sent", old.sent, "resp", resp.broadcastID, "canBeRouted", old.canBeRouted())
		}
	}
	return nil
}

func (c *content) getResponse() *reply {
	if c.client != nil {
		return c.client.getResponse()
	}
	return nil
}

func (c *content) setResponse(response *reply) error {
	if c.client == nil {
		c.client = &responseData{}
	}
	if c.client.response != nil {
		return errors.New("already added a response")
	}
	//slog.Info("setting response", "resp", response.response)
	c.client.addResponse(response)
	c.sent = true
	return nil
}

type BroadcastRouter struct {
	sendMutex      sync.Mutex
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
	msgs     map[string]*content
	doneChan chan struct{}
}

func newBroadcastStorage() *BroadcastState {
	return &BroadcastState{
		msgs:     make(map[string]*content),
		doneChan: make(chan struct{}),
	}
}

func (s *BroadcastState) newData(ctx context.Context, senderType, originAddr, originMethod string, messageID uint64, method string, finished chan<- *Message) (*content, error) {
	if senderType != BroadcastClient && senderType != BroadcastServer {
		return nil, errors.New(fmt.Sprintf("senderType must be either %s or %s", BroadcastServer, BroadcastClient))
	}
	var client *responseData
	if senderType == BroadcastClient {
		client = &responseData{
			messageID: messageID,
			method:    method,
			finished:  finished,
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	data := &content{
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

func (s *BroadcastState) addOrUpdate(broadcastID string, msg *content) error {
	s.mut.Lock()
	defer s.mut.Unlock()
	if old, ok := s.msgs[broadcastID]; ok {
		// update old with new
		if err := old.update(msg); err != nil {
			return err
		}
		s.msgs[broadcastID] = old
		return nil
	}
	s.msgs[broadcastID] = msg
	return nil
}

//func (s *BroadcastState) add(broadcastID string, msg content) error {
//s.mut.Lock()
//defer s.mut.Unlock()
//if _, ok := s.msgs[broadcastID]; ok {
//return errors.New("already added")
//}
//s.msgs[broadcastID] = msg
//return nil
//}

//func (s *BroadcastState) update(broadcastID string, new content) error {
//s.mut.Lock()
//defer s.mut.Unlock()
//if old, ok := s.msgs[broadcastID]; ok {
//// update old with new
//err := old.update(new)
//if err != nil {
//return err
//}
//// data is not a pointer and must thus be saved
//s.msgs[broadcastID] = old
//return nil
//}
//return errors.New("not found")
//}

//func (s *BroadcastState) isValid(broadcastID string) bool {
//s.mut.RLock()
//defer s.mut.RUnlock()
//msg, ok := s.msgs[broadcastID]
//if !ok {
//return false
//}
//// the request is done if the server has replied to the client
//return !msg.shouldWaitForClient()
//}

var emptyContent = content{}

func (s *BroadcastState) get(broadcastID string) (content, error) {
	s.mut.RLock()
	defer s.mut.RUnlock()
	if msg, ok := s.msgs[broadcastID]; ok {
		return *msg, nil
	}
	return emptyContent, errors.New("not found")
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

func (s *BroadcastState) setShouldWaitForClient(broadcastID string, response *reply) error {
	s.mut.Lock()
	defer s.mut.Unlock()
	msg, ok := s.msgs[broadcastID]
	if !ok {
		return errors.New("not found")
	}
	return msg.setResponse(response)
	//if err := msg.setResponse(response); err != nil {
	//	return
	//}
	//s.msgs[broadcastID] = msg
}

func (s *BroadcastState) prune() error {
	s.msgs = make(map[string]*content)
	s.doneChan = make(chan struct{})
	return nil
}
