package gorums

import (
	"hash/fnv"
	"log/slog"
	"net"
	"sync"
)

type broadcastServer struct {
	propertiesMutex   sync.Mutex
	viewMutex         sync.RWMutex
	id                uint32
	addr              string
	view              RawConfiguration
	createBroadcaster func(m BroadcastMetadata, o *BroadcastOrchestrator) Broadcaster
	orchestrator      *BroadcastOrchestrator
	state             *BroadcastState
	router            *BroadcastRouter
	logger            *slog.Logger
}

func newBroadcastServer(logger *slog.Logger) *broadcastServer {
	return &broadcastServer{
		router: newBroadcastRouter(),
		state:  newBroadcastStorage(),
		logger: logger,
	}
}

func (srv *broadcastServer) addAddr(lis net.Listener) {
	srv.propertiesMutex.Lock()
	defer srv.propertiesMutex.Unlock()
	srv.addr = lis.Addr().String()
	h := fnv.New32a()
	_, _ = h.Write([]byte(srv.addr))
	srv.id = h.Sum32()
	srv.router.addAddr(srv.id, srv.addr)
}

func (srv *broadcastServer) broadcast(msg *broadcastMsg) error {
	// set the message as handled when returning from the method
	defer msg.setFinished()
	broadcastID := msg.broadcastID
	data, err := srv.state.get(broadcastID)
	if err != nil {
		return err
	}
	return srv.router.send(broadcastID, data, msg)
}

func (srv *broadcastServer) sendToClient(response *reply) error {
	srv.router.lock()
	defer srv.router.unlock()
	broadcastID := response.getBroadcastID()
	data, err := srv.state.get(broadcastID)
	if err != nil {
		return err
	}
	err = srv.router.send(broadcastID, data, response)
	if err != nil {
		srv.state.setShouldWaitForClient(broadcastID, response)
		return err
	}
	return srv.state.remove(broadcastID)
}

func (srv *broadcastServer) sendToClientHandler(broadcastID string, resp ResponseTypes, err error) {
	srv.sendToClient(newReply(resp, err, broadcastID))
}
