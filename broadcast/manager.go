package broadcast

import (
	"context"
	"github.com/relab/gorums/broadcast/dtos"
	"github.com/relab/gorums/broadcast/processor"
	"github.com/relab/gorums/broadcast/router"
	"github.com/relab/gorums/broadcast/shard"
	"log/slog"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type Manager struct {
	router router.Router
	logger *slog.Logger

	mut                 sync.Mutex
	shardMut            sync.RWMutex // RW because we often read and very seldom write to the state
	parentCtx           context.Context
	parentCtxCancelFunc context.CancelFunc
	reqTTL              time.Duration
	sendBuffer          int
	shardBuffer         int
	snowflake           *Snowflake
	order               map[string]int
	shards              []*shard.Shard
	numShards           uint16
}

type ManagerConfig struct {
	ID           uint32
	Addr         string
	MachineID    uint64
	Logger       *slog.Logger
	CreateClient func(addr string, dialOpts []grpc.DialOption) (*dtos.Client, error)
	Order        map[string]int
	DialTimeout  time.Duration
	ReqTTL       time.Duration
	ShardBuffer  int
	SendBuffer   int
	AllowList    map[string]string
	DialOpts     []grpc.DialOption
	NumShards    uint16
}

func NewBroadcastManager(config *ManagerConfig) *Manager {
	routerConfig := &router.Config{
		ID:           config.ID,
		Addr:         config.Addr,
		Logger:       config.Logger,
		CreateClient: config.CreateClient,
		DialTimeout:  config.DialTimeout,
		AllowList:    config.AllowList,
		DialOpts:     config.DialOpts,
	}
	r := router.NewRouter(routerConfig)
	ctx, cancel := context.WithCancel(context.Background())
	mgr := &Manager{
		router:              r,
		logger:              config.Logger,
		parentCtx:           ctx,
		parentCtxCancelFunc: cancel,
		reqTTL:              config.ReqTTL,
		sendBuffer:          config.SendBuffer,
		shardBuffer:         config.ShardBuffer,
		order:               config.Order,
		numShards:           config.NumShards,
		snowflake:           NewSnowflake(config.MachineID),
	}
	mgr.initShards()
	return mgr
}

func (m *Manager) initShards() {
	m.shards = make([]*shard.Shard, m.numShards)
	for i := uint16(0); i < m.numShards; i++ {
		shardConfig := &shard.Config{
			Id:          i,
			ParentCtx:   m.parentCtx,
			ShardBuffer: m.shardBuffer,
			SendBuffer:  m.sendBuffer,
			ReqTTL:      m.reqTTL,
			Router:      m.router,
			Order:       m.order,
			Logger:      m.logger,
		}
		m.shards[i] = shard.New(shardConfig)
	}
}

func (m *Manager) Process(msg *processor.RequestDto) {
	_, shardID, _, _ := DecodeBroadcastID(msg.BroadcastID)
	shardID = shardID % m.numShards
	s := m.shards[shardID]
	s.HandleMsg(msg)
}

func (m *Manager) Broadcast(broadcastID uint64, req protoreflect.ProtoMessage, method string, enqueueBroadcast processor.EnqueueMsg, opts ...dtos.BroadcastOptions) error {
	var options dtos.BroadcastOptions
	if len(opts) > 0 {
		options = opts[0]
	}
	return enqueueBroadcast(
		&dtos.BroadcastMsg{
			Info: dtos.Info{
				BroadcastID: broadcastID,
				Message:     req,
				Method:      method,
			},
			Options: options,
		},
	)
}

func (m *Manager) SendToClient(broadcastID uint64, resp protoreflect.ProtoMessage, err error, enqueueMsg processor.EnqueueMsg) error {
	return enqueueMsg(
		&dtos.ReplyMsg{
			Info: dtos.Info{
				BroadcastID: broadcastID,
				Message:     resp,
			},
			Err: err,
		},
	)
}

func (m *Manager) NewBroadcastID() uint64 {
	return m.snowflake.NewBroadcastID()
}

func (m *Manager) AddHandler(method string, handler any) {
	m.router.AddHandler(method, handler)
}

func (m *Manager) Close() error {
	m.mut.Lock()
	defer m.mut.Unlock()
	if m.logger != nil {
		m.logger.Debug("broadcast: closing state")
	}
	m.parentCtxCancelFunc()
	return m.router.Close()
}

func (m *Manager) ResetState() {
	m.parentCtxCancelFunc()
	m.mut.Lock()
	m.parentCtx, m.parentCtxCancelFunc = context.WithCancel(context.Background())
	m.initShards()
	m.shardMut.Unlock()
}
