package gorums

import (
	"errors"
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
		router: newBroadcastRouter(logger),
		state:  newBroadcastStorage(logger),
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
	broadcastID := msg.broadcastID
	unlock, data, err := srv.state.lockRequest(broadcastID)
	if err != nil {
		return err
	}
	defer unlock()
	if data.hasBeenBroadcasted(msg.method) {
		return errors.New("already broadcasted")
	}
	if data.isDone() {
		return errors.New("request is done and handled")
	}
	err = srv.router.send(broadcastID, data, msg)
	if err != nil {
		return err
	}
	return srv.state.setBroadcasted(broadcastID, msg.method)
}

func (srv *broadcastServer) sendToClient(resp *reply) error {
	broadcastID := resp.getBroadcastID()
	unlock, data, err := srv.state.lockRequest(broadcastID)
	if err != nil {
		return err
	}
	defer unlock()
	if data.isDone() {
		return errors.New("request is done and handled")
	}
	err = srv.router.send(broadcastID, data, resp)
	if err != nil {
		srv.state.setShouldWaitForClient(broadcastID, resp)
		return err
	}
	return srv.state.remove(broadcastID)
}
