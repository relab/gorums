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
	start             time.Time
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
	ShardDistribution map[uint32]uint64
	// measures unique number of broadcastIDs processed simultaneounsly
	ConcurrencyDistribution timingMetric
}

type Metric struct {
	mut sync.Mutex
	m   Metrics
}

func NewMetric() *Metric {
	return &Metric{
		m: Metrics{
			start:             time.Now(),
			ShardDistribution: make(map[uint32]uint64, 16),
			Goroutines:        make(map[string]*goroutineMetric, 2000),
		},
	}
}

func (m *Metric) Reset() {
	m.mut.Lock()
	defer m.mut.Unlock()
	m.m.start = time.Now()
	m.m.TotalNum = 0
	m.m.GoroutinesStarted = 0
	m.m.GoroutinesStopped = 0
	m.m.Goroutines = make(map[string]*goroutineMetric, 2000)
	m.m.FinishedReqs.Total = 0
	m.m.FinishedReqs.Succesful = 0
	m.m.FinishedReqs.Failed = 0
	m.m.Processed = 0
	m.m.Dropped = 0
	m.m.Invalid = 0
	m.m.AlreadyProcessed = 0
	m.m.RoundTripLatency.Avg = 0
	m.m.RoundTripLatency.Min = 100 * time.Hour
	m.m.RoundTripLatency.Max = 0
	m.m.ReqLatency.Avg = 0
	m.m.ReqLatency.Min = 100 * time.Hour
	m.m.ReqLatency.Max = 0
	m.m.ShardDistribution = make(map[uint32]uint64, 16)
	// measures unique number of broadcastIDs processed simultaneounsly
	//m.ConcurrencyDistribution timingMetric
}

func (m Metrics) String() string {
	dur := time.Since(m.start)
	res := "Metrics:"
	res += "\n\t- TotalTime: " + fmt.Sprintf("%v", dur)
	if m.FinishedReqs.Total > 0 {
		res += "\n\t- ReqTime: " + fmt.Sprintf("%v", dur/time.Duration(m.FinishedReqs.Total))
	}
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

func (m *Metric) GetStats() Metrics {
	if m == nil {
		return Metrics{}
	}
	m.mut.Lock()
	defer m.mut.Unlock()
	if m.m.Processed > 0 {
		m.m.ReqLatency.Avg /= time.Duration(m.m.Processed)
	}
	if m.m.FinishedReqs.Total > 0 {
		m.m.RoundTripLatency.Avg /= time.Duration(m.m.FinishedReqs.Total)
	}
	return m.m
}

func (m *Metric) AddReqLatency(start time.Time) {
	latency := time.Since(start)
	m.mut.Lock()
	defer m.mut.Unlock()
	if latency > m.m.ReqLatency.Max {
		m.m.ReqLatency.Max = latency
	}
	if latency < m.m.ReqLatency.Min {
		m.m.ReqLatency.Min = latency
	}
	m.m.ReqLatency.Avg += latency
}

func (m *Metric) AddRoundTripLatency(start time.Time) {
	latency := time.Since(start)
	m.mut.Lock()
	defer m.mut.Unlock()
	if latency > m.m.RoundTripLatency.Max {
		m.m.RoundTripLatency.Max = latency
	}
	if latency < m.m.RoundTripLatency.Min {
		m.m.RoundTripLatency.Min = latency
	}
	m.m.RoundTripLatency.Avg += latency
	m.m.FinishedReqs.Total++
	if m.m.FinishedReqs.Total%50000 == 0 {
		fmt.Println("1000 done")
	}
}

func (m *Metric) AddFinishedSuccessful() {
	m.mut.Lock()
	defer m.mut.Unlock()
	m.m.FinishedReqs.Succesful = m.m.FinishedReqs.Succesful + 1
}

func (m *Metric) AddFinishedFailed() {
	m.mut.Lock()
	defer m.mut.Unlock()
	m.m.FinishedReqs.Failed++
}

func (m *Metric) AddMsg() {
	m.mut.Lock()
	defer m.mut.Unlock()
	m.m.TotalNum++
}

func (m *Metric) AddGoroutine(broadcastID uint64, name string) {
	m.mut.Lock()
	defer m.mut.Unlock()
	m.m.GoroutinesStarted++
	index := fmt.Sprintf("%s.%v", name, broadcastID)
	m.m.Goroutines[index] = &goroutineMetric{
		started: time.Now(),
	}
}

func (m *Metric) RemoveGoroutine(broadcastID uint64, name string) {
	m.mut.Lock()
	defer m.mut.Unlock()
	m.m.GoroutinesStopped++
	index := fmt.Sprintf("%s.%v", name, broadcastID)
	if g, ok := m.m.Goroutines[index]; ok {
		g.ended = time.Now()
		//} else {
		//panic("what")
	}
}

func (m *Metric) AddDropped(invalid bool) {
	m.mut.Lock()
	defer m.mut.Unlock()
	m.m.Dropped++
	if invalid {
		m.m.Invalid++
	} else {
		m.m.AlreadyProcessed++
	}
}

func (m *Metric) AddProcessed() {
	m.mut.Lock()
	defer m.mut.Unlock()
	m.m.Processed++
}

func (m *Metric) AddShardDistribution(i int) {
	m.mut.Lock()
	defer m.mut.Unlock()
	m.m.ShardDistribution[uint32(i)]++
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
	metrics             *Metric
	snowflake           *Snowflake

	shards [16]*shardElement
}

const NumShards = 16

func NewState(logger *slog.Logger, router *BroadcastRouter, metrics *Metric) *BroadcastState {
	shardBuffer := 100
	TTL := 5 * time.Second
	ctx, cancel := context.WithCancel(context.Background())
	shards := createShards(ctx, shardBuffer, NumShards)
	state := &BroadcastState{
		parentCtx:           ctx,
		parentCtxCancelFunc: cancel,
		shards:              shards,
		logger:              logger,
		reqTTL:              TTL,
		sendBuffer:          5,
		shardBuffer:         shardBuffer,
		metrics:             metrics,
		snowflake:           NewSnowflake(router.addr),
	}
	for i := 0; i < NumShards; i++ {
		go state.runShard(shards[i], router, state.reqTTL, state.sendBuffer, state.shardBuffer)
	}
	return state
}

func createShards(ctx context.Context, shardBuffer, numShards int) [16]*shardElement {
	var shards [16]*shardElement
	for i := 0; i < numShards; i++ {
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
				if !msg.IsBroadcastClient {
					// no need to send it to the broadcast request goroutine.
					//
					msg.ReceiveChan <- nil
					continue
				}
				select {
				case <-req.ctx.Done():
					msg.ReceiveChan <- fmt.Errorf("req is done. broadcastID: %v", msg.BroadcastID)
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
	shardID = shardID % NumShards
	//if shardID >= 16 {
	//return errors.New("wrong shardID")
	//}
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
	shardID = shardID % NumShards
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
	shardID = shardID % NumShards
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

func (s *BroadcastState) NewBroadcastID() uint64 {
	return s.snowflake.NewBroadcastID()
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
