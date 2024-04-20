package gorums

import (
	"errors"
	"hash/fnv"
	"log/slog"
	"math/rand"
	"net"
	"sync"
	"time"
)

type timingMetric struct {
	Avg    uint64
	Median uint64
	Min    uint64
	Max    uint64
}

type metrics struct {
	mut               sync.Mutex
	TotalNum          uint64
	Processed         uint64
	RoundTripLatency  timingMetric
	ReqLatency        timingMetric
	ShardDistribution map[int]int
	// measures unique number of broadcastIDs processed simultaneounsly
	ConcurrencyDistribution timingMetric
}

func (m *metrics) AddMsg() {
	m.mut.Lock()
	defer m.mut.Unlock()
	m.TotalNum++
}

func (m *metrics) AddProcessed() {
	m.mut.Lock()
	defer m.mut.Unlock()
	m.Processed++
}

type broadcastServer struct {
	propertiesMutex   sync.Mutex
	viewMutex         sync.RWMutex
	id                uint32
	addr              string
	view              RawConfiguration
	createBroadcaster func(m BroadcastMetadata, o *BroadcastOrchestrator) Broadcaster
	orchestrator      *BroadcastOrchestrator
	state             *BroadcastState
	router            IBroadcastRouter
	logger            *slog.Logger
	metrics           *metrics
}

func newBroadcastServer(logger *slog.Logger, withMetrics bool) *broadcastServer {
	var m *metrics = nil
	if withMetrics {
		m = &metrics{}
	}
	router := newBroadcastRouter(logger)
	return &broadcastServer{
		router:  router,
		state:   newBroadcastStorage(logger, router),
		logger:  logger,
		metrics: m,
	}
}

func (srv *broadcastServer) stop() {
	srv.state.prune()
}

type Snowflake interface {
	NewBroadcastID() uint64
}

type snowflake struct {
	mut         sync.Mutex
	machineID   uint64
	sequenceNum uint64
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

func NewSnowflake(addr string) *snowflake {
	timestamp, _ := time.Parse("2006-01-02T15:04:05", epoch)
	return &snowflake{
		machineID:   uint64(rand.Int31n(int32(maxMachineID))),
		sequenceNum: 0,
		epoch:       timestamp,
		//sequenceNum: uint32(maxSequenceNum * rand.Float32()),
	}
}

func (s *snowflake) NewBroadcastID() uint64 {
	// timestamp: 30 bit -> seconds since 01.01.2024
	// shardID: 4 bit -> 16 different shards
	// machineID: 12 bit -> 4096 clients
	// sequenceNum: 18 bit -> 262 144 messages
start:
	s.mut.Lock()
	timestamp := uint64(time.Since(s.epoch).Seconds())
	l := (s.sequenceNum + 1) % uint64(maxSequenceNum)
	if timestamp-s.lastT <= 0 && l == s.lastS {
		s.mut.Unlock()
		time.Sleep(10 * time.Millisecond)
		goto start
	}
	if timestamp > s.lastT {
		s.lastT = timestamp
		s.lastS = l
	}
	s.sequenceNum = l
	s.mut.Unlock()

	t := (timestamp << 34) & bitMaskTimestamp
	shard := (uint64(rand.Int31n(int32(maxShard))) << 30) & bitMaskShardID
	m := uint64(s.machineID<<18) & bitMaskMachineID
	n := l & bitMaskSequenceNum
	return t | shard | m | n
}

func decodeBroadcastID(broadcastID uint64) (uint32, uint16, uint16, uint32) {
	t := (broadcastID & bitMaskTimestamp) >> 34
	shard := (broadcastID & bitMaskShardID) >> 30
	m := (broadcastID & bitMaskMachineID) >> 18
	n := (broadcastID & bitMaskSequenceNum)
	return uint32(t), uint16(shard), uint16(m), uint32(n)
}

//func NewMachineID(addr string) uint16 {
//return uint16(maxMachineID * rand.Float32())
//}

//func NewBroadcastID(machineID uint16, sequenceNum uint32) uint64 {
//// timestamp: 30 bit -> seconds since 01.01.2024
//// shardID: 4 bit -> 16 different shards
//// machineID: 12 bit -> 4096 clients
//bitMaskMachineID := uint64(maxMachineID - 1)
//// sequenceNum: 18 bit -> 262 144 messages
//bitMaskSequenceNum := uint64(maxSequenceNum - 1)

//t := uint64(time.Since(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)).Seconds()) << 34
//s := uint64(16*rand.Float32()) << 30
//m := (uint64(machineID) & bitMaskMachineID) << 18
//n := uint64(sequenceNum) & bitMaskSequenceNum
//return t | s | m | n
//}

func (srv *broadcastServer) addAddr(lis net.Listener) {
	srv.propertiesMutex.Lock()
	defer srv.propertiesMutex.Unlock()
	srv.addr = lis.Addr().String()
	h := fnv.New32a()
	_, _ = h.Write([]byte(srv.addr))
	srv.id = h.Sum32()
	srv.router.AddAddr(srv.id, srv.addr)
}

func (srv *broadcastServer) broadcast(msg *broadcastMsg) error {
	// set the message as handled when returning from the method
	broadcastID := msg.broadcastID
	unlock, data, err := srv.state.lockRequest(broadcastID)
	if err != nil {
		return err
	}
	defer unlock()
	if data.hasBeenBroadcasted(msg.method) {
		return errors.New("already broadcasted")
	}
	if data.isDone() {
		return errors.New("request is done and handled")
	}
	err = srv.router.SendOrg(broadcastID, data, msg)
	if err != nil {
		return err
	}
	return srv.state.setBroadcasted(broadcastID, msg.method)
}

func (srv *broadcastServer) sendToClient(resp *reply) error {
	broadcastID := resp.getBroadcastID()
	unlock, data, err := srv.state.lockRequest(broadcastID)
	if err != nil {
		return err
	}
	defer unlock()
	if data.isDone() {
		return errors.New("request is done and handled")
	}
	err = srv.router.SendOrg(broadcastID, data, resp)
	if err != nil {
		srv.state.setShouldWaitForClient(broadcastID, resp)
		return err
	}
	return srv.state.remove(broadcastID)
}
