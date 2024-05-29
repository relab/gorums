package broadcast

import (
	"context"
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
*/

type BroadcastState struct {
	mut                 sync.Mutex
	parentCtx           context.Context
	parentCtxCancelFunc context.CancelFunc
	logger              *slog.Logger
	reqTTL              time.Duration
	sendBuffer          int
	shardBuffer         int
	snowflake           *Snowflake
	clients             map[string]*Client
	router              Router
	order               map[string]int

	shards []*shard
}

func NewState(logger *slog.Logger, router Router, order map[string]int) *BroadcastState {
	shardBuffer := 100
	sendBuffer := 30
	TTL := 5 * time.Minute
	ctx, cancel := context.WithCancel(context.Background())
	shards := createShards(ctx, shardBuffer, sendBuffer, router, order, TTL, logger)
	state := &BroadcastState{
		parentCtx:           ctx,
		parentCtxCancelFunc: cancel,
		shards:              shards,
		logger:              logger,
		reqTTL:              TTL,
		sendBuffer:          sendBuffer,
		shardBuffer:         shardBuffer,
		router:              router,
		order:               order,
		clients:             make(map[string]*Client),
	}
	return state
}

func (s *BroadcastState) Close() error {
	s.mut.Lock()
	defer s.mut.Unlock()
	if s.logger != nil {
		s.logger.Debug("broadcast: closing state")
	}
	//s.debug()
	s.parentCtxCancelFunc()
	var err error
	for _, client := range s.clients {
		clientErr := client.Close()
		if clientErr != nil {
			err = clientErr
		}
	}
	return err
}

func (s *BroadcastState) debug() {
	time.Sleep(1 * time.Second)
	for _, shard := range s.shards {
		for _, req := range shard.reqs {
			select {
			case <-req.ctx.Done():
			default:
				slog.Info("req not done", "req", req)
			}
		}
	}
}

func (s *BroadcastState) RunShards() {
	return
	//for _, shard := range s.shards {
	//go shard.run(s.sendBuffer)
	//}
}

func (s *BroadcastState) reset() {
	s.mut.Lock()
	defer s.mut.Unlock()
	s.parentCtxCancelFunc()
	ctx, cancel := context.WithCancel(context.Background())
	s.parentCtx = ctx
	s.parentCtxCancelFunc = cancel
	s.shards = createShards(ctx, s.shardBuffer, s.sendBuffer, s.router, s.order, s.reqTTL, s.logger)
	s.RunShards()
	for _, client := range s.clients {
		client.Close()
	}
	s.clients = make(map[string]*Client)
}

func (s *BroadcastState) getClient(addr string) (*Client, bool) {
	s.mut.Lock()
	defer s.mut.Unlock()
	client, ok := s.clients[addr]
	return client, ok
}

func (s *BroadcastState) addClient(addr string, client *Client) {
	s.mut.Lock()
	defer s.mut.Unlock()
	s.clients[addr] = client
}

func (state *BroadcastState) getStats() shardMetrics {
	m := shardMetrics{
		lifetimes: make([][]time.Time, 0),
	}
	for _, shard := range state.shards {
		metric := shard.getStats()
		m.totalMsgs += metric.totalMsgs
		m.numMsgs += metric.numMsgs
		m.droppedMsgs += metric.droppedMsgs
		m.numBroadcastMsgs += metric.numBroadcastMsgs
		m.droppedBroadcastMsgs += metric.droppedBroadcastMsgs
		m.numReqs += metric.numReqs
		m.finishedReqs += metric.finishedReqs
		m.lifetimes = append(m.lifetimes, metric.lifetimes...)
		m.avgLifetime += metric.avgLifetime
		m.maxLifetime += metric.maxLifetime
		m.minLifetime += metric.minLifetime
	}
	if m.numReqs > 0 {
		m.avgLifetime /= time.Duration(m.numReqs)
	}
	return m
}

type shardResponse struct {
	err              error
	reqCtx           context.Context
	enqueueBroadcast func(Msg) error
}

type Content struct {
	BroadcastID       uint64
	IsBroadcastClient bool
	IsCancellation    bool
	OriginAddr        string
	OriginMethod      string
	SenderAddr        string
	CurrentMethod     string
	ReceiveChan       chan shardResponse
	SendFn            func(resp protoreflect.ProtoMessage, err error) error
	Ctx               context.Context
	CancelCtx         context.CancelFunc
	Run               func(context.Context, func(Msg) error)
}

func (c Content) send(resp protoreflect.ProtoMessage, err error) error {
	if !c.hasReceivedClientRequest() {
		return MissingClientReqErr{}
	}
	//if c.senderType != BroadcastClient {
	//return errors.New("has not received client req yet")
	//}
	c.SendFn(resp, err)
	return nil
}
