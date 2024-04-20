package gorums

import (
	"hash/fnv"
	"log/slog"
	"net"
	"sync"

	"github.com/relab/gorums/broadcast"
	"google.golang.org/protobuf/reflect/protoreflect"
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
	state             BroadcastState
	router            BroadcastRouter
	logger            *slog.Logger
	metrics           *metrics
}

func newBroadcastServer(logger *slog.Logger, withMetrics bool) *broadcastServer {
	var m *metrics = nil
	if withMetrics {
		m = &metrics{}
	}
	router := broadcast.NewRouter(logger)
	return &broadcastServer{
		router:  router,
		state:   broadcast.NewState(logger, router),
		logger:  logger,
		metrics: m,
	}
}

func (srv *broadcastServer) stop() {
	srv.state.Prune()
}

type BroadcastState interface {
	Prune() error
	Process(broadcast.Content2) error
	ProcessBroadcast(uint64, protoreflect.ProtoMessage, string)
	ProcessSendToClient(uint64, protoreflect.ProtoMessage, error)
}

type BroadcastRouter interface {
	Send(broadcastID uint64, addr, method string, msg any) error
	CreateConnection(addr string)
	AddAddr(id uint32, addr string)
	AddServerHandler(method string, handler broadcast.ServerHandler)
	AddClientHandler(method string, handler broadcast.ClientHandler)
}

type Snowflake interface {
	NewBroadcastID() uint64
}

func (srv *broadcastServer) addAddr(lis net.Listener) {
	srv.propertiesMutex.Lock()
	defer srv.propertiesMutex.Unlock()
	srv.addr = lis.Addr().String()
	h := fnv.New32a()
	_, _ = h.Write([]byte(srv.addr))
	srv.id = h.Sum32()
	srv.router.AddAddr(srv.id, srv.addr)
}
