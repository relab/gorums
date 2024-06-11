package broadcast

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"google.golang.org/protobuf/reflect/protoreflect"
)

type BroadcastState struct {
	mut                 sync.Mutex
	shardMut            sync.RWMutex // RW because we often read and very seldom write to the state
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

func NewState(logger *slog.Logger, router Router, order map[string]int, reqTTL time.Duration, shardBuffer, sendBuffer int) *BroadcastState {
	ctx, cancel := context.WithCancel(context.Background())
	shards := createShards(ctx, shardBuffer, sendBuffer, router, order, reqTTL, logger)
	state := &BroadcastState{
		parentCtx:           ctx,
		parentCtxCancelFunc: cancel,
		shards:              shards,
		logger:              logger,
		reqTTL:              reqTTL,
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

func (s *BroadcastState) reset() {
	s.parentCtxCancelFunc()
	s.mut.Lock()
	s.parentCtx, s.parentCtxCancelFunc = context.WithCancel(context.Background())
	for _, client := range s.clients {
		_ = client.Close()
	}
	s.clients = make(map[string]*Client)
	shards := createShards(s.parentCtx, s.shardBuffer, s.sendBuffer, s.router, s.order, s.reqTTL, s.logger)
	s.mut.Unlock()
	// unlocking because we don't want to end up with a deadlock.
	s.shardMut.Lock()
	s.shards = shards
	s.shardMut.Unlock()
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

func (s *BroadcastState) getShard(i uint16) *shard {
	s.shardMut.RLock()
	defer s.shardMut.RUnlock()
	return s.shards[i]
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
	enqueueBroadcast func(*Msg) error
}

type Content struct {
	BroadcastID       uint64
	IsBroadcastClient bool
	IsCancellation    bool
	OriginAddr        string
	OriginMethod      string
	OriginPubKey      string
	OriginSignature   []byte
	OriginDigest      []byte
	ViewNumber        uint64
	SenderAddr        string
	CurrentMethod     string
	ReceiveChan       chan shardResponse
	SendFn            func(resp protoreflect.ProtoMessage, err error) error
	Ctx               context.Context
	CancelCtx         context.CancelFunc
	Run               func(context.Context, func(*Msg) error)
}
