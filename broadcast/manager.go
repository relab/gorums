package broadcast

import (
	"errors"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type BroadcastManager interface {
	Process(Content) error
	ProcessBroadcast(uint64, protoreflect.ProtoMessage, string, ...BroadcastOptions)
	ProcessSendToClient(uint64, protoreflect.ProtoMessage, error)
	NewBroadcastID() uint64
	AddAddr(id uint32, addr string)
	AddServerHandler(method string, handler ServerHandler)
	AddClientHandler(method string)
	Close() error
	GetStats() Metrics
}

type broadcastManager struct {
	state   *BroadcastState
	router  *BroadcastRouter
	metrics *Metric
	logger  *slog.Logger
}

func NewBroadcastManager(logger *slog.Logger, m *Metric, createClient func(addr string, dialOpts []grpc.DialOption) (*Client, error)) BroadcastManager {
	router := NewRouter(logger, m, createClient)
	state := NewState(logger, m)
	for _, shard := range state.shards {
		go shard.run(router, state.reqTTL, state.sendBuffer, m)
	}
	return &broadcastManager{
		state:   state,
		router:  router,
		logger:  logger,
		metrics: m,
	}
}

func (mgr *broadcastManager) Process(msg Content) error {
	_, shardID, _, _ := DecodeBroadcastID(msg.BroadcastID)
	shardID = shardID % NumShards
	shard := mgr.state.shards[shardID]

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

func (mgr *broadcastManager) ProcessBroadcast(broadcastID uint64, req protoreflect.ProtoMessage, method string, opts ...BroadcastOptions) {
	var options BroadcastOptions
	if len(opts) > 0 {
		options = opts[0]
	}
	_, shardID, _, _ := DecodeBroadcastID(broadcastID)
	shardID = shardID % NumShards
	shard := mgr.state.shards[shardID]
	select {
	case shard.broadcastChan <- Msg{
		Broadcast:   true,
		Msg:         NewMsg(broadcastID, req, method, options),
		Method:      method,
		BroadcastID: broadcastID,
	}:
	case <-shard.ctx.Done():
	}
}

func (mgr *broadcastManager) ProcessSendToClient(broadcastID uint64, resp protoreflect.ProtoMessage, err error) {
	_, shardID, _, _ := DecodeBroadcastID(broadcastID)
	shardID = shardID % NumShards
	shard := mgr.state.shards[shardID]
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

func (mgr *broadcastManager) NewBroadcastID() uint64 {
	return mgr.state.snowflake.NewBroadcastID()
}

func (mgr *broadcastManager) AddAddr(id uint32, addr string) {
	mgr.router.id = id
	mgr.router.addr = addr
	mgr.state.snowflake = NewSnowflake(addr)
}

func (mgr *broadcastManager) AddServerHandler(method string, handler ServerHandler) {
	mgr.router.serverHandlers[method] = handler
}

func (mgr *broadcastManager) AddClientHandler(method string) {
	// only needs to know whether the handler exists. routing is done
	// client-side using the provided metadata in the request.
	mgr.router.clientHandlers[method] = struct{}{}
}

func (mgr *broadcastManager) Close() error {
	return mgr.state.Close()
}

func (mgr *broadcastManager) GetStats() Metrics {
	time.Sleep(5 * time.Second)
	m := mgr.state.getStats()
	return Metrics{
		TotalNum: uint64(m.totalMsgs),
		FinishedReqs: struct {
			Total     uint64
			Succesful uint64
			Failed    uint64
		}{
			Total:     m.numReqs,
			Succesful: m.finishedReqs,
			Failed:    m.numReqs - m.finishedReqs,
		},
		RoundTripLatency: timingMetric{
			Avg: m.avgLifetime,
			Max: m.maxLifetime,
			Min: m.minLifetime,
		},
	}
}
