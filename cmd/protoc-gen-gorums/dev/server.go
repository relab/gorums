package dev

import (
	"net"

	"github.com/relab/gorums"
	grpc "google.golang.org/grpc"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
)

type Server struct {
	*gorums.Server
	broadcast *Broadcast
	View      *Configuration
}

func NewServer() *Server {
	srv := &Server{
		Server: gorums.NewServer(),
	}
	b := &Broadcast{
		Broadcaster:  gorums.NewBroadcaster(),
		orchestrator: gorums.NewBroadcastOrchestrator(),
		metadata:     gorums.BroadcastMetadata{},
	}
	srv.broadcast = b
	set, reset := configureMetadata(b)
	srv.RegisterBroadcaster(b, configureHandlers(b), set, reset)
	return srv
}

func (srv *Server) SetView(config *Configuration) {
	srv.View = config
	srv.RegisterConfig(config.RawConfiguration)
	srv.ListenForBroadcast()
}

type Broadcast struct {
	*gorums.Broadcaster
	orchestrator *gorums.BroadcastOrchestrator
	metadata     gorums.BroadcastMetadata
}

func configureHandlers(b *Broadcast) func(bh gorums.BroadcastHandlerFunc, ch gorums.BroadcastSendToClientHandlerFunc) {
	return func(bh gorums.BroadcastHandlerFunc, ch gorums.BroadcastSendToClientHandlerFunc) {
		b.orchestrator.BroadcastHandler = bh
		b.orchestrator.SendToClientHandler = ch
	}
}

func configureMetadata(b *Broadcast) (func(metadata gorums.BroadcastMetadata), func()) {
	return func(metadata gorums.BroadcastMetadata) {
			b.metadata = metadata
		}, func() {
			b.metadata = gorums.BroadcastMetadata{}
		}
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

func (c *Configuration) RegisterClientServer(lis net.Listener, opts ...grpc.ServerOption) error {
	srvImpl := &clientServerImpl{
		grpcServer: grpc.NewServer(opts...),
	}
	srv, err := gorums.NewClientServer(lis)
	if err != nil {
		return err
	}
	srvImpl.grpcServer.RegisterService(&clientServer_ServiceDesc, srvImpl)
	go srvImpl.grpcServer.Serve(lis)
	srvImpl.ClientServer = srv
	c.srv = srvImpl
	return nil
}

func (b *Broadcast) SendToClient(resp protoreflect.ProtoMessage, err error) {
	b.orchestrator.SendToClientHandler(b.metadata.BroadcastID, resp, err)
}

func (srv *Server) SendToClient(resp protoreflect.ProtoMessage, err error, broadcastID string) {
	srv.RetToClient(resp, err, broadcastID)
}
