package benchmark

import (
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/benchkit"
)

// workloadServer implements the gorums Benchmark workload RPCs. Each handled
// operation and each server-measured Multicast latency is recorded into the
// shared benchkit.Control, so the control plane's Stop reply observes this
// server's work. The measurement control plane (Start/Stop/ClockSync) lives in
// benchkit.Control, registered alongside this server on one listener.
type workloadServer struct {
	ctrl *benchkit.Control
}

func (srv *workloadServer) QuorumCall(_ gorums.ServerContext, in *Echo) (*Echo, error) {
	srv.ctrl.RecordOp()
	return in, nil
}

func (srv *workloadServer) SlowServer(ctx gorums.ServerContext, in *Echo) (*Echo, error) {
	ctx.Release()
	srv.ctrl.RecordOp()
	time.Sleep(10 * time.Millisecond)
	return in, nil
}

func (srv *workloadServer) Multicast(_ gorums.ServerContext, msg *TimedMsg) {
	srv.ctrl.RecordOp()
	latency := time.Duration(time.Now().UnixNano() - msg.GetSendTime())
	// A symmetric sender tags the message with its node ID (>= 1) so its
	// samples can be corrected by that sender's clock offset; an untagged
	// message (sender ID 0) records into the flat samples instead.
	if id := msg.GetSenderId(); id != 0 {
		srv.ctrl.Stats().AddLatencyBySender(id, latency)
	} else {
		srv.ctrl.Stats().AddLatency(latency)
	}
}

// attachBenchServer registers benchkit's Control server and the gorums workload
// server on srv, sharing one Control instance, and returns the Control handle.
func attachBenchServer(srv *gorums.Server) *benchkit.Control {
	ctrl := benchkit.NewControl()
	w := &workloadServer{ctrl: ctrl}
	ctrl.SetID(srv.NodeID())
	benchkit.RegisterControlServer(srv, ctrl)
	RegisterBenchmarkServer(srv, w)
	return ctrl
}

// Server is a unified server registering both benchkit's Control plane and the
// gorums workload service on one listener.
type Server struct {
	*gorums.Server
	ctrl *benchkit.Control
}

// NewBenchServer returns a new benchmark server.
func NewBenchServer(opts ...gorums.ServerOption) *Server {
	srv := gorums.NewServer(opts...)
	return &Server{Server: srv, ctrl: attachBenchServer(srv)}
}
