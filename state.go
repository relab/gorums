package gorums

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/relab/gorums/ordering"
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

type BroadcastState struct {
	mut      sync.Mutex // change to mutex
	msgs     map[string]*content
	logger   *slog.Logger
	doneChan chan struct{}
}

func newBroadcastStorage(logger *slog.Logger) *BroadcastState {
	return &BroadcastState{
		msgs:     make(map[string]*content),
		logger:   logger,
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
	data := &content{
		ctx: ctx,
		//cancelFunc:   nil,
		senderType:   senderType,
		originAddr:   originAddr,
		originMethod: originMethod,
		methods:      make([]string, 0),
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
		s.logger.Debug("broadcast: updated req", "broadcastID", broadcastID, "msg", msg)
		s.msgs[broadcastID] = old
		return nil
	}
	s.logger.Debug("broadcast: added req", "broadcastID", broadcastID, "msg", msg)
	msg.mut = &sync.Mutex{}
	s.msgs[broadcastID] = msg
	return nil
}

var emptyContent = content{}

func (s *BroadcastState) lockRequest(broadcastID string) (func(), content, error) {
	content, err := s.get(broadcastID)
	if err != nil {
		return func() {}, content, err
	}
	content.mut.Lock()
	return func() { content.mut.Unlock() }, content, nil
}

func (s *BroadcastState) get(broadcastID string) (content, error) {
	s.mut.Lock()
	defer s.mut.Unlock()
	if msg, ok := s.msgs[broadcastID]; ok {
		return *msg, nil
	}
	return emptyContent, errors.New("not found")
}

func (s *BroadcastState) remove(broadcastID string) error {
	s.mut.Lock()
	defer s.mut.Unlock()
	msg, ok := s.msgs[broadcastID]
	if !ok {
		return errors.New("not found")
	}
	//delete(s.msgs, broadcastID)
	msg.setDone()
	s.logger.Debug("broadcast: removed req", "broadcastID", broadcastID)
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
}

func (s *BroadcastState) setBroadcasted(broadcastID, method string) error {
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
	s.msgs = make(map[string]*content)
	s.doneChan = make(chan struct{})
	s.logger.Debug("broadcast: pruned reqs")
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

type content struct {
	mut          *sync.Mutex
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
	done         bool
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
	return c.done
}

func (c *content) setDone() {
	c.done = true
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
	if old.senderType == BroadcastServer && new.senderType == BroadcastClient {
		old.senderType = BroadcastClient
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
