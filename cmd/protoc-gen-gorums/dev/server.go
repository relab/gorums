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
	srv.ListenForBroadcast()
	return srv
}

type Broadcast struct {
	*gorums.BroadcastStruct
}

//
//func (b *Broadcast) PrePrepare(req *Request) {
//	b.SetBroadcastValues("protos.PBFTNode.PrePrepare", req)
//}
//
