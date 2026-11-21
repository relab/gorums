package benchmark

import (
	"context"
	"fmt"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/benchkit"
)

// SetupTarget builds the Target for local or coordinator mode and fills in
// the topology-derived Options fields (Remote, NumNodes):
//
//   - local: remotes is empty; configSize in-process servers are created.
//   - coordinator: remotes lists the servers; this process coordinates against
//     them, using configSize nodes when 1 <= configSize <= len(remotes) and all
//     of them otherwise.
//
// Distributed mode uses [ListenDistributed] and [Workload] instead. SetupTarget
// returns the target and a cleanup function the caller must invoke (e.g.
// defer) once the benchmarks finish.
func SetupTarget(opts *benchkit.Options, remotes []string, configSize int, dialOpts ...gorums.DialOption) (Target, func(), error) {
	if len(remotes) < 1 {
		return setupLocal(opts, configSize, dialOpts)
	}
	return setupCoordinator(opts, remotes, configSize, dialOpts)
}

// ListenDistributed returns the peer self of a distributed run over remotes
// (see [benchkit.ListenPeer]) and fills in the topology-derived Options fields
// (Remote, NumNodes). remotes must hold at least two addresses, self included.
// Run the peer with [Workload].
func ListenDistributed(opts *benchkit.Options, self string, remotes []string, dialOpts ...gorums.DialOption) (*benchkit.Peer, error) {
	if len(remotes) < 2 {
		return nil, fmt.Errorf("distributed mode requires at least 2 remotes (including self)")
	}
	opts.Remote = true
	opts.NumNodes = len(remotes)
	return benchkit.ListenPeer(self, remotes, *opts, dialOpts...)
}

// Workload returns the workload of one peer of a distributed run for
// [benchkit.Peer.Run]: its setup hook registers the Benchmark service and
// returns the symmetric benchmarks.
func Workload() benchkit.Workload {
	return benchkit.Workload{
		Setup: func(p *benchkit.Peer) ([]benchkit.Bench, error) {
			RegisterBenchmarkServer(p.Server(), &workloadServer{ctrl: p.Control()})
			// Symmetric benchmarks let each process own its peer's measurement
			// window. Traditional benchmarks control the windows of all peers.
			return bindBenchmarks(symmetricBenchDescs, peerConfig{p}), nil
		},
	}
}

// setupLocal builds the symmetric target backed by configSize in-process
// servers.
func setupLocal(opts *benchkit.Options, configSize int, dialOpts []gorums.DialOption) (Target, func(), error) {
	var target Target
	if configSize < 1 {
		return target, nil, fmt.Errorf("local mode requires config-size >= 1, got %d", configSize)
	}
	opts.Remote = false
	opts.NumNodes = configSize

	peers, stop, err := setupSymmetricServers(configSize, opts.ServerOptions(), dialOpts...)
	if err != nil {
		return target, nil, fmt.Errorf("symmetric servers setup: %w", err)
	}

	readyCtx, readyCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer readyCancel()
	if err := awaitReady(readyCtx, peers); err != nil {
		stop()
		return target, nil, fmt.Errorf("symmetric servers not ready: %w", err)
	}

	target.Symmetric = peers
	return target, stop, nil
}

// setupCoordinator builds the traditional coordinator-side configuration
// against the given remote servers.
func setupCoordinator(opts *benchkit.Options, remotes []string, configSize int, dialOpts []gorums.DialOption) (Target, func(), error) {
	var target Target
	opts.Remote = true
	numNodes := len(remotes)
	if configSize < 1 || configSize > numNodes {
		opts.NumNodes = numNodes
	} else {
		opts.NumNodes = configSize
	}

	cfg, closeFn, err := gorums.NewConfig(gorums.WithNodeList(remotes[:opts.NumNodes]), dialOpts...)
	if err != nil {
		return target, nil, fmt.Errorf("configuration setup: %w", err)
	}

	target.Config = cfg
	return target, closeFn, nil
}
