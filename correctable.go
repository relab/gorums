package gorums

import (
	"iter"
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

func (c *Correctable[resultType]) set(reply resultType, level int, err error, done bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.done {
		panic("set(...) called on a done correctable")
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

func IterToCorrectable[responseType proto.Message, dataType any, resultType any](
	iter Iterator[responseType],
	data dataType,
	corrFunction func(Iterator[responseType], dataType) iter.Seq2[resultType, int],
) *Correctable[resultType] {
	corr := &Correctable[resultType]{donech: make(chan struct{}, 1)}
	go func() {
		for result, level := range corrFunction(iter, data) {
			corr.set(result, level, nil, false)
		}
		corr.done = true
		close(corr.donech)
		for _, watcher := range corr.watchers {
			if watcher != nil {
				close(watcher.ch)
			}
		}
	}()
	return corr
}
