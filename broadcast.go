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
	//state             BroadcastState
	manager BroadcastManger
	//router            BroadcastRouter
	logger  *slog.Logger
	metrics *broadcast.Metric
}

func (srv *Server) PrintStats() {
	fmt.Println(srv.broadcastSrv.metrics.GetStats())
	srv.broadcastSrv.metrics.Reset()
}

func (srv *Server) GetStats() broadcast.Metrics {
	m := srv.broadcastSrv.metrics.GetStats()
	srv.broadcastSrv.metrics.Reset()
	return m
}

func newBroadcastServer(logger *slog.Logger, withMetrics bool) *broadcastServer {
	var m *broadcast.Metric = nil
	if withMetrics {
		m = broadcast.NewMetric()
	}
	return &broadcastServer{
		manager: broadcast.NewBroadcastManager(logger, m, createClient),
		logger:  logger,
		metrics: m,
	}
}

//func newBroadcastServer(logger *slog.Logger, withMetrics bool) *broadcastServer {
//var m *broadcast.Metric = nil
//if withMetrics {
//m = broadcast.NewMetric()
//}
//router := broadcast.NewRouter(logger, m, createClient)
//return &broadcastServer{
//router:  router,
//state:   broadcast.NewState(logger, router, m),
//logger:  logger,
//metrics: m,
//}
//}

func (srv *broadcastServer) stop() {
	//srv.state.Prune()
	srv.manager.Close()
}

//type BroadcastState interface {
//Prune() error
//Process(broadcast.Content) error
//ProcessBroadcast(uint64, protoreflect.ProtoMessage, string)
//ProcessSendToClient(uint64, protoreflect.ProtoMessage, error)
//NewBroadcastID() uint64
//}

//type BroadcastRouter interface {
//AddAddr(id uint32, addr string)
//AddServerHandler(method string, handler broadcast.ServerHandler)
//AddClientHandler(method string, handler broadcast.ClientHandler)
//}

type BroadcastManger interface {
	Process(broadcast.Content) error
	ProcessBroadcast(uint64, protoreflect.ProtoMessage, string)
	ProcessSendToClient(uint64, protoreflect.ProtoMessage, error)
	NewBroadcastID() uint64
	AddAddr(id uint32, addr string)
	AddServerHandler(method string, handler broadcast.ServerHandler)
	AddClientHandler(method string)
	Close() error
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
	//srv.router.AddAddr(srv.id, srv.addr)
	srv.manager.AddAddr(srv.id, srv.addr)
}
