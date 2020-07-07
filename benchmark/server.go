package benchmark

import (
	context "context"
	fmt "fmt"
	log "log"
	net "net"
	"time"

	empty "github.com/golang/protobuf/ptypes/empty"
)

type unorderedServer struct {
	stats *Stats
	UnimplementedBenchmarkServer
}

func (srv *unorderedServer) StartServerBenchmark(_ context.Context, _ *StartRequest) (_ *StartResponse, _ error) {
	panic("Not implemented")
}

func (srv *unorderedServer) StopServerBenchmark(_ context.Context, _ *StopRequest) (_ *Result, _ error) {
	panic("Not implemented")
}

func (srv *unorderedServer) StartBenchmark(_ context.Context, _ *StartRequest) (_ *StartResponse, _ error) {
	panic("Not implemented")
}

func (srv *unorderedServer) StopBenchmark(_ context.Context, _ *StopRequest) (_ *MemoryStat, _ error) {
	panic("Not implemented")
}

func (srv *unorderedServer) UnorderedQC(_ context.Context, in *Echo) (out *Echo, _ error) {
	out = in
	return
}

func (srv *unorderedServer) OrderedQC(_ context.Context, _ *Echo) (_ *Echo, _ error) {
	panic("Not implemented")
}

func (srv *unorderedServer) UnorderedAsync(_ context.Context, in *Echo) (out *Echo, _ error) {
	out = in
	return
}

func (srv *unorderedServer) OrderedAsync(_ context.Context, _ *Echo) (_ *Echo, _ error) {
	panic("Not implemented")
}

func (srv *unorderedServer) UnorderedSlowServer(_ context.Context, in *Echo) (out *Echo, _ error) {
	time.Sleep(10 * time.Millisecond)
	out = in
	return
}

func (srv *unorderedServer) OrderedSlowServer(_ context.Context, _ *Echo) (_ *Echo, _ error) {
	panic("Not implemented")
}

func (srv *unorderedServer) Multicast(_ context.Context, _ *TimedMsg) (_ *empty.Empty, _ error) {
	panic("Not implemented")
}

type orderedServer struct {
	stats *Stats
}

func (srv *orderedServer) OrderedQC(_ context.Context, in *Echo, out func(*Echo)) {
	out(in)
}

func (srv *orderedServer) OrderedAsync(_ context.Context, in *Echo, out func(*Echo)) {
	out(in)
}

func (srv *orderedServer) OrderedSlowServer(_ context.Context, in *Echo, out func(*Echo)) {
	go func() {
		time.Sleep(10 * time.Millisecond)
		out(in)
	}()
}

func (srv *orderedServer) Multicast(_ context.Context, msg *TimedMsg) {
	latency := time.Now().UnixNano() - msg.SendTime
	srv.stats.AddLatency(time.Duration(latency))
}

func (srv *orderedServer) StartServerBenchmark(_ context.Context, _ *StartRequest, out func(*StartResponse)) {
	srv.stats.Clear()
	srv.stats.Start()
	out(&StartResponse{})
}

func (srv *orderedServer) StopServerBenchmark(_ context.Context, _ *StopRequest, out func(*Result)) {
	srv.stats.End()
	out(srv.stats.GetResult())
}

func (srv *orderedServer) StartBenchmark(_ context.Context, _ *StartRequest, out func(*StartResponse)) {
	srv.stats.Clear()
	srv.stats.Start()
	out(&StartResponse{})
}

func (srv *orderedServer) StopBenchmark(_ context.Context, _ *StopRequest, out func(*MemoryStat)) {
	srv.stats.End()
	out(&MemoryStat{
		Allocs: srv.stats.endMs.Mallocs - srv.stats.startMs.Mallocs,
		Memory: srv.stats.endMs.TotalAlloc - srv.stats.startMs.TotalAlloc,
	})
}

// Server is a unified server for both ordered and unordered methods
type Server struct {
	*GorumsServer
	orderedSrv   orderedServer
	unorderedSrv unorderedServer
	stats        Stats
}

// NewServer returns a new Server
func NewServer(opts ...ServerOption) *Server {
	srv := &Server{}
	srv.orderedSrv.stats = &srv.stats
	srv.unorderedSrv.stats = &srv.stats

	srv.GorumsServer = NewGorumsServer(opts...)
	srv.GorumsServer.RegisterBenchmarkServer(&srv.orderedSrv)
	RegisterBenchmarkServer(srv.GorumsServer.grpcServer, &srv.unorderedSrv)
	return srv
}

// StartLocalServers starts benchmark servers locally
func StartLocalServers(ctx context.Context, n int, opts ...ServerOption) []string {
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
		srv := NewServer(opts...)
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
