package benchkit

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	// meshAttemptTimeout bounds one PeerView call to one node in [AwaitMesh],
	// so a lost reply costs one round, not the caller's whole deadline.
	meshAttemptTimeout = 2 * time.Second
	// meshRetryDelay is the pause between two rounds of [AwaitMesh].
	meshRetryDelay = 200 * time.Millisecond
)

// peerView is one node's answer to a PeerView call in [AwaitMesh].
type peerView struct {
	id      uint32
	missing []uint32 // configured peers that the node has no stream to
	err     error    // the error of the call; nil if the node answered
}

// complete reports whether the node answered with a complete view.
func (v peerView) complete() bool {
	return v.err == nil && len(v.missing) == 0
}

// String describes an incomplete view for the error of [AwaitMesh].
func (v peerView) String() string {
	if v.err != nil {
		return fmt.Sprintf("node %d did not answer: %v", v.id, v.err)
	}
	return fmt.Sprintf("node %d is missing peers %v", v.id, v.missing)
}

// AwaitMesh returns nil once every node of cfg reports, in the same round of
// calls, that it has a connected stream to each of its configured peers. A
// node reports its view through the PeerView RPC of the [Control] service, so
// every node of cfg must register a [Control] with [RegisterControlServer].
// cfg is typically [gorums.Server.PeerConfig], which includes the local node,
// so the caller's own view is part of the check.
//
// Each round calls PeerView on every node of cfg in parallel, and each call has
// its own deadline of a few seconds. Each node answers from its view at the
// time of the call, so a node that loses a stream between rounds reports the
// loss in the next round. When ctx ends, AwaitMesh returns an error that wraps
// ctx.Err() and names each node that did not answer in the last round, and
// each node that answered with missing peers, together with those peers.
//
// A nil return is an observation at the time of the last round. A stream can
// still fail after AwaitMesh returns.
func AwaitMesh(ctx context.Context, cfg Config) error {
	for {
		views := peerViews(ctx, cfg)
		var incomplete []string
		for _, v := range views {
			if !v.complete() {
				incomplete = append(incomplete, v.String())
			}
		}
		if len(incomplete) == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("mesh not ready: %s: %w", strings.Join(incomplete, "; "), ctx.Err())
		case <-time.After(meshRetryDelay):
		}
	}
}

// peerViews calls PeerView on every node of cfg in parallel and returns the
// answers in the order of cfg.
func peerViews(ctx context.Context, cfg Config) []peerView {
	ctx, cancel := context.WithTimeout(ctx, meshAttemptTimeout)
	defer cancel()
	views := make([]peerView, len(cfg))
	var wg sync.WaitGroup
	for i, node := range cfg {
		wg.Go(func() {
			views[i].id = node.ID()
			peer := NewControlClient(Config{node})
			resp, err := peer.PeerView(ctx, &PeerViewRequest{}).Threshold(1)
			if err != nil {
				views[i].err = err
				return
			}
			views[i].missing = resp.GetMissing()
		})
	}
	wg.Wait()
	return views
}
