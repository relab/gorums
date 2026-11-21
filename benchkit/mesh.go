package benchkit

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// meshRetryDelay is the pause between two rounds of [AwaitMesh].
const meshRetryDelay = 200 * time.Millisecond

var (
	// meshAttemptTimeout bounds one PeerView call to one node in [AwaitMesh],
	// so a lost reply costs one round, not the caller's whole deadline. It is a
	// variable so tests can shorten it.
	meshAttemptTimeout = 2 * time.Second
	// meshStallTimeout bounds how long [AwaitMesh] waits for one more node to
	// report a complete view. It is a variable so tests can shorten it.
	meshStallTimeout = 20 * time.Second
)

// peerView is one node's answer to a PeerView call in [AwaitMesh].
type peerView struct {
	id      uint32
	addr    string
	missing []uint32 // configured peers that the node has no stream to
	err     error    // the error of the call; nil if the node answered
}

// complete reports whether the node answered with a complete view.
func (v peerView) complete() bool {
	return v.err == nil && len(v.missing) == 0
}

// String describes an incomplete view for the log and the error of
// [AwaitMesh].
func (v peerView) String() string {
	if v.err != nil {
		return fmt.Sprintf("node %d (%s) did not answer: %v", v.id, v.addr, v.err)
	}
	return fmt.Sprintf("node %d (%s) is missing peers %v", v.id, v.addr, v.missing)
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
// loss in the next round. AwaitMesh returns an error that names each node that
// did not answer in the last round, and each node that answered with missing
// peers, together with those peers. It returns this error when ctx ends, and
// then the error wraps ctx.Err(). It also returns this error when no further
// node has reported a complete view for 20 seconds, so a dead peer fails the
// call long before a generous deadline.
//
// A nil return is an observation at the time of the last round. A stream can
// still fail after AwaitMesh returns.
func AwaitMesh(ctx context.Context, cfg Config) error {
	best, lastProgress := 0, time.Now()
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
		desc := strings.Join(incomplete, "; ")
		if complete := len(views) - len(incomplete); complete > best {
			best, lastProgress = complete, time.Now()
		} else if time.Since(lastProgress) >= meshStallTimeout {
			return fmt.Errorf("mesh not ready: no further node reported a complete view for %v: %s", meshStallTimeout, desc)
		}
		Logf("[ready] mesh: %d of %d nodes incomplete: %s\n", len(incomplete), len(views), desc)
		select {
		case <-ctx.Done():
			return fmt.Errorf("mesh not ready: %s: %w", desc, ctx.Err())
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
			views[i].addr = node.Address()
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
