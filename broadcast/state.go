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

type timingMetric struct {
	Avg time.Duration
	//Median time.Duration
	Min time.Duration
	Max time.Duration
}

type goroutineMetric struct {
	started time.Time
	ended   time.Time
}

type Metrics struct {
	mut               sync.Mutex
	TotalNum          uint64
	GoroutinesStarted uint64
	GoroutinesStopped uint64
	Goroutines        map[string]*goroutineMetric
	FinishedReqs      struct {
		Total     uint64
		Succesful uint64
		Failed    uint64
	}
	Processed         uint64
	Dropped           uint64
	Invalid           uint64
	AlreadyProcessed  uint64
	RoundTripLatency  timingMetric
	ReqLatency        timingMetric
	ShardDistribution map[uint16]uint64
	// measures unique number of broadcastIDs processed simultaneounsly
	ConcurrencyDistribution timingMetric
}

func NewMetrics() *Metrics {
	return &Metrics{
		ShardDistribution: make(map[uint16]uint64, 16),
		Goroutines:        make(map[string]*goroutineMetric, 2000),
	}
}

func (m *Metrics) Reset() {
	m.mut.Lock()
	defer m.mut.Unlock()
	m.TotalNum = 0
	m.GoroutinesStarted = 0
	m.GoroutinesStopped = 0
	m.Goroutines = make(map[string]*goroutineMetric, 2000)
	m.FinishedReqs.Total = 0
	m.FinishedReqs.Succesful = 0
	m.FinishedReqs.Failed = 0
	m.Processed = 0
	m.Dropped = 0
	m.Invalid = 0
	m.AlreadyProcessed = 0
	m.RoundTripLatency.Avg = 0
	m.RoundTripLatency.Min = 100 * time.Hour
	m.RoundTripLatency.Max = 0
	m.ReqLatency.Avg = 0
	m.ReqLatency.Min = 100 * time.Hour
	m.ReqLatency.Max = 0
	m.ShardDistribution = make(map[uint16]uint64, 16)
	// measures unique number of broadcastIDs processed simultaneounsly
	//m.ConcurrencyDistribution timingMetric
}

func (m *Metrics) String() string {
	m.mut.Lock()
	defer m.mut.Unlock()
	res := "Metrics:"
	res += "\n\t- TotalNum: " + fmt.Sprintf("%v", m.TotalNum)
	res += "\n\t- Goroutines started: " + fmt.Sprintf("%v", m.GoroutinesStarted)
	res += "\n\t- Goroutines stopped: " + fmt.Sprintf("%v", m.GoroutinesStopped)
	res += "\n\t- FinishedReqs:"
	res += "\n\t\t- Total: " + fmt.Sprintf("%v", m.FinishedReqs.Total)
	res += "\n\t\t- Successful: " + fmt.Sprintf("%v", m.FinishedReqs.Succesful)
	res += "\n\t\t- Failed: " + fmt.Sprintf("%v", m.FinishedReqs.Failed)
	res += "\n\t- Processed: " + fmt.Sprintf("%v", m.Processed)
	res += "\n\t- Dropped: " + fmt.Sprintf("%v", m.Dropped)
	res += "\n\t\t- Invalid: " + fmt.Sprintf("%v", m.Invalid)
	res += "\n\t\t- AlreadyProcessed: " + fmt.Sprintf("%v", m.AlreadyProcessed)
	res += "\n\t- RoundTripLatency:"
	res += "\n\t\t- Avg: " + fmt.Sprintf("%v", m.RoundTripLatency.Avg)
	res += "\n\t\t- Min: " + fmt.Sprintf("%v", m.RoundTripLatency.Min)
	res += "\n\t\t- Max: " + fmt.Sprintf("%v", m.RoundTripLatency.Max)
	res += "\n\t- ReqLatency:"
	res += "\n\t\t- Avg: " + fmt.Sprintf("%v", m.ReqLatency.Avg)
	res += "\n\t\t- Min: " + fmt.Sprintf("%v", m.ReqLatency.Min)
	res += "\n\t\t- Max: " + fmt.Sprintf("%v", m.ReqLatency.Max)
	res += "\n\t- ShardDistribution: " + fmt.Sprintf("%v", m.ShardDistribution)
	return res
}

func (m *Metrics) GetStats() *Metrics {
	if m == nil {
		return &Metrics{}
	}
	m.mut.Lock()
	defer m.mut.Unlock()
	if m.Processed > 0 {
		m.ReqLatency.Avg /= time.Duration(m.Processed)
	}
	if m.FinishedReqs.Total > 0 {
		m.RoundTripLatency.Avg /= time.Duration(m.FinishedReqs.Total)
	}
	return m
}

func (m *Metrics) AddReqLatency(start time.Time) {
	latency := time.Since(start)
	m.mut.Lock()
	defer m.mut.Unlock()
	if latency > m.ReqLatency.Max {
		m.ReqLatency.Max = latency
	}
	if latency < m.ReqLatency.Min {
		m.ReqLatency.Min = latency
	}
	m.ReqLatency.Avg += latency
}

func (m *Metrics) AddRoundTripLatency(start time.Time) {
	latency := time.Since(start)
	m.mut.Lock()
	defer m.mut.Unlock()
	if latency > m.RoundTripLatency.Max {
		m.RoundTripLatency.Max = latency
	}
	if latency < m.RoundTripLatency.Min {
		m.RoundTripLatency.Min = latency
	}
	m.RoundTripLatency.Avg += latency
	m.FinishedReqs.Total++
}

func (m *Metrics) AddFinishedSuccessful() {
	m.mut.Lock()
	defer m.mut.Unlock()
	m.FinishedReqs.Succesful = m.FinishedReqs.Succesful + 1
}

func (m *Metrics) AddFinishedFailed() {
	m.mut.Lock()
	defer m.mut.Unlock()
	m.FinishedReqs.Failed++
}

func (m *Metrics) AddMsg() {
	m.mut.Lock()
	defer m.mut.Unlock()
	m.TotalNum++
}

func (m *Metrics) AddGoroutine(broadcastID uint64, name string) {
	m.mut.Lock()
	defer m.mut.Unlock()
	if name == "handler" {
		m.GoroutinesStarted++
	}
	index := fmt.Sprintf("%s.%v", name, broadcastID)
	m.Goroutines[index] = &goroutineMetric{
		started: time.Now(),
	}
}

func (m *Metrics) RemoveGoroutine(broadcastID uint64, name string) {
	m.mut.Lock()
	defer m.mut.Unlock()
	if name == "handler" {
		m.GoroutinesStopped++
	}
	index := fmt.Sprintf("%s.%v", name, broadcastID)
	if g, ok := m.Goroutines[index]; ok {
		g.ended = time.Now()
	} else {
		panic("what")
	}
}

func (m *Metrics) AddDropped(invalid bool) {
	m.mut.Lock()
	defer m.mut.Unlock()
	m.Dropped++
	if invalid {
		m.Invalid++
	} else {
		m.AlreadyProcessed++
	}
}

func (m *Metrics) AddProcessed() {
	m.mut.Lock()
	defer m.mut.Unlock()
	m.Processed++
}

func (m *Metrics) AddShardDistribution(i int) {
	m.mut.Lock()
	defer m.mut.Unlock()
	m.ShardDistribution[uint16(i)]++
}

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
	sendChan      chan Content
	ctx           context.Context
	cancelFunc    context.CancelFunc
	//sync.Once
}

type shardElement struct {
	id            int
	sendChan      chan Content
	broadcastChan chan Msg
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
	metrics             *Metrics

	shards [16]*shardElement
}

func NewState(logger *slog.Logger, router *BroadcastRouter, metrics *Metrics) *BroadcastState {
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
			id:            i,
			sendChan:      make(chan Content, shardBuffer),
			broadcastChan: make(chan Msg, shardBuffer),
			ctx:           ctx,
			cancelFunc:    cancel,
		}
		shards[i] = shard
	}
	return shards
}

func (s *BroadcastState) runShard(shard *shardElement, router *BroadcastRouter, reqTTL time.Duration, sendBuffer, shardBuffer int) {
	reqs := make(map[uint64]*reqContent, shardBuffer)
	for {
		select {
		case <-shard.ctx.Done():
			return
		case msg := <-shard.sendChan:
			if s.metrics != nil {
				s.metrics.AddShardDistribution(shard.id)
			}
			if req, ok := reqs[msg.BroadcastID]; ok {
				select {
				case <-req.ctx.Done():
					msg.ReceiveChan <- errors.New(fmt.Sprintf("req is done. broadcastID: %v", msg.BroadcastID))
				case req.sendChan <- msg:
				}
			} else {
				ctx, cancel := context.WithTimeout(s.parentCtx, reqTTL)
				req := &reqContent{
					ctx:        ctx,
					cancelFunc: cancel,
					// it is important to not buffer the channel. Otherwise,
					// it will cause a deadlock on the receiving channel. Will
					// happen in this scenario: A msg is queued/buffered but the
					// listening goroutine is stopped.
					sendChan:      make(chan Content),
					broadcastChan: make(chan Msg, sendBuffer),
				}
				reqs[msg.BroadcastID] = req
				go handleReq(router, msg.BroadcastID, req, msg, s.metrics)
				select {
				case <-req.ctx.Done():
					msg.ReceiveChan <- errors.New("req is done")
				case req.sendChan <- msg:
				}
			}
		case msg := <-shard.broadcastChan:
			if s.metrics != nil {
				s.metrics.AddShardDistribution(shard.id)
			}
			if req, ok := reqs[msg.BroadcastID]; ok {
				select {
				case <-req.ctx.Done():
				case req.broadcastChan <- msg:
				}
			}
		}
	}
}

func (s *BroadcastState) Process(msg Content) error {
	_, shardID, _, _ := DecodeBroadcastID(msg.BroadcastID)
	if shardID >= 16 {
		return errors.New("wrong shardID")
	}
	shard := s.shards[shardID]

	receiveChan := make(chan error)
	msg.ReceiveChan = receiveChan
	select {
	case <-shard.ctx.Done():
		return errors.New("shard is down")
	case shard.sendChan <- msg:
	}
	select {
	case <-shard.ctx.Done():
		return errors.New("shard is down")
	case err := <-receiveChan:
		return err
	}
}

func (s *BroadcastState) ProcessBroadcast(broadcastID uint64, req protoreflect.ProtoMessage, method string) {
	_, shardID, _, _ := DecodeBroadcastID(broadcastID)
	if shardID >= 16 {
		return
	}
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

func (s *BroadcastState) Prune() error {
	if s.logger != nil {
		s.logger.Debug("broadcast: pruned reqs")
	}
	s.parentCtxCancelFunc()
	return nil
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
