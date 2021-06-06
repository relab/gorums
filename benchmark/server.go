package benchmark

import (
	context "context"
	fmt "fmt"
	log "log"
	net "net"
	"time"

	gorums "github.com/relab/gorums"
)

type server struct {
	stats *Stats
}

func (srv *server) QuorumCall(_ gorums.ServerCtx, in *Echo) (resp *Echo, err error) {
	return in, nil
}

func (srv *server) AsyncQuorumCall(_ gorums.ServerCtx, in *Echo) (resp *Echo, err error) {
	return in, nil
}

func (srv *server) SlowServer(ctx gorums.ServerCtx, in *Echo) (resp *Echo, err error) {
	ctx.Release()
	time.Sleep(10 * time.Millisecond)
	return in, nil
}

func (srv *server) Multicast(_ gorums.ServerCtx, msg *TimedMsg) {
	latency := time.Now().UnixNano() - msg.SendTime
	srv.stats.AddLatency(time.Duration(latency))
}

func (srv *server) StartServerBenchmark(_ gorums.ServerCtx, _ *StartRequest) (resp *StartResponse, err error) {
	srv.stats.Clear()
	srv.stats.Start()
	return &StartResponse{}, nil
}

func (srv *server) StopServerBenchmark(_ gorums.ServerCtx, _ *StopRequest) (resp *Result, err error) {
	srv.stats.End()
	return srv.stats.GetResult(), nil
}

func (srv *server) StartBenchmark(_ gorums.ServerCtx, _ *StartRequest) (resp *StartResponse, err error) {
	srv.stats.Clear()
	srv.stats.Start()
	return &StartResponse{}, nil
}

func (srv *server) StopBenchmark(_ gorums.ServerCtx, _ *StopRequest) (resp *MemoryStat, err error) {
	srv.stats.End()
	return &MemoryStat{
		Allocs: srv.stats.endMs.Mallocs - srv.stats.startMs.Mallocs,
		Memory: srv.stats.endMs.TotalAlloc - srv.stats.startMs.TotalAlloc,
	}, nil
}

// Server is a unified server for both ordered and unordered methods
type Server struct {
	*gorums.Server
	server server
	stats  Stats
}

// NewServer returns a new Server
func NewBenchServer(opts ...gorums.ServerOption) *Server {
	srv := &Server{}
	srv.server.stats = &srv.stats

	srv.Server = gorums.NewServer(opts...)
	RegisterBenchmarkServer(srv.Server, &srv.server)
	return srv
}

// StartLocalServers starts benchmark servers locally
func StartLocalServers(ctx context.Context, n int, opts ...gorums.ServerOption) []string {
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
		srv := NewBenchServer(opts...)
		servers = append(servers, srv)
		go func() { _ = srv.Serve(lis) }()
	}
	go func() {
		<-ctx.Done()
		for _, srv := range servers {
			srv.Stop()
		}
	}()
	return ports
}
