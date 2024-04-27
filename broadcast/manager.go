package broadcast

import (
	"errors"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type BroadcastManager struct {
	state   *BroadcastState
	router  *BroadcastRouter
	metrics *Metric
	logger  *slog.Logger
}

func NewBroadcastManager(logger *slog.Logger, m *Metric, createClient func(addr string, dialOpts []grpc.DialOption) (*Client, error)) *BroadcastManager {
	router := NewRouter(logger, m, createClient)
	state := NewState(logger, m)
	for _, shard := range state.shards {
		go shard.run(router, state.reqTTL, state.sendBuffer, state.shardBuffer, m)
	}
	return &BroadcastManager{
		state:   state,
		router:  router,
		logger:  logger,
		metrics: m,
	}
}

func (mgr *BroadcastManager) Process(msg Content) error {
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

func (mgr *BroadcastManager) ProcessBroadcast(broadcastID uint64, req protoreflect.ProtoMessage, method string) {
	_, shardID, _, _ := DecodeBroadcastID(broadcastID)
	shardID = shardID % NumShards
	shard := mgr.state.shards[shardID]
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

func (mgr *BroadcastManager) ProcessSendToClient(broadcastID uint64, resp protoreflect.ProtoMessage, err error) {
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

func (mgr *BroadcastManager) NewBroadcastID() uint64 {
	return mgr.state.snowflake.NewBroadcastID()
}

func (mgr *BroadcastManager) AddAddr(id uint32, addr string) {
	mgr.router.id = id
	mgr.router.addr = addr
	mgr.state.snowflake = NewSnowflake(addr)
}

func (mgr *BroadcastManager) AddServerHandler(method string, handler ServerHandler) {
	mgr.router.serverHandlers[method] = handler
}

func (mgr *BroadcastManager) AddClientHandler(method string) {
	mgr.router.clientHandlers[method] = struct{}{}
}

func (mgr *BroadcastManager) Close() error {
	return mgr.state.Close()
}
