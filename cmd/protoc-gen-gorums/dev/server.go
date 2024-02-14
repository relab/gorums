package dev

import (
	"github.com/relab/gorums"
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
	srv.RegisterBroadcastStruct(b, assign(b), assignValues(b))
	return srv
}

func (srv *Server) RegisterConfiguration(ownAddr string, srvAddrs []string, opts ...gorums.ManagerOption) error {
	err := srv.RegisterConfig(ownAddr, srvAddrs, opts...)
	srv.ListenForBroadcast()
	return err
}

type Broadcast struct {
	*gorums.BroadcastStruct
	sp              *gorums.SpBroadcast
	serverAddresses []string
	metadata        gorums.BroadcastMetadata
}

func assign(b *Broadcast) func(bh gorums.BroadcastHandlerFunc, ch gorums.BroadcastReturnToClientHandlerFunc) {
	return func(bh gorums.BroadcastHandlerFunc, ch gorums.BroadcastReturnToClientHandlerFunc) {
		b.sp.BroadcastHandler = bh
		b.sp.ReturnToClientHandler = ch
	}
}

func assignValues(b *Broadcast) func(metadata gorums.BroadcastMetadata) {
	return func(metadata gorums.BroadcastMetadata) {
		b.metadata = metadata
	}
}

func (b *Broadcast) To(srvAddrs ...string) *Broadcast {
	b.serverAddresses = append(b.serverAddresses, srvAddrs...)
	return b
}

func (b *Broadcast) OmitUniquenessChecks() *Broadcast {
	return b
}

func (b *Broadcast) Gossip(percentage float32) *Broadcast {
	return b
}

func (b *Broadcast) GetMetadata() gorums.BroadcastMetadata {
	return b.metadata
}
