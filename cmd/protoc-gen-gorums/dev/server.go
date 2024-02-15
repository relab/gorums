package dev

import (
	sync "sync"

	"github.com/relab/gorums"
)

type Server struct {
	*gorums.Server
}

func NewServer() *Server {
	srv := &Server{
		gorums.NewServer(),
	}
	bd := &broadcastData{
		data: gorums.BroadcastOptions{},
	}
	b := &Broadcast{
		BroadcastStruct: gorums.NewBroadcastStruct(),
		sp:              gorums.NewSpBroadcastStruct(),
		bd:              bd,
	}
	bd.b = b
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
	sp       *gorums.SpBroadcast
	metadata gorums.BroadcastMetadata
	bd       *broadcastData
}

type broadcastData struct {
	mu   sync.Mutex
	data gorums.BroadcastOptions
	b    *Broadcast
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

func (b *Broadcast) GetMetadata() gorums.BroadcastMetadata {
	return b.metadata
}

func (b *Broadcast) Opts() *broadcastData {
	b.bd.mu.Lock()
	b.bd.data = gorums.BroadcastOptions{}
	return b.bd
}

func (b *broadcastData) To(srvAddrs ...string) *broadcastData {
	b.data.ServerAddresses = append(b.data.ServerAddresses, srvAddrs...)
	return b
}

func (b *broadcastData) OmitUniquenessChecks() *broadcastData {
	return b
}

func (b *broadcastData) SkipSelf() *broadcastData {
	return b
}

func (b *broadcastData) Gossip(percentage float32) *broadcastData {
	return b
}
