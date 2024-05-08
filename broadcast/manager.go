package broadcast

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type Manager interface {
	Process(Content) (error, context.Context)
	Broadcast(uint64, protoreflect.ProtoMessage, string, ...BroadcastOptions)
	SendToClient(uint64, protoreflect.ProtoMessage, error)
	Cancel(uint64, []string)
	NewBroadcastID() uint64
	AddAddr(id uint32, addr string)
	AddHandler(method string, handler any)
	Close() error
	ResetState()
	GetStats() Metrics
}

type manager struct {
	state   *BroadcastState
	router  *BroadcastRouter
	metrics *Metric
	logger  *slog.Logger
}

func NewBroadcastManager(logger *slog.Logger, m *Metric, createClient func(addr string, dialOpts []grpc.DialOption) (*Client, error), canceler func(broadcastID uint64, srvAddrs []string)) Manager {
	router := NewRouter(logger, m, createClient, canceler)
	state := NewState(logger, m, router)
	router.registerState(state)
	state.RunShards()
	return &manager{
		state:   state,
		router:  router,
		logger:  logger,
		metrics: m,
	}
}

func (mgr *manager) Process(msg Content) (error, context.Context) {
	_, shardID, _, _ := DecodeBroadcastID(msg.BroadcastID)
	shardID = shardID % NumShards
	shard := mgr.state.shards[shardID]

	// we only need a single response
	receiveChan := make(chan shardResponse, 1)
	msg.ReceiveChan = receiveChan
	select {
	case <-shard.ctx.Done():
		return errors.New("shard is down"), nil
	case shard.sendChan <- msg:
	}
	select {
	case <-shard.ctx.Done():
		return errors.New("shard is down"), nil
	case resp := <-receiveChan:
		return resp.err, resp.reqCtx
	}
}

func (mgr *manager) Broadcast(broadcastID uint64, req protoreflect.ProtoMessage, method string, opts ...BroadcastOptions) {
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

func (mgr *manager) SendToClient(broadcastID uint64, resp protoreflect.ProtoMessage, err error) {
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

func (mgr *manager) Cancel(broadcastID uint64, srvAddrs []string) {
	_, shardID, _, _ := DecodeBroadcastID(broadcastID)
	shardID = shardID % NumShards
	shard := mgr.state.shards[shardID]
	select {
	case shard.broadcastChan <- Msg{
		Cancellation: &cancellation{
			srvAddrs: srvAddrs,
		},
		BroadcastID: broadcastID,
	}:
	case <-shard.ctx.Done():
	}
}

func (mgr *manager) NewBroadcastID() uint64 {
	return mgr.state.snowflake.NewBroadcastID()
}

func (mgr *manager) AddAddr(id uint32, addr string) {
	mgr.router.id = id
	mgr.router.addr = addr
	mgr.state.snowflake = NewSnowflake(addr)
}

func (mgr *manager) AddHandler(method string, handler any) {
	switch h := handler.(type) {
	case ServerHandler:
		mgr.router.serverHandlers[method] = h
	default:
		// only needs to know whether the handler exists. routing is done
		// client-side using the provided metadata in the request.
		mgr.router.clientHandlers[method] = struct{}{}
	}
}

func (mgr *manager) Close() error {
	return mgr.state.Close()
}

func (mgr *manager) ResetState() {
	mgr.state.reset()
}

func (mgr *manager) GetStats() Metrics {
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
