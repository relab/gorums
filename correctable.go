package gorums

import (
	"sync"

	"google.golang.org/protobuf/proto"
)

// LevelNotSet is the zero value level used to indicate that no level (and
// thereby no reply) has been set for a correctable quorum call.
const LevelNotSet = -1

type watcher struct {
	level int
	ch    chan struct{}
}

// Correctable encapsulates the state of a correctable quorum call.
//
// This struct should be used by generated code only.
type Correctable[resultType any] struct {
	mu       sync.Mutex
	reply    resultType
	level    int
	err      error
	done     bool
	watchers []*watcher
	donech   chan struct{}
}

// Get returns the latest response, the current level, and the last error.
func (c *Correctable[resultType]) Get() (resultType, int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reply, c.level, c.err
}

// Done returns a channel that will close when the correctable call is completed.
func (c *Correctable[resultType]) Done() <-chan struct{} {
	return c.donech
}

// Watch returns a channel that will close when the correctable call has reached a specified level.
func (c *Correctable[resultType]) Watch(level int) <-chan struct{} {
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

func (c *Correctable[resultType]) set(reply resultType, level int, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.done {
		panic("set(...) called on a done correctable")
	}
	c.reply, c.level, c.err = reply, level, err
	for i := range c.watchers {
		if c.watchers[i] != nil && c.watchers[i].level <= level {
			close(c.watchers[i].ch)
			c.watchers[i] = nil
		}
	}
}

func (c *Correctable[resultType]) setDone() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.done {
		panic("setDone(...) called on a done correctable")
	}

	c.done = true
	close(c.donech)
	for _, watcher := range c.watchers {
		if watcher != nil {
			close(watcher.ch)
		}
	}
}

// NewCorrectable lets you use a quorum call as a correctable call.
func NewCorrectable[responseType proto.Message, resultType any](
	iter Responses[responseType],
	corrFunction func(Responses[responseType], func(resultType, int, error)),
) *Correctable[resultType] {
	corr := &Correctable[resultType]{donech: make(chan struct{}, 1)}
	go func() {
		corrFunction(iter, corr.set)
		corr.setDone()
	}()
	return corr
}
