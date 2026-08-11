package benchmark

import (
	"context"
	"fmt"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/benchkit"
)

// SetupTarget builds the BenchTarget for one of the three run modes and fills
// in the topology-derived Options fields (Remote, NumNodes):
//
//   - distributed: self is this node's listen address and remotes lists all
//     peers (including self); every node runs the same binary.
//   - local: neither self nor remotes is set; configSize in-process servers are
//     created.
//   - coordinator: remotes lists the servers and self is unset; this process
//     coordinates against them, using configSize nodes when 1 <= configSize <=
//     len(remotes) and all of them otherwise.
//
// It returns the target and a cleanup function the caller must invoke (e.g.
// defer) once the benchmarks finish. In distributed mode the caller must also
// linger for ExitGrace before invoking cleanup, so slower peers can finish
// their trailing cross-node RPCs before this node closes its listener.
func SetupTarget(opts *benchkit.Options, self string, remotes []string, configSize int, dialOpts ...gorums.DialOption) (BenchTarget, func(), error) {
	switch {
	case self != "":
		return setupDistributed(opts, self, remotes, dialOpts)
	case len(remotes) < 1:
		return setupLocal(opts, configSize, dialOpts)
	default:
		return setupCoordinator(opts, remotes, configSize, dialOpts)
	}
}

// setupDistributed builds the symmetric peer-to-peer target for one node of a
// distributed run and waits until every peer is reachable.
func setupDistributed(opts *benchkit.Options, self string, remotes []string, dialOpts []gorums.DialOption) (BenchTarget, func(), error) {
	var target BenchTarget
	if len(remotes) < 2 {
		return target, nil, fmt.Errorf("distributed mode requires at least 2 remotes (including self)")
	}
	opts.Remote = true
	opts.NumNodes = len(remotes)

	symTarget, symStop, err := SetupRemoteServer(self, remotes, opts.ServerOptions(), dialOpts...)
	if err != nil {
		return target, nil, fmt.Errorf("remote server setup: %w", err)
	}

	// In dedup mode, wait for every shared stream to be live first, so the
	// probe below exercises the shared topology instead of failing fast with
	// ErrStreamDown for lower-ID peers that have not yet connected.
	if opts.StreamMode == "dedup" {
		dedupCtx, dedupCancel := context.WithTimeout(context.Background(), 2*time.Minute)
		err := awaitStreamDedup(dedupCtx, symTarget)
		dedupCancel()
		if err != nil {
			symStop()
			return target, nil, err
		}
	}
	// The deadline is only an upper bound for very large clusters that are
	// still making progress; a dead peer is detected within readyStallTimeout
	// by the probe's stall check.
	readyCtx, readyCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer readyCancel()
	if err := AwaitReady(readyCtx, symTarget); err != nil {
		symStop()
		return target, nil, fmt.Errorf("remote peers not ready: %w", err)
	}

	target.Symmetric = symTarget
	return target, symStop, nil
}

// setupLocal builds the symmetric target backed by configSize in-process
// servers.
func setupLocal(opts *benchkit.Options, configSize int, dialOpts []gorums.DialOption) (BenchTarget, func(), error) {
	var target BenchTarget
	if configSize < 1 {
		return target, nil, fmt.Errorf("local mode requires config-size >= 1, got %d", configSize)
	}
	opts.Remote = false
	opts.NumNodes = configSize

	symTarget, symStop, err := SetupSymmetricServers(configSize, opts.ServerOptions(), dialOpts...)
	if err != nil {
		return target, nil, fmt.Errorf("symmetric servers setup: %w", err)
	}

	// In dedup mode, wait for every shared stream to be live first, so the
	// probe below exercises the shared topology instead of failing fast with
	// ErrStreamDown for lower-ID peers that have not yet connected.
	if opts.StreamMode == "dedup" {
		dedupCtx, dedupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := awaitStreamDedup(dedupCtx, symTarget)
		dedupCancel()
		if err != nil {
			symStop()
			return target, nil, err
		}
	}
	readyCtx, readyCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer readyCancel()
	if err := AwaitReady(readyCtx, symTarget); err != nil {
		symStop()
		return target, nil, fmt.Errorf("symmetric servers not ready: %w", err)
	}

	target.Symmetric = symTarget
	return target, symStop, nil
}

func awaitStreamDedup(ctx context.Context, t *SymmetricTarget) error {
	for i, srv := range t.servers {
		if _, err := srv.WaitForAll(ctx); err != nil {
			return fmt.Errorf("%s: stream dedup setup: %w", t.label(i), err)
		}
	}
	return nil
}

// setupCoordinator builds the traditional coordinator-side configuration
// against the given remote servers.
func setupCoordinator(opts *benchkit.Options, remotes []string, configSize int, dialOpts []gorums.DialOption) (BenchTarget, func(), error) {
	var target BenchTarget
	opts.Remote = true
	numNodes := len(remotes)
	if configSize < 1 || configSize > numNodes {
		opts.NumNodes = numNodes
	} else {
		opts.NumNodes = configSize
	}

	cfg, err := gorums.NewConfig(gorums.WithNodeList(remotes[:opts.NumNodes]), dialOpts...)
	if err != nil {
		return target, nil, fmt.Errorf("configuration setup: %w", err)
	}

	target.Config = cfg
	return target, func() { _ = cfg.Close() }, nil
}
