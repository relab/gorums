package gorums

import (
	"fmt"
	"hash/fnv"
	"log/slog"
	"net"
	"sync"

	"github.com/relab/gorums/broadcast"
)

type broadcastServer struct {
	propertiesMutex   sync.Mutex
	viewMutex         sync.RWMutex
	id                uint32
	addr              string
	view              RawConfiguration
	createBroadcaster func(m BroadcastMetadata, o *BroadcastOrchestrator) Broadcaster
	orchestrator      *BroadcastOrchestrator
	manager           broadcast.Manager
	logger            *slog.Logger
	metrics           *broadcast.Metric
}

func (srv *Server) PrintStats() {
	fmt.Println(srv.broadcastSrv.metrics.GetStats())
	srv.broadcastSrv.metrics.Reset()
}

func (srv *Server) GetStats() broadcast.Metrics {
	//m := srv.broadcastSrv.metrics.GetStats()
	//srv.broadcastSrv.metrics.Reset()
	//return m
	return srv.broadcastSrv.manager.GetStats()
}

func newBroadcastServer(logger *slog.Logger, withMetrics bool) *broadcastServer {
	var m *broadcast.Metric = nil
	if withMetrics {
		//m = broadcast.NewMetric()
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
	srv.manager.AddAddr(srv.id, srv.addr)
}
