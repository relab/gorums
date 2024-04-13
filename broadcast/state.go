package broadcast

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

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
	sendChan      chan Content2
	ctx           context.Context
	cancelFunc    context.CancelFunc
}

type BroadcastState struct {
	mut        sync.Mutex
	logger     *slog.Logger
	doneChan   chan struct{}
	reqs       map[string]*reqContent
	reqTTL     time.Duration
	sendBuffer int
}

func newBroadcastStorage(logger *slog.Logger) *BroadcastState {
	return &BroadcastState{
		logger:   logger,
		doneChan: make(chan struct{}),
		reqs:     make(map[string]*reqContent),
		reqTTL:   5 * time.Second,
	}
}

const (
	BroadcastClient = "BroadcastClient"
	BroadcastServer = "BroadcastServer"
)

func (s *BroadcastState) addOrUpdate2(broadcastID string) (bool, *reqContent) {
	s.mut.Lock()
	defer s.mut.Unlock()
	if req, ok := s.reqs[broadcastID]; ok {
		return false, req
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.reqTTL)
	rC := &reqContent{
		ctx:           ctx,
		cancelFunc:    cancel,
		sendChan:      make(chan Content2, s.sendBuffer),
		broadcastChan: make(chan bMsg, s.sendBuffer),
	}
	s.reqs[broadcastID] = rC
	return true, rC
}

var emptyContent2 = Content2{}

func (s *BroadcastState) getReqContent(broadcastID string) (*reqContent, error) {
	s.mut.Lock()
	defer s.mut.Unlock()
	if req, ok := s.reqs[broadcastID]; ok {
		return req, nil
	}
	return nil, errors.New("not found")
}

func (s *BroadcastState) prune() error {
	s.doneChan = make(chan struct{})
	s.logger.Debug("broadcast: pruned reqs")
	return nil
}

type Content2 struct {
	resp         protoreflect.ProtoMessage
	err          error
	senderType   string
	originAddr   string
	originMethod string
	receiveChan  chan error
	sendFn       func(resp protoreflect.ProtoMessage, err error)
}

func (c Content2) send(resp protoreflect.ProtoMessage, err error) error {
	if c.sendFn == nil {
		return errors.New("has not received client req yet")
	}
	if c.senderType != BroadcastClient {
		return errors.New("has not received client req yet")
	}
	c.sendFn(resp, err)
	return nil
}

func (c Content2) retrySend() error {
	if c.sendFn == nil {
		return errors.New("has not received client req yet")
	}
	c.sendFn(c.resp, c.err)
	return nil
}
