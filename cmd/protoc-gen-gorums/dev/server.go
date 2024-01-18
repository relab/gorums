package dev

import (
	context "context"
	"time"

	"github.com/relab/gorums"
)

type Server struct {
	*gorums.Server
	methods map[string]func(ctx context.Context, req any) (any, error)
}

func NewServer(id string) *Server {
	return &Server{
		Server:  gorums.NewServer(),
		methods: make(map[string]func(ctx context.Context, req any) (resp any, err error)),
	}
}

func (srv *Server) RegisterQCStorageServer(impl ZorumsService) {
	srv.RegisterHandler("dev.ZorumsService.QuorumCall", gorums.DefaultHandler(impl.QuorumCall))
	srv.RegisterHandler("dev.ZorumsService.QuorumCallAllToAll", gorums.BestEffortBroadcastHandler(impl.QuorumCall, srv.Server))
}

func (srv *Server) RegisterConfiguration(c *Configuration) {
	srv.methods["dev.ZorumsService.QuorumCall"] = gorums.RegisterBroadcastFunc(c.QuorumCall)
	go srv.run()
}

func (srv *Server) run() {
	for msg := range srv.BroadcastChan {
		req := msg.GetRequest()
		method := msg.GetMethod()
		//ctx := msg.GetContext() <- old context, will be cancelled by calling client
		time.Sleep(5 * time.Second)
		srv.methods[method](context.Background(), req)
	}
}

/*type Server struct {
	*gorums.Server
	methods map[string]func(ctx context.Context, req any) (any, error)
}

func NewServer(id string) *Server {
	return &Server{
		Server:  gorums.NewServer(),
		methods: make(map[string]func(ctx context.Context, req any) (resp any, err error)),
	}
}

func (srv *Server) RegisterQCStorageServer(impl QuorumSpec) {
	srv.RegisterHandler("protos.QCStorage.Read", gorums.DefaultHandler(impl.Read))
	srv.RegisterHandler("protos.QCStorage.Write", gorums.BestEffortBroadcastHandler(impl.Write, srv.Server))
}

func (srv *Server) RegisterConfiguration(c *Configuration) {
	srv.methods["protos.QCStorage.Write"] = gorums.RegisterBroadcastFunc(c.Write)
	go srv.run()
}

func (srv *Server) run() {
	for msg := range srv.BroadcastChan {
		req := msg.GetRequest()
		method := msg.GetMethod()
		//ctx := msg.GetContext() <- old context, will be cancelled by calling client
		time.Sleep(5 * time.Second)
		srv.methods[method](context.Background(), req)
	}
}*/
