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

type CacheOption int

/*
redis:

  - noeviction: New values aren’t saved when memory limit is reached. When a database uses replication, this applies to the primary database
  - allkeys-lru: Keeps most recently used keys; removes least recently used (LRU) keys
  - allkeys-lfu: Keeps frequently used keys; removes least frequently used (LFU) keys
  - volatile-lru: Removes least recently used keys with the expire field set to true.
  - volatile-lfu: Removes least frequently used keys with the expire field set to true.
  - allkeys-random: Randomly removes keys to make space for the new data added.
  - volatile-random: Randomly removes keys with expire field set to true.
  - volatile-ttl: Removes keys with expire field set to true and the shortest remaining time-to-live (TTL) value.
*/
const (
	noeviction CacheOption = iota
	allkeysLRU
	allkeysLFU
	volatileLRU
	volatileLFU
	allkeysRANDOM
	volatileRANDOM
	volatileTTL
)

type reqContent struct {
	broadcastChan chan bMsg
	sendChan      chan content2
	ctx           context.Context
	cancelFunc    context.CancelFunc
	//sync.Once
}

type shardElement struct {
	sendChan      chan content2
	broadcastChan chan bMsg
	ctx           context.Context
	cancelFunc    context.CancelFunc
	//sync.Once
}

type BroadcastState struct {
	parentCtx        context.Context
	parentCancelFunc context.CancelFunc
	mut              sync.Mutex
	msgs             map[uint64]*content
	logger           *slog.Logger
	doneChan         chan struct{}
	reqs             map[uint64]*reqContent
	reqTTL           time.Duration
	sendBuffer       int
	shardBuffer      int

	shards [16]*shardElement
}

func newBroadcastStorage(logger *slog.Logger, router IBroadcastRouter) *BroadcastState {
	shardBuffer := 100
	TTL := 5 * time.Second
	ctx, cancel := context.WithCancel(context.Background())
	shards := createShards(ctx, shardBuffer)
	state := &BroadcastState{
		parentCtx:        ctx,
		parentCancelFunc: cancel,
		msgs:             make(map[uint64]*content),
		shards:           shards,
		logger:           logger,
		doneChan:         make(chan struct{}),
		reqs:             make(map[uint64]*reqContent),
		reqTTL:           TTL,
		sendBuffer:       5,
		shardBuffer:      shardBuffer,
	}
	for i := 0; i < 16; i++ {
		go state.runShard(shards[i], router, state.reqTTL, state.sendBuffer, state.shardBuffer)
	}
	return state
}

func createShards(ctx context.Context, shardBuffer int) [16]*shardElement {
	var shards [16]*shardElement
	for i := 0; i < 16; i++ {
		ctx, cancel := context.WithCancel(ctx)
		shard := &shardElement{
			sendChan:      make(chan content2, shardBuffer),
			broadcastChan: make(chan bMsg, shardBuffer),
			ctx:           ctx,
			cancelFunc:    cancel,
		}
		shards[i] = shard
	}
	return shards
}

func (s *BroadcastState) runShard(shard *shardElement, router IBroadcastRouter, reqTTL time.Duration, sendBuffer, shardBuffer int) {
	//slog.Info("shard running", "ID", i)
	reqs := make(map[uint64]*reqContent, shardBuffer)
	for {
		select {
		case <-shard.ctx.Done():
			//slog.Info("shard done")
			return
		case msg := <-shard.sendChan:
			//slog.Info("shard got req")
			if req, ok := reqs[msg.broadcastID]; ok {
				select {
				case <-req.ctx.Done():
					msg.receiveChan <- errors.New(fmt.Sprintf("req is done. broadcastID: %v", msg.broadcastID))
				case req.sendChan <- msg:
				}
			} else {
				ctx, cancel := context.WithTimeout(s.parentCtx, reqTTL)
				req := &reqContent{
					ctx:           ctx,
					cancelFunc:    cancel,
					sendChan:      make(chan content2, sendBuffer),
					broadcastChan: make(chan bMsg, sendBuffer),
				}
				reqs[msg.broadcastID] = req
				go handleReq(router, msg.broadcastID, req, msg)
				select {
				case <-req.ctx.Done():
					msg.receiveChan <- errors.New("req is done")
				case req.sendChan <- msg:
				}
			}
		case msg := <-shard.broadcastChan:
			//slog.Info("shard got bMsg")
			if req, ok := reqs[msg.broadcastID]; ok {
				select {
				case <-req.ctx.Done():
				case req.broadcastChan <- msg:
				}
			}
		}
	}
}

func (s *BroadcastState) newData(ctx context.Context, isBroadcastClient bool, originAddr, originMethod string, messageID uint64, method string, finished chan<- *Message) (*content, error) {
	var client *responseData
	if isBroadcastClient {
		client = &responseData{
			messageID: messageID,
			method:    method,
			finished:  finished,
		}
	}
	data := &content{
		ctx: ctx,
		//cancelFunc:   nil,
		isBroadcastClient: isBroadcastClient,
		originAddr:        originAddr,
		originMethod:      originMethod,
		methods:           make([]string, 0),
		client:            client,
		timestamp:         time.Now(),
		reqTS:             time.Now(),
		sent:              false,
	}
	return data, nil
}

func (s *BroadcastState) addOrUpdate2(broadcastID uint64) (bool, *reqContent) {
	s.mut.Lock()
	defer s.mut.Unlock()
	if req, ok := s.reqs[broadcastID]; ok {
		return false, req
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.reqTTL)
	rC := &reqContent{
		ctx:           ctx,
		cancelFunc:    cancel,
		sendChan:      make(chan content2, s.sendBuffer),
		broadcastChan: make(chan bMsg, s.sendBuffer),
	}
	s.reqs[broadcastID] = rC
	return true, rC
}

func (s *BroadcastState) process(msg content2) error {
	_, shardID, _, _ := decodeBroadcastID(msg.broadcastID)
	if shardID >= 16 {
		return errors.New("wrong shardID")
	}
	//s.mut.Lock()
	//shard := s.shards[shardID]
	//s.mut.Unlock()
	shard := s.shards[shardID]

	receiveChan := make(chan error)
	msg.receiveChan = receiveChan
	select {
	case shard.sendChan <- msg:
	case <-shard.ctx.Done():
		return errors.New("shard is down")
	}
	return <-receiveChan
}

func (s *BroadcastState) processBroadcast(broadcastID uint64, req protoreflect.ProtoMessage, method string) {
	_, shardID, _, _ := decodeBroadcastID(broadcastID)
	if shardID >= 16 {
		return
	}
	//s.mut.Lock()
	//shard := s.shards[shardID]
	//s.mut.Unlock()
	shard := s.shards[shardID]
	select {
	case shard.broadcastChan <- bMsg{
		broadcast:   true,
		msg:         newBroadcastMessage2(broadcastID, req, method),
		method:      method,
		broadcastID: broadcastID,
	}:
	case <-shard.ctx.Done():
	}
}

func (s *BroadcastState) processSendToClient(broadcastID uint64, resp protoreflect.ProtoMessage, err error) {
	_, shardID, _, _ := decodeBroadcastID(broadcastID)
	if shardID >= 16 {
		return
	}
	//s.mut.Lock()
	//shard := s.shards[shardID]
	//s.mut.Unlock()
	shard := s.shards[shardID]
	select {
	case shard.broadcastChan <- bMsg{
		reply: &reply{
			response: resp,
			err:      err,
		},
		broadcastID: broadcastID,
	}:
	case <-shard.ctx.Done():
	}
}

func (s *BroadcastState) get2(broadcastID uint64) (bool, *reqContent) {
	s.mut.Lock()
	defer s.mut.Unlock()
	req, ok := s.reqs[broadcastID]
	return ok, req
}

func (s *BroadcastState) add2(broadcastID uint64) (bool, *reqContent) {
	ctx, cancel := context.WithTimeout(s.parentCtx, s.reqTTL)
	rC := reqContent{
		ctx:           ctx,
		cancelFunc:    cancel,
		sendChan:      make(chan content2, s.sendBuffer),
		broadcastChan: make(chan bMsg, s.sendBuffer),
	}
	s.mut.Lock()
	defer s.mut.Unlock()
	if req, ok := s.reqs[broadcastID]; ok {
		cancel()
		return false, req
	}
	s.reqs[broadcastID] = &rC
	return true, &rC
}

func (s *BroadcastState) addOrUpdate(broadcastID uint64, msg *content) error {
	s.mut.Lock()
	defer s.mut.Unlock()
	if old, ok := s.msgs[broadcastID]; ok {
		// update old with new. err if msg is done.
		if err := old.update(msg); err != nil {
			return err
		}
		s.logger.Debug("broadcast: updated req", "broadcastID", broadcastID, "msg", msg)
		s.msgs[broadcastID] = old // NOT NECESSARY??
		return nil
	}
	s.logger.Debug("broadcast: added req", "broadcastID", broadcastID, "msg", msg)
	msg.mut = &sync.Mutex{}
	s.msgs[broadcastID] = msg
	return nil
}

var emptyContent = content{}
var emptyContent2 = content2{}

func (s *BroadcastState) lockRequest(broadcastID uint64) (func(), content, error) {
	content, err := s.get(broadcastID)
	if err != nil {
		return func() {}, content, err
	}
	//slog.Info("broadcast: before locking req", "broadcastID", broadcastID, "took", content.getTotalTime())
	content.mut.Lock()
	return func() { content.mut.Unlock() }, content, nil
}

func (s *BroadcastState) getReqContent(broadcastID uint64) (*reqContent, error) {
	s.mut.Lock()
	defer s.mut.Unlock()
	if req, ok := s.reqs[broadcastID]; ok {
		return req, nil
	}
	return nil, errors.New("not found")
}

func (s *BroadcastState) get(broadcastID uint64) (content, error) {
	s.mut.Lock()
	defer s.mut.Unlock()
	if msg, ok := s.msgs[broadcastID]; ok {
		return *msg, nil
	}
	return emptyContent, errors.New("not found")
}

func (s *BroadcastState) remove(broadcastID uint64) error {
	s.mut.Lock()
	defer s.mut.Unlock()
	msg, ok := s.msgs[broadcastID]
	if !ok {
		return errors.New("not found")
	}
	//delete(s.msgs, broadcastID)
	msg.setDone()
	s.logger.Debug("broadcast: removed req", "broadcastID", broadcastID, "took", msg.getTotalTime())
	return nil
}

func (s *BroadcastState) setShouldWaitForClient(broadcastID uint64, response *reply) error {
	s.mut.Lock()
	defer s.mut.Unlock()
	msg, ok := s.msgs[broadcastID]
	if !ok {
		return errors.New("not found")
	}
	return msg.setResponse(response)
}

func (s *BroadcastState) setBroadcasted(broadcastID uint64, method string) error {
	s.mut.Lock()
	defer s.mut.Unlock()
	msg, ok := s.msgs[broadcastID]
	if !ok {
		return errors.New("not found")
	}
	msg.addMethod(method)
	return nil
}

func (s *BroadcastState) prune() error {
	s.msgs = make(map[uint64]*content)
	s.reqs = make(map[uint64]*reqContent)
	s.doneChan = make(chan struct{})
	s.logger.Debug("broadcast: pruned reqs")
	s.parentCancelFunc()
	return nil
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

type content2 struct {
	broadcastID       uint64
	isBroadcastClient bool
	originAddr        string
	originMethod      string
	receiveChan       chan error
	sendFn            func(resp protoreflect.ProtoMessage, err error)
}

func (c content2) send(resp protoreflect.ProtoMessage, err error) error {
	if c.sendFn == nil {
		return errors.New("has not received client req yet")
	}
	//if c.senderType != BroadcastClient {
	//return errors.New("has not received client req yet")
	//}
	c.sendFn(resp, err)
	return nil
}

type content struct {
	mut               *sync.Mutex
	ctx               context.Context
	cancelFunc        context.CancelFunc
	client            *responseData
	isBroadcastClient bool
	originAddr        string
	originMethod      string
	methods           []string
	timestamp         time.Time
	reqTS             time.Time
	sent              bool
	done              bool
	receiveChan       chan error
}

func (c *content) getCtx() context.Context {
	if c.ctx != nil {
		return c.ctx
	}
	return context.Background()
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
	return c.isBroadcastClient
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
	return c.sent && c.isBroadcastClient
}

func (c *content) getTotalTime() time.Duration {
	return time.Since(c.timestamp)
}

func (c *content) getProcessingTime() time.Duration {
	return time.Since(c.reqTS)
}

func (c *content) isDone() bool {
	if c.ctx != nil {
		select {
		case <-c.ctx.Done():
			return true
		default:
		}
	}
	return c.done
}

func (c *content) setDone() {
	c.done = true
	if c.cancelFunc != nil {
		c.cancelFunc()
	}
	//go func() {
	//time.Sleep(100 * time.Millisecond)
	//c.cancelFunc()
	//}()
}

func (old *content) update(new *content) error {
	if old.isDone() {
		return errors.New("req is done")
	}
	old.reqTS = time.Now()
	if old.originAddr == "" && new.originAddr != "" {
		old.originAddr = new.originAddr
	}
	if old.originMethod == "" && new.originMethod != "" {
		old.originMethod = new.originMethod
	}
	if !old.isBroadcastClient && new.isBroadcastClient {
		old.isBroadcastClient = true
		if new.client != nil && new.client.getFinished() != nil {
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
		}
	}
	return nil
}

func (c *content) hasBeenBroadcasted(method string) bool {
	for _, m := range c.methods {
		if m == method {
			return true
		}
	}
	return false
}

func (c *content) addMethod(method string) {
	c.methods = append(c.methods, method)
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
	c.client.addResponse(response)
	c.sent = true
	return nil
}
