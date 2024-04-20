package gorums

import (
	"fmt"
	"hash/fnv"
	"log/slog"
	"net"
	"sync"

	"github.com/relab/gorums/broadcast"
	"google.golang.org/protobuf/reflect/protoreflect"
)

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
	metrics           *broadcast.Metrics
}

func (srv *Server) PrintStats() {
	fmt.Println(srv.broadcastSrv.metrics.GetStats())
	srv.broadcastSrv.metrics.Reset()
}

func newBroadcastServer(logger *slog.Logger, withMetrics bool) *broadcastServer {
	var m *broadcast.Metrics = nil
	if withMetrics {
		m = &broadcast.Metrics{
			ShardDistribution: make(map[uint16]uint64),
		}
	}
	router := broadcast.NewRouter(logger, m)
	return &broadcastServer{
		router:  router,
		state:   broadcast.NewState(logger, router, m),
		logger:  logger,
		metrics: m,
	}
}

func (srv *broadcastServer) stop() {
	srv.state.Prune()
}

type BroadcastState interface {
	Prune() error
	Process(broadcast.Content) error
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
