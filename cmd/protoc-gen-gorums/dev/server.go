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
	bd       *broadcastData
}

type broadcastData struct {
	mu   sync.Mutex
	data gorums.BroadcastOptions
	b    *Broadcast
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

// Allows to configure the broadcast request.
// A broadcast method must be called before specifying this option again.
func (b *Broadcast) Opts() *broadcastData {
	b.bd.mu.Lock()
	b.bd.data = gorums.BroadcastOptions{}
	return b.bd
}

// The broadcast call will only be sent to the specified addresses.
//
// Note: It will only broadcast to addresses that are a subset of
// the servers in the server configuration.
func (b *broadcastData) To(srvAddrs ...string) *broadcastData {
	b.data.ServerAddresses = append(b.data.ServerAddresses, srvAddrs...)
	return b
}

// Will remove all uniqueness checks done before broadcasting.
//
// Note: This could result in infinite recursion/loops. Be careful when using this.
func (b *broadcastData) OmitUniquenessChecks() *broadcastData {
	b.data.OmitUniquenessChecks = true
	return b
}

// The broadcast will not be sent to itself.
func (b *broadcastData) SkipSelf() *broadcastData {
	b.data.SkipSelf = true
	return b
}

// A subset of the nodes in the configuration corresponding to the given
// percentage will be selected at random.
func (b *broadcastData) Gossip(percentage float32) *broadcastData {
	b.data.GossipPercentage = percentage
	return b
}
