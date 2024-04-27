package broadcast

import (
	"context"
	"errors"
	"log/slog"
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
	broadcastChan chan Msg
	sendChan      chan Content
	ctx           context.Context
	cancelFunc    context.CancelFunc
	//sync.Once
}

type BroadcastState struct {
	parentCtx           context.Context
	parentCtxCancelFunc context.CancelFunc
	logger              *slog.Logger
	reqTTL              time.Duration
	sendBuffer          int
	shardBuffer         int
	metrics             *Metric
	snowflake           *Snowflake
	clients             map[string]*Client

	shards []*shardElement
}

func NewState(logger *slog.Logger, metrics *Metric) *BroadcastState {
	shardBuffer := 100
	TTL := 5 * time.Second
	ctx, cancel := context.WithCancel(context.Background())
	shards := createShards(ctx, shardBuffer)
	state := &BroadcastState{
		parentCtx:           ctx,
		parentCtxCancelFunc: cancel,
		shards:              shards,
		logger:              logger,
		reqTTL:              TTL,
		sendBuffer:          5,
		shardBuffer:         shardBuffer,
		metrics:             metrics,
		clients:             make(map[string]*Client),
	}
	return state
}

func (s *BroadcastState) Close() error {
	s.prune()
	var err error
	for _, client := range s.clients {
		err = client.Close()
	}
	return err
}

func (state *BroadcastState) prune() {
	if state.logger != nil {
		state.logger.Debug("broadcast: pruned reqs")
	}
	state.parentCtxCancelFunc()
}

type Content struct {
	BroadcastID       uint64
	IsBroadcastClient bool
	OriginAddr        string
	OriginMethod      string
	ReceiveChan       chan error
	SendFn            func(resp protoreflect.ProtoMessage, err error)
}

func (c Content) send(resp protoreflect.ProtoMessage, err error) error {
	if c.SendFn == nil {
		return errors.New("has not received client req yet")
	}
	//if c.senderType != BroadcastClient {
	//return errors.New("has not received client req yet")
	//}
	c.SendFn(resp, err)
	return nil
}
