package broadcast

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type Manager interface {
	Process(*Content) (context.Context, func(*Msg) error, error)
	Broadcast(uint64, protoreflect.ProtoMessage, string, func(*Msg) error, ...BroadcastOptions) error
	SendToClient(uint64, protoreflect.ProtoMessage, error, func(*Msg) error) error
	Cancel(uint64, []string, func(*Msg) error) error
	Done(uint64, func(*Msg) error)
	NewBroadcastID() uint64
	AddAddr(id uint32, addr string, machineID uint64)
	AddHandler(method string, handler any)
	Close() error
	ResetState()
}

type manager struct {
	state  *BroadcastState
	router *BroadcastRouter
	logger *slog.Logger
}

func NewBroadcastManager(logger *slog.Logger, createClient func(addr string, dialOpts []grpc.DialOption) (*Client, error), canceler func(broadcastID uint64, srvAddrs []string), order map[string]int, dialTimeout, reqTTL time.Duration, shardBuffer, sendBuffer int, allowList map[string]string, dialOpts ...grpc.DialOption) Manager {
	router := NewRouter(logger, createClient, canceler, dialTimeout, allowList, dialOpts...)
	state := NewState(logger, router, order, reqTTL, shardBuffer, sendBuffer)
	router.registerState(state)
	return &manager{
		state:  state,
		router: router,
		logger: logger,
	}
}

func (mgr *manager) Process(msg *Content) (context.Context, func(*Msg) error, error) {
	_, shardID, _, _ := DecodeBroadcastID(msg.BroadcastID)
	shardID = shardID % NumShards
	shard := mgr.state.shards[shardID]

	// we only need a single response
	receiveChan := make(chan shardResponse, 1)
	msg.ReceiveChan = receiveChan
	resp := shard.handleMsg(msg)
	return resp.reqCtx, resp.enqueueBroadcast, resp.err
}

func (mgr *manager) Broadcast(broadcastID uint64, req protoreflect.ProtoMessage, method string, enqueueBroadcast func(*Msg) error, opts ...BroadcastOptions) error {
	var options BroadcastOptions
	if len(opts) > 0 {
		options = opts[0]
	}
	msg := &Msg{
		MsgType:     BroadcastMsg,
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
	shard := mgr.state.getShard(shardID)
	shard.handleBMsg(msg)
	return nil
}

func (mgr *manager) SendToClient(broadcastID uint64, resp protoreflect.ProtoMessage, err error, enqueueBroadcast func(*Msg) error) error {
	msg := &Msg{
		MsgType: ReplyMsg,
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
	shard := mgr.state.getShard(shardID)
	shard.handleBMsg(msg)
	return nil
}

func (mgr *manager) Cancel(broadcastID uint64, srvAddrs []string, enqueueBroadcast func(*Msg) error) error {
	msg := &Msg{
		MsgType: CancellationMsg,
		Cancellation: &cancellation{
			end:      false, // should NOT stop the request
			srvAddrs: srvAddrs,
		},
		BroadcastID: broadcastID,
	}
	if enqueueBroadcast != nil {
		return enqueueBroadcast(msg)
	}
	_, shardID, _, _ := DecodeBroadcastID(broadcastID)
	shardID = shardID % NumShards
	shard := mgr.state.getShard(shardID)
	shard.handleBMsg(msg)
	return nil
}

func (mgr *manager) Done(broadcastID uint64, enqueueBroadcast func(*Msg) error) {
	msg := &Msg{
		MsgType: CancellationMsg,
		Cancellation: &cancellation{
			end: true, // should stop the request
		},
		BroadcastID: broadcastID,
	}
	if enqueueBroadcast != nil {
		// no need to check error because the processor
		// is stopped.
		_ = enqueueBroadcast(msg)
		return
	}
	_, shardID, _, _ := DecodeBroadcastID(broadcastID)
	shardID = shardID % NumShards
	shard := mgr.state.getShard(shardID)
	shard.handleBMsg(msg)
}

func (mgr *manager) NewBroadcastID() uint64 {
	return mgr.state.snowflake.NewBroadcastID()
}

func (mgr *manager) AddAddr(id uint32, addr string, machineID uint64) {
	mgr.router.id = id
	mgr.router.addr = addr
	mgr.state.snowflake = NewSnowflake(machineID)
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
