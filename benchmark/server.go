package benchmark

import (
	context "context"
	fmt "fmt"
	log "log"
	net "net"
	"time"
)

type unorderedServer struct {
	stats *Stats
}

func (srv *unorderedServer) StartServerBenchmark(_ context.Context, _ *StartRequest) (_ *StartResponse, _ error) {
	panic("Not implemented")
}

func (srv *unorderedServer) StopServerBenchmark(_ context.Context, _ *StopRequest) (_ *StopResponse, _ error) {
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

func (srv *unorderedServer) Multicast(stream Benchmark_MulticastServer) error {
	for {
		msg, err := stream.Recv()
		if err != nil {
			return err
		}
		latency := time.Now().UnixNano() - msg.SendTime
		srv.stats.AddLatency(time.Duration(latency))
	}
}

type orderedServer struct {
	stats *Stats
}

func (srv *orderedServer) OrderedQC(in *Echo) *Echo {
	return in
}

func (srv *orderedServer) OrderedAsync(in *Echo) *Echo {
	return in
}

func (srv *orderedServer) OrderedSlowServer(in *Echo) *Echo {
	time.Sleep(10 * time.Millisecond)
	return in
}

// Server is a unified server for both ordered and unordered methods
type Server struct {
	*GorumsServer
	orderedSrv   orderedServer
	unorderedSrv unorderedServer
	stats        Stats
}

// NewServer returns a new Server
func NewServer() *Server {
	srv := &Server{}
	srv.orderedSrv.stats = &srv.stats
	srv.unorderedSrv.stats = &srv.stats

	srv.GorumsServer = NewGorumsServer()
	srv.RegisterStartServerBenchmarkHandler(srv)
	srv.RegisterStopServerBenchmarkHandler(srv)
	srv.RegisterOrderedQCHandler(&srv.orderedSrv)
	srv.RegisterOrderedAsyncHandler(&srv.orderedSrv)
	srv.RegisterOrderedSlowServerHandler(&srv.orderedSrv)
	RegisterBenchmarkServer(srv.GorumsServer.grpcServer, &srv.unorderedSrv)
	return srv
}

func (srv *Server) StartServerBenchmark(in *StartRequest) *StartResponse {
	srv.stats.Clear()
	srv.stats.Start()
	return &StartResponse{}
}

func (srv *Server) StopServerBenchmark(in *StopRequest) *StopResponse {
	srv.stats.End()
	results := srv.stats.GetResult()
	return &StopResponse{
		TotalOps:    results.TotalOps,
		TotalTime:   int64(results.TotalTime),
		Throughput:  results.Throughput,
		LatencyAvg:  results.LatencyAvg,
		LatencyVar:  results.LatencyVar,
		AllocsPerOp: results.AllocsPerOp,
		MemPerOp:    results.MemPerOp,
	}
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
