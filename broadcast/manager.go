package broadcast

import (
	"context"
	"errors"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type Manager interface {
	Process(Content) (context.Context, func(Msg) error, error)
	Broadcast(uint64, protoreflect.ProtoMessage, string, func(Msg) error, ...BroadcastOptions) error
	SendToClient(uint64, protoreflect.ProtoMessage, error, func(Msg) error) error
	Cancel(uint64, []string) error
	Done(uint64)
	NewBroadcastID() uint64
	AddAddr(id uint32, addr string)
	AddHandler(method string, handler any)
	Close() error
	ResetState()
	GetStats() Metrics
}

type manager struct {
	state  *BroadcastState
	router *BroadcastRouter
	logger *slog.Logger
}

func NewBroadcastManager(logger *slog.Logger, createClient func(addr string, dialOpts []grpc.DialOption) (*Client, error), canceler func(broadcastID uint64, srvAddrs []string), order map[string]int) Manager {
	router := NewRouter(logger, createClient, canceler)
	state := NewState(logger, router, order)
	router.registerState(state)
	state.RunShards()
	return &manager{
		state:  state,
		router: router,
		logger: logger,
	}
}

func (mgr *manager) Process(msg Content) (context.Context, func(Msg) error, error) {
	_, shardID, _, _ := DecodeBroadcastID(msg.BroadcastID)
	shardID = shardID % NumShards
	shard := mgr.state.shards[shardID]

	// we only need a single response
	receiveChan := make(chan shardResponse, 1)
	msg.ReceiveChan = receiveChan
	select {
	case <-shard.ctx.Done():
		return nil, nil, errors.New("shard is down")
	case shard.sendChan <- msg:
	}
	select {
	case <-shard.ctx.Done():
		return nil, nil, errors.New("shard is down")
	case resp := <-receiveChan:
		return resp.reqCtx, resp.enqueueBroadcast, resp.err
	}
}

func (mgr *manager) Broadcast(broadcastID uint64, req protoreflect.ProtoMessage, method string, enqueueBroadcast func(Msg) error, opts ...BroadcastOptions) error {
	var options BroadcastOptions
	if len(opts) > 0 {
		options = opts[0]
	}
	msg := Msg{
		Broadcast:   true,
		Msg:         NewMsg(broadcastID, req, method, options),
		Method:      method,
		BroadcastID: broadcastID,
	}
	// fast path: communicate directly with the broadcast request
	if enqueueBroadcast != nil {
		return enqueueBroadcast(msg)
	}
	// slow path: communicate with the shard first
	_, shardID, _, _ := DecodeBroadcastID(broadcastID)
	shardID = shardID % NumShards
	shard := mgr.state.shards[shardID]
	select {
	case shard.broadcastChan <- msg:
		return nil
	case <-shard.ctx.Done():
		return ShardDownErr{}
	}
}

func (mgr *manager) SendToClient(broadcastID uint64, resp protoreflect.ProtoMessage, err error, enqueueBroadcast func(Msg) error) error {
	msg := Msg{
		Reply: &reply{
			Response: resp,
			Err:      err,
		},
		BroadcastID: broadcastID,
	}
	// fast path: communicate directly with the broadcast request
	if enqueueBroadcast != nil {
		return enqueueBroadcast(msg)
	}
	// slow path: communicate with the shard first
	_, shardID, _, _ := DecodeBroadcastID(broadcastID)
	shardID = shardID % NumShards
	shard := mgr.state.shards[shardID]
	select {
	case shard.broadcastChan <- msg:
		return nil
	case <-shard.ctx.Done():
		return ShardDownErr{}
	}
}

func (mgr *manager) Cancel(broadcastID uint64, srvAddrs []string) error {
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
		return nil
	case <-shard.ctx.Done():
		return ShardDownErr{}
	}
}

func (mgr *manager) Done(broadcastID uint64) {
	_, shardID, _, _ := DecodeBroadcastID(broadcastID)
	shardID = shardID % NumShards
	shard := mgr.state.shards[shardID]
	select {
	case shard.broadcastChan <- Msg{
		Cancellation: &cancellation{
			end: true,
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
	m := mgr.state.getStats()
	return Metrics{
		TotalNum: uint64(m.totalMsgs),
		Dropped:  m.droppedMsgs,
		FinishedReqs: struct {
			Total     uint64
			Succesful uint64
			Failed    uint64
		}{
			Total:     m.numReqs,
			Succesful: m.finishedReqs,
			Failed:    m.numReqs - m.finishedReqs,
		},
	}
}
