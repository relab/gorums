package dev

import (
	"github.com/relab/gorums"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
)

type Server struct {
	*gorums.Server
}

func NewServer() *Server {
	srv := &Server{
		gorums.NewServer(),
	}
	b := &Broadcast{
		BroadcastStruct: gorums.NewBroadcastStruct(),
		sp:              gorums.NewSpBroadcastStruct(),
	}
	srv.RegisterBroadcastStruct(b, configureHandlers(b), configureMetadata(b))
	return srv
}

func (srv *Server) SetView(ownAddr string, srvAddrs []string, opts ...gorums.ManagerOption) error {
	err := srv.RegisterView(ownAddr, srvAddrs, opts...)
	srv.ListenForBroadcast()
	return err
}

type Broadcast struct {
	*gorums.BroadcastStruct
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

type clientResponse struct {
	broadcastID string
	data        protoreflect.ProtoMessage
}
