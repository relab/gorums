package benchmark

import (
	context "context"
	fmt "fmt"
	log "log"
	net "net"
)

type unorderedServer struct{}

func (srv *unorderedServer) UnorderedQC(_ context.Context, in *Echo) (out *Echo, _ error) {
	out = in
	return
}

func (srv *unorderedServer) OrderedQC(_ context.Context, in *Echo) (out *Echo, _ error) {
	panic("Not implemented")
}

func (srv *unorderedServer) UnorderedAsync(_ context.Context, in *Echo) (out *Echo, _ error) {
	out = in
	return
}

func (srv *unorderedServer) OrderedAsync(_ context.Context, in *Echo) (out *Echo, _ error) {
	panic("Not implemented")
}

type orderedServer struct{}

func (srv *orderedServer) OrderedQC(in *Echo) *Echo {
	return in
}

func (srv *orderedServer) OrderedAsync(in *Echo) *Echo {
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
	srv.RegisterOrderedAsyncHandler(&srv.orderedSrv)
	RegisterBenchmarkServer(srv.GorumsServer.grpcServer, &srv.unorderedSrv)
	return srv
}

// StartLocalServers starts benchmark servers locally
func StartLocalServers(ctx context.Context, n int) []string {
	var ports []string
	basePort := 40000
	var servers []*Server
	for p := basePort; p < basePort+n; p++ {
		port := fmt.Sprintf(":%d", p)
		ports = append(ports, port)
		lis, err := net.Listen("tcp", port)
		if err != nil {
			log.Fatalf("Failed to start local server: %v\n", err)
		}
		srv := NewServer()
		servers = append(servers, srv)
		go srv.Serve(lis)
	}
	go func() {
		<-ctx.Done()
		for _, srv := range servers {
			srv.Stop()
		}
	}()
	return ports
}
