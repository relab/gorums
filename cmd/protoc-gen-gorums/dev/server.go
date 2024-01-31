package dev

import (
	context "context"

	"github.com/relab/gorums"
)

type Server struct {
	*gorums.Server
	methods     map[string]gorums.BroadcastFunc
	conversions map[string]gorums.ConversionFunc
}

func NewServer() *Server {
	return &Server{
		Server: gorums.NewServer(),
	}
}

func RegisterQCStorageServer(srv *Server, impl ZorumsService) {
	srv.RegisterHandler("dev.ZorumsService.QuorumCall", gorums.DefaultHandler(impl.QuorumCall))
	srv.RegisterHandler("dev.ZorumsService.QuorumCallAllToAll", gorums.BroadcastHandler(impl.QuorumCallAllToAll, srv.Server))

	//srv.conversions["dev.ZorumsService.QuorumCallAllToAll"] = gorums.RegisterConversionFunc(impl.QuorumCall)
}

func (srv *Server) RegisterConfiguration(c *Configuration) {
	srv.methods["dev.ZorumsService.QuorumCallAllToAll"] = gorums.RegisterBroadcastFunc(c.QuorumCall)
	go srv.run()
}

func (srv *Server) run() {
	for msg := range srv.BroadcastChan {
		req := msg.GetRequest()
		method := msg.GetMethod()
		ctx := context.Background()
		// if another function is called in broadcast, the request needs to be converted
		//if convertFunc, ok := srv.conversions[method]; ok {
		//	convertedReq := convertFunc(ctx, req)
		//	srv.methods[method](ctx, convertedReq)
		//	continue
		//}
		srv.methods[method](ctx, req)
	}
}

/*
type Server struct {
	*gorums.Server
	methods map[string]gorums.BroadcastFunc
	conversions map[string]gorums.ConversionFunc
}

func NewServer(id string) *Server {
	return &Server{
		Server: gorums.NewServer(),
		methods: make(map[string]gorums.BroadcastFunc),
		conversions: make(map[string]gorums.ConversionFunc),
	}
}

func RegisterPBFTNodeServer(srv *Server, impl PBFTNode) {
	srv.RegisterHandler("protos.PBFTNode.PrePrepare", gorums.BestEffortBroadcastHandler(impl.PrePrepare, srv.Server))
	srv.RegisterHandler("protos.PBFTNode.Prepare", gorums.BestEffortBroadcastHandler(impl.Prepare, srv.Server))
	srv.RegisterHandler("protos.PBFTNode.Commit", gorums.DefaultHandler(impl.Commit))

	srv.conversions["protos.PBFTNode.PrePrepare"] = gorums.RegisterConversionFunc(impl.ConvertPrePrepareToPrepareRequest)
}

func (srv *Server) RegisterConfiguration(c *Configuration) {
	srv.methods["protos.QCStorage.PrePrepare"] = gorums.RegisterBroadcastFunc(c.Prepare)
	srv.methods["protos.QCStorage.Prepare"] = gorums.RegisterBroadcastFunc(c.Commit)
	go srv.run()
}

func (srv *Server) run() {
	for msg := range srv.BroadcastChan {
		req := msg.GetRequest()
		method := msg.GetMethod()
		ctx := context.Background()
		// if another function is called in broadcast, the request needs to be converted
		if convertFunc, ok := srv.conversions[method]; ok {
			convertedReq := convertFunc(ctx, req)
			srv.methods[method](ctx, convertedReq)
			continue
		}
		srv.methods[method](ctx, req)
	}
}
*/

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
