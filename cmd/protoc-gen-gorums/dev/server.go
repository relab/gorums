package dev

import (
	"net"

	"github.com/relab/gorums"
	grpc "google.golang.org/grpc"
)

type Server struct {
	*gorums.Server
	View *Configuration
}

func NewServer() *Server {
	srv := &Server{
		Server: gorums.NewServer(),
	}
	b := &Broadcast{
		Broadcaster: gorums.NewBroadcaster(),
		sp:          gorums.NewSpBroadcastStruct(),
	}
	srv.RegisterBroadcastStruct(b, configureHandlers(b), configureMetadata(b))
	return srv
}

func (srv *Server) SetView(config *Configuration) {
	srv.View = config
	srv.RegisterConfig(config.RawConfiguration)
	srv.ListenForBroadcast()
}

type Broadcast struct {
	*gorums.Broadcaster
	sp       *gorums.SpBroadcast
	metadata gorums.BroadcastMetadata
}

func configureHandlers(b *Broadcast) func(bh gorums.BroadcastHandlerFunc, ch gorums.BroadcastReturnToClientHandlerFunc) {
	return func(bh gorums.BroadcastHandlerFunc, ch gorums.BroadcastReturnToClientHandlerFunc) {
		b.sp.BroadcastHandler = bh
		b.sp.ReturnToClientHandler = ch
	}
}

func configureMetadata(b *Broadcast) func(metadata gorums.BroadcastMetadata) {
	return func(metadata gorums.BroadcastMetadata) {
		b.metadata = metadata
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
