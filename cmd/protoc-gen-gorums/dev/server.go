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

type Broadcast struct {
	*gorums.BroadcastStruct
}
