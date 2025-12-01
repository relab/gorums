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

// Update sets the current state of the correctable call.
// It updates the response, level, and error, and notifies any watchers.
// If done is true, the call is considered complete and the Done channel is closed.
func (c *Correctable[Resp]) Update(reply Resp, level int, done bool, err error) {
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

// CorrectableCall performs a correctable quorum call and returns a Correctable object.
// The Correctable reports progress as responses arrive and completes when the
// majority threshold is reached.
//
// This function should be used by generated code only.
func CorrectableCall[Req, Resp msg](
	ctx *ConfigContext,
	req Req,
	method string,
	opts ...CallOption,
) *Correctable[Resp] {
	callOpts := getCallOptions(E_Correctable, opts...)
	clientCtx := newClientCtxBuilder[Req, Resp](ctx, req, method).Build()

	// Apply interceptors
	for _, ic := range callOpts.interceptors {
		interceptor := ic.(QuorumInterceptor[Req, Resp])
		interceptor(clientCtx)
	}

	// Default to majority threshold for correctable
	quorumSize := clientCtx.Size()/2 + 1
	return NewResponses(clientCtx).WaitForLevel(quorumSize)
}

// CorrectableStreamCall performs a correctable streaming quorum call.
// This is similar to CorrectableCall but uses a streaming response iterator
// that continues until the context is canceled.
//
// This function should be used by generated code only.
func CorrectableStreamCall[Req, Resp msg](
	ctx *ConfigContext,
	req Req,
	method string,
	opts ...CallOption,
) *Correctable[Resp] {
	callOpts := getCallOptions(E_Correctable, opts...)
	clientCtx := newClientCtxBuilder[Req, Resp](ctx, req, method).WithStreaming().Build()

	// Apply interceptors
	for _, ic := range callOpts.interceptors {
		interceptor := ic.(QuorumInterceptor[Req, Resp])
		interceptor(clientCtx)
	}

	// Default to majority threshold for correctable
	quorumSize := clientCtx.Size()/2 + 1
	return NewResponses(clientCtx).WaitForLevel(quorumSize)
}
