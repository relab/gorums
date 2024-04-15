package gorums

import (
	"errors"
	"hash/fnv"
	"log/slog"
	"net"
	"sync"
	"time"
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
	router            IBroadcastRouter
	logger            *slog.Logger
}

func newBroadcastServer(logger *slog.Logger) *broadcastServer {
	return &broadcastServer{
		router: newBroadcastRouter(logger),
		state:  newBroadcastStorage(logger),
		logger: logger,
	}
}

func NewMachineID(addr string) uint16 {
	return 1
}

func NewBroadcastID(machineID uint16, sequenceNum uint32) uint64 {
	// timestamp: 32 bit -> seconds since 01.01.2024
	//bitMaskTimestamp := uint64((1<<32)-1) << 32
	// machineID: 12 bit -> 4096 clients
	bitMaskMachineID := uint64((1 << 12) - 1)
	// sequenceNum: 20 bit -> 1 048 576 messages
	bitMaskSequenceNum := uint64((1 << 20) - 1)

	t := uint64(time.Since(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)).Seconds()) << 32
	m := (uint64(machineID) & bitMaskMachineID) << 20
	s := uint64(sequenceNum) & bitMaskSequenceNum
	return t | m | s
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
	err = srv.router.SendOrg(broadcastID, data, msg)
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
	err = srv.router.SendOrg(broadcastID, data, resp)
	if err != nil {
		srv.state.setShouldWaitForClient(broadcastID, resp)
		return err
	}
	return srv.state.remove(broadcastID)
}
