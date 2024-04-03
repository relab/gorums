package gorums

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/relab/gorums/ordering"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type content interface {
	getCtx() context.Context
	getFinished() chan<- *Message
	getMetadata() *ordering.Metadata
	isFromClient() bool
	getMethod() string
	getClientHandlerName() string
	getOriginAddr() string
	getOriginMethod() string
	shouldWaitForClient() bool
	update(content) error
	getResponse() *reply
	setResponse(*reply)
}

type clientReq struct {
	ctx        context.Context
	finished   chan<- *Message
	metadata   *ordering.Metadata
	senderType string
	methods    []string
	timestamp  time.Time
	doneChan   chan struct{}
	sent       bool
}

func (c *clientReq) getMetadata() *ordering.Metadata {
	return &ordering.Metadata{
		MessageID: c.metadata.MessageID,
	}
}

func (c *clientReq) isFromClient() bool {
	return c.senderType == BroadcastClient
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

func (r *BroadcastRouter) route(broadcastID string, data content, req any) error {
	switch val := req.(type) {
	case *reply:
		return r.routeClientReply(broadcastID, data, val)
	case *broadcastMsg:
		return r.routeBroadcast(broadcastID, data, val)
	}
	return errors.New("wrong req type")
}

func (r *BroadcastRouter) routeBroadcast(broadcastID string, data content, msg *broadcastMsg) error {
	// set the message as handled when returning from the method
	defer msg.setFinished()
	if handler, ok := r.serverHandlers[msg.method]; ok {
		// it runs an interceptor prior to broadcastCall, hence a different signature.
		// see (srv *broadcastServer) registerBroadcastFunc(method string).
		handler(msg.ctx, msg.request, broadcastID, data.getOriginAddr(), data.getOriginMethod(), msg.options, r.id, r.addr)
		return nil
	}
	return errors.New("not found")
}

func (r *BroadcastRouter) routeClientReply(broadcastID string, data content, resp *reply) error {
	// the client has initiated a broadcast call and the reply should be sent as an RPC
	if handler, ok := r.clientHandlers[data.getClientHandlerName()]; ok && data.getOriginAddr() != "" {
		handler(data.getOriginAddr(), broadcastID, resp.getResponse())
		return nil
	}
	// there is a direct connection to the client, e.g. from a QuorumCall
	if data.isFromClient() {
		msg := WrapMessage(data.getMetadata(), protoreflect.ProtoMessage(resp.getResponse()), resp.getError())
		SendMessage(data.getCtx(), data.getFinished(), msg)
		return nil
	}
	// the server can receive a broadcast from another server before a client sends a direct message.
	// it should thus wait for a potential message from the client. otherwise, it should be removed.
	return errors.New("not routed")
}

type BroadcastStorage struct {
	mut      sync.RWMutex
	msgs     map[string]content
	doneChan chan struct{}
}

func newBroadcastStorage() *BroadcastStorage {
	return &BroadcastStorage{
		msgs:     make(map[string]content),
		doneChan: make(chan struct{}),
	}
}

func (s *BroadcastStorage) add(broadcastID string, msg content) error {
	s.mut.Lock()
	defer s.mut.Unlock()
	if _, ok := s.msgs[broadcastID]; ok {
		return errors.New("already added")
	}
	s.msgs[broadcastID] = msg
	return nil
}

func (s *BroadcastStorage) update(broadcastID string, new content) error {
	s.mut.Lock()
	defer s.mut.Unlock()
	if old, ok := s.msgs[broadcastID]; ok {
		return old.update(new)
	}
	return errors.New("not found")
}

func (s *BroadcastStorage) get(broadcastID string, onlyValid bool) (content, error) {
	s.mut.RLock()
	defer s.mut.RUnlock()
	if msg, ok := s.msgs[broadcastID]; ok {
		if onlyValid && msg.shouldWaitForClient() {
			return nil, errors.New("not valid")
		}
		return msg, nil
	}
	return nil, errors.New("not found")
}

func (s *BroadcastStorage) remove(broadcastID string) error {
	s.mut.Lock()
	defer s.mut.Unlock()
	_, ok := s.msgs[broadcastID]
	if !ok {
		return errors.New("not found")
	}
	delete(s.msgs, broadcastID)
	return nil
}

func (s *BroadcastStorage) setShouldWaitForClient(broadcastID string, response *reply) {
	s.mut.Lock()
	defer s.mut.Unlock()
	msg, ok := s.msgs[broadcastID]
	if !ok {
		return
	}
	msg.setResponse(response)
}
