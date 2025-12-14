package gorums

import (
	"sync"
)

// LevelNotSet is the zero value level used to indicate that no level (and
// thereby no reply) has been set for a correctable quorum call.
const LevelNotSet = -1

type watcher struct {
	level int
	ch    chan struct{}
}

// Correctable is a generic type for correctable quorum calls.
// It encapsulates the state of a correctable call and provides methods
// for checking the status or waiting for completion at specific levels.
//
// Type parameter Resp is the response type from nodes.
type Correctable[Resp any] struct {
	mu       sync.Mutex
	reply    Resp
	level    int
	err      error
	done     bool
	watchers []*watcher
	donech   chan struct{}
}

// NewCorrectable creates a new Correctable object.
func NewCorrectable[Resp any]() *Correctable[Resp] {
	return &Correctable[Resp]{
		level:  LevelNotSet,
		donech: make(chan struct{}, 1),
	}
}

// Get returns the latest response, the current level, and the last error.
func (c *Correctable[Resp]) Get() (Resp, int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reply, c.level, c.err
}

// Done returns a channel that will close when the correctable call is completed.
func (c *Correctable[Resp]) Done() <-chan struct{} {
	return c.donech
}

// Watch returns a channel that will close when the correctable call has reached a specified level.
func (c *Correctable[Resp]) Watch(level int) <-chan struct{} {
	ch := make(chan struct{})
	c.mu.Lock()
	defer c.mu.Unlock()
	if level <= c.level {
		close(ch)
		return ch
	}
	c.watchers = append(c.watchers, &watcher{level, ch})
	return ch
}

// update updates the current state of the correctable call.
// It updates the response, level, and error, and notifies any watchers.
// If done is true, the call is considered complete and the Done channel is closed.
func (c *Correctable[Resp]) update(reply Resp, level int, done bool, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.done {
		panic("Update(...) called on a done correctable")
	}
	c.reply, c.level, c.err, c.done = reply, level, err, done
	if done {
		close(c.donech)
		for _, watcher := range c.watchers {
			if watcher != nil {
				close(watcher.ch)
			}
		}
		return
	}
	for i := range c.watchers {
		if c.watchers[i] != nil && c.watchers[i].level <= level {
			close(c.watchers[i].ch)
			c.watchers[i] = nil
		}
	}
}

// Correctable returns a Correctable that provides progressive updates
// as responses arrive. The level increases with each successful response.
// Use this for correctable quorum patterns where you want to observe
// intermediate states.
//
// Example:
//
//	corr := ReadQC(ctx, req).Correctable(2)
//	// Wait for level 2 to be reached
//	<-corr.Watch(2)
//	resp, level, err := corr.Get()
func (r *Responses[Resp]) Correctable(threshold int) *Correctable[Resp] {
	corr := &Correctable[Resp]{
		level:  LevelNotSet,
		donech: make(chan struct{}, 1),
	}

	go func() {
		var (
			lastResp Resp
			found    bool
			count    int
			errs     []nodeError
		)

		for result := range r.ResponseSeq {
			if result.Err != nil {
				errs = append(errs, nodeError{nodeID: result.NodeID, cause: result.Err})
				continue
			}

			count++
			lastResp = result.Value
			found = true

			// Check if we have reached the threshold
			done := count >= threshold
			corr.update(lastResp, count, done, nil)
			if done {
				return
			}
		}

		// If we didn't reach the threshold, mark as done with error
		if !found {
			var zero Resp
			corr.update(zero, count, true, QuorumCallError{cause: ErrIncomplete, errors: errs})
		} else {
			corr.update(lastResp, count, true, QuorumCallError{cause: ErrIncomplete, errors: errs})
		}
	}()

	return corr
}
