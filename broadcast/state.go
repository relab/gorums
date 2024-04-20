package broadcast

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"sync"
	"time"

	"google.golang.org/protobuf/reflect/protoreflect"
)

type Snowflake struct {
	mut         sync.Mutex
	MachineID   uint64
	SequenceNum uint64
	lastT       uint64
	lastS       uint64
	epoch       time.Time
}

const (
	maxShard           = float32(1 << 4)
	maxMachineID       = float32(1 << 12)
	maxSequenceNum     = uint32(1 << 18)
	bitMaskTimestamp   = uint64((1<<30)-1) << 34
	bitMaskShardID     = uint64((1<<4)-1) << 30
	bitMaskMachineID   = uint64((1<<12)-1) << 18
	bitMaskSequenceNum = uint64((1 << 18) - 1)
	epoch              = "2024-01-01T00:00:00"
)

func NewSnowflake(addr string) *Snowflake {
	timestamp, _ := time.Parse("2006-01-02T15:04:05", epoch)
	return &Snowflake{
		MachineID:   uint64(rand.Int31n(int32(maxMachineID))),
		SequenceNum: 0,
		epoch:       timestamp,
		//sequenceNum: uint32(maxSequenceNum * rand.Float32()),
	}
}

func (s *Snowflake) NewBroadcastID() uint64 {
	// timestamp: 30 bit -> seconds since 01.01.2024
	// shardID: 4 bit -> 16 different shards
	// machineID: 12 bit -> 4096 clients
	// sequenceNum: 18 bit -> 262 144 messages
start:
	s.mut.Lock()
	timestamp := uint64(time.Since(s.epoch).Seconds())
	l := (s.SequenceNum + 1) % uint64(maxSequenceNum)
	if timestamp-s.lastT <= 0 && l == s.lastS {
		s.mut.Unlock()
		time.Sleep(10 * time.Millisecond)
		goto start
	}
	if timestamp > s.lastT {
		s.lastT = timestamp
		s.lastS = l
	}
	s.SequenceNum = l
	s.mut.Unlock()

	t := (timestamp << 34) & bitMaskTimestamp
	shard := (uint64(rand.Int31n(int32(maxShard))) << 30) & bitMaskShardID
	m := uint64(s.MachineID<<18) & bitMaskMachineID
	n := l & bitMaskSequenceNum
	return t | shard | m | n
}

func DecodeBroadcastID(broadcastID uint64) (uint32, uint16, uint16, uint32) {
	t := (broadcastID & bitMaskTimestamp) >> 34
	shard := (broadcastID & bitMaskShardID) >> 30
	m := (broadcastID & bitMaskMachineID) >> 18
	n := (broadcastID & bitMaskSequenceNum)
	return uint32(t), uint16(shard), uint16(m), uint32(n)
}

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
	sendChan      chan Content2
	ctx           context.Context
	cancelFunc    context.CancelFunc
	//sync.Once
}

type shardElement struct {
	sendChan      chan Content2
	broadcastChan chan Msg
	ctx           context.Context
	cancelFunc    context.CancelFunc
	//sync.Once
}

type BroadcastState struct {
	parentCtx        context.Context
	parentCancelFunc context.CancelFunc
	mut              sync.Mutex
	logger           *slog.Logger
	doneChan         chan struct{}
	reqs             map[uint64]*reqContent
	reqTTL           time.Duration
	sendBuffer       int
	shardBuffer      int

	shards [16]*shardElement
}

func NewState(logger *slog.Logger, router IBroadcastRouter) *BroadcastState {
	shardBuffer := 100
	TTL := 5 * time.Second
	ctx, cancel := context.WithCancel(context.Background())
	shards := createShards(ctx, shardBuffer)
	state := &BroadcastState{
		parentCtx:        ctx,
		parentCancelFunc: cancel,
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
			sendChan:      make(chan Content2, shardBuffer),
			broadcastChan: make(chan Msg, shardBuffer),
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
			if req, ok := reqs[msg.BroadcastID]; ok {
				select {
				case <-req.ctx.Done():
					msg.ReceiveChan <- errors.New(fmt.Sprintf("req is done. broadcastID: %v", msg.BroadcastID))
				case req.sendChan <- msg:
				}
			} else {
				ctx, cancel := context.WithTimeout(s.parentCtx, reqTTL)
				req := &reqContent{
					ctx:           ctx,
					cancelFunc:    cancel,
					sendChan:      make(chan Content2, sendBuffer),
					broadcastChan: make(chan Msg, sendBuffer),
				}
				reqs[msg.BroadcastID] = req
				go HandleReq(router, msg.BroadcastID, req, msg)
				select {
				case <-req.ctx.Done():
					msg.ReceiveChan <- errors.New("req is done")
				case req.sendChan <- msg:
				}
			}
		case msg := <-shard.broadcastChan:
			//slog.Info("shard got bMsg")
			if req, ok := reqs[msg.BroadcastID]; ok {
				select {
				case <-req.ctx.Done():
				case req.broadcastChan <- msg:
				}
			}
		}
	}
}

func (s *BroadcastState) Process(msg Content2) error {
	_, shardID, _, _ := DecodeBroadcastID(msg.BroadcastID)
	if shardID >= 16 {
		return errors.New("wrong shardID")
	}
	//s.mut.Lock()
	//shard := s.shards[shardID]
	//s.mut.Unlock()
	shard := s.shards[shardID]

	receiveChan := make(chan error)
	msg.ReceiveChan = receiveChan
	select {
	case shard.sendChan <- msg:
	case <-shard.ctx.Done():
		return errors.New("shard is down")
	}
	return <-receiveChan
}

func (s *BroadcastState) ProcessBroadcast(broadcastID uint64, req protoreflect.ProtoMessage, method string) {
	_, shardID, _, _ := DecodeBroadcastID(broadcastID)
	if shardID >= 16 {
		return
	}
	//s.mut.Lock()
	//shard := s.shards[shardID]
	//s.mut.Unlock()
	shard := s.shards[shardID]
	select {
	case shard.broadcastChan <- Msg{
		Broadcast:   true,
		Msg:         NewMsg(broadcastID, req, method),
		Method:      method,
		BroadcastID: broadcastID,
	}:
	case <-shard.ctx.Done():
	}
}

func (s *BroadcastState) ProcessSendToClient(broadcastID uint64, resp protoreflect.ProtoMessage, err error) {
	_, shardID, _, _ := DecodeBroadcastID(broadcastID)
	if shardID >= 16 {
		return
	}
	//s.mut.Lock()
	//shard := s.shards[shardID]
	//s.mut.Unlock()
	shard := s.shards[shardID]
	select {
	case shard.broadcastChan <- Msg{
		Reply: &reply{
			Response: resp,
			Err:      err,
		},
		BroadcastID: broadcastID,
	}:
	case <-shard.ctx.Done():
	}
}

var emptyContent2 = Content2{}

func (s *BroadcastState) Prune() error {
	s.reqs = make(map[uint64]*reqContent)
	s.doneChan = make(chan struct{})
	if s.logger != nil {
		s.logger.Debug("broadcast: pruned reqs")
	}
	s.parentCancelFunc()
	return nil
}

type Content2 struct {
	BroadcastID       uint64
	IsBroadcastClient bool
	OriginAddr        string
	OriginMethod      string
	ReceiveChan       chan error
	SendFn            func(resp protoreflect.ProtoMessage, err error)
}

func (c Content2) send(resp protoreflect.ProtoMessage, err error) error {
	if c.SendFn == nil {
		return errors.New("has not received client req yet")
	}
	//if c.senderType != BroadcastClient {
	//return errors.New("has not received client req yet")
	//}
	c.SendFn(resp, err)
	return nil
}
