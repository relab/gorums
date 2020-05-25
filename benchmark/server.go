package benchmark

import (
	context "context"
)

type unorderedServer struct{}

func (srv *unorderedServer) UnorderedQC(_ context.Context, in *Echo) (out *Echo, _ error) {
	out = in
	return
}

func (srv *unorderedServer) OrderedQC(_ context.Context, in *Echo) (out *Echo, _ error) {
	panic("Not implemented")
}

type orderedServer struct{}

func (srv *orderedServer) OrderedQC(in *Echo) *Echo {
	return in
}

// Server is a unified server for both ordered and unordered methods
type Server struct {
	*GorumsServer
	orderedSrv   orderedServer
	unorderedSrv unorderedServer
}

// NewServer returns a new Server
func NewServer() *Server {
	srv := &Server{}
	srv.GorumsServer = NewGorumsServer()
	srv.RegisterOrderedQCHandler(&srv.orderedSrv)
	RegisterBenchmarkServer(srv.GorumsServer.grpcServer, &srv.unorderedSrv)
	return srv
}
