package dev

import (
	"fmt"

	"github.com/relab/gorums"
	grpc "google.golang.org/grpc"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
)

type Server struct {
	*gorums.Server
	broadcast *Broadcast
	View      *Configuration
}

func NewServer(opts ...gorums.ServerOption) *Server {
	srv := &Server{
		Server: gorums.NewServer(opts...),
	}
	b := &Broadcast{
		orchestrator: gorums.NewBroadcastOrchestrator(srv.Server),
	}
	srv.broadcast = b
	srv.RegisterBroadcaster(newBroadcaster)
	return srv
}

func newBroadcaster(m gorums.BroadcastMetadata, o *gorums.BroadcastOrchestrator) gorums.Broadcaster {
	return &Broadcast{
		orchestrator: o,
		metadata:     m,
		srvAddrs:     make([]string, 0),
	}
}

func (srv *Server) SetView(config *Configuration) {
	srv.View = config
	srv.RegisterConfig(config.RawConfiguration)
}

type Broadcast struct {
	orchestrator *gorums.BroadcastOrchestrator
	metadata     gorums.BroadcastMetadata
	srvAddrs     []string
}

// Returns a readonly struct of the metadata used in the broadcast.
//
// Note: Some of the data are equal across the cluster, such as BroadcastID.
// Other fields are local, such as SenderAddr.
func (b *Broadcast) GetMetadata() gorums.BroadcastMetadata {
	return b.metadata
}

type clientServerImpl struct {
	*gorums.ClientServer
	grpcServer *grpc.Server
}

func (c *clientServerImpl) stop() {
	c.ClientServer.Stop()
	if c.grpcServer != nil {
		c.grpcServer.Stop()
	}
}

func (b *Broadcast) To(addrs ...string) *Broadcast {
	if len(addrs) <= 0 {
		return b
	}
	b.srvAddrs = append(b.srvAddrs, addrs...)
	return b
}

func (b *Broadcast) Forward(req protoreflect.ProtoMessage, addr string) error {
	if addr == "" {
		return fmt.Errorf("cannot forward to empty addr, got: %s", addr)
	}
	if !b.metadata.IsBroadcastClient {
		return fmt.Errorf("can only forward client requests")
	}
	go b.orchestrator.ForwardHandler(req, b.metadata.OriginMethod, b.metadata.BroadcastID, addr, b.metadata.OriginAddr)
	return nil
}

func (b *Broadcast) SendToClient(resp protoreflect.ProtoMessage, err error) {
	b.orchestrator.SendToClientHandler(b.metadata.BroadcastID, resp, err)
}

func (srv *Server) SendToClient(resp protoreflect.ProtoMessage, err error, broadcastID uint64) {
	srv.SendToClientHandler(resp, err, broadcastID)
}
