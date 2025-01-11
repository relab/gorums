package router

import "github.com/relab/gorums/broadcast/dtos"

type ServerHandler func(broadcastMsg *dtos.BroadcastMsg)

const (
	// ServerOriginAddr is special origin Addr used in creating a broadcast request from a server
	ServerOriginAddr string = "server"
)
