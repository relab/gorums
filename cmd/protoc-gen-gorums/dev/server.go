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
	srv.RegisterBroadcastStruct(&Broadcast{gorums.NewBroadcastStruct()})
	return srv
}

func (srv *Server) RegisterConfiguration(srvAddrs []string, opts ...gorums.ManagerOption) error {
	err := srv.RegisterConfig(srvAddrs, opts...)
	srv.ListenForBroadcast()
	return err
}

type Broadcast struct {
	*gorums.BroadcastStruct
}
