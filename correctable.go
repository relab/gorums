package gorums

import (
	"sync"

	"github.com/relab/gorums/ordering"
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
	config := ctx.Configuration()
	callOpts := getCallOptions(E_Correctable, opts...)

	md := ordering.NewGorumsMetadata(ctx, config.getMsgID(), method)
	replyChan := make(chan NodeResponse[msg], len(config))

	// Create ClientCtx first so sendOnce can access it
	clientCtx := newClientCtx[Req, Resp](ctx, config, req, method, replyChan)

	// Create sendOnce function that will be called lazily on first Responses() call
	sendOnce := func() {
		var expected int
		for _, n := range config {
			// Apply registered request transformations (if any)
			msg := clientCtx.applyTransforms(req, n)
			if msg == nil {
				continue // Skip node if transformation function returns nil
			}
			expected++
			n.channel.enqueue(request{ctx: ctx, msg: NewRequestMessage(md, msg), streaming: false, responseChan: replyChan})
		}
		clientCtx.expectedReplies = expected
	}

	// Wrap sendOnce with sync.OnceFunc to ensure it's only called once
	clientCtx.sendOnce = sync.OnceFunc(sendOnce)

	// Create the Responses object and apply interceptors
	responses := &Responses[Req, Resp]{ctx: clientCtx}
	for _, ic := range callOpts.interceptors {
		interceptor := ic.(QuorumInterceptor[Req, Resp])
		interceptor(responses)
	}

	// Default to majority threshold for correctable
	quorumSize := clientCtx.Size()/2 + 1
	return responses.WaitForLevel(quorumSize)
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
	config := ctx.Configuration()
	callOpts := getCallOptions(E_Correctable, opts...)

	md := ordering.NewGorumsMetadata(ctx, config.getMsgID(), method)
	// Buffer more messages for streaming
	chanSize := len(config) * 10
	replyChan := make(chan NodeResponse[msg], chanSize)

	// Create ClientCtx first so sendOnce can access it
	clientCtx := newClientCtx[Req, Resp](ctx, config, req, method, replyChan)
	clientCtx.responseSeq = clientCtx.streamingResponseSeq()

	// Create sendOnce function that will be called lazily on first Responses() call
	sendOnce := func() {
		var expected int
		for _, n := range config {
			// Apply registered request transformations (if any)
			msg := clientCtx.applyTransforms(req, n)
			if msg == nil {
				continue // Skip node if transformation function returns nil
			}
			expected++
			n.channel.enqueue(request{ctx: ctx, msg: NewRequestMessage(md, msg), streaming: true, responseChan: replyChan})
		}
		clientCtx.expectedReplies = expected
	}

	// Wrap sendOnce with sync.OnceFunc to ensure it's only called once
	clientCtx.sendOnce = sync.OnceFunc(sendOnce)

	// Create the Responses object and apply interceptors
	responses := &Responses[Req, Resp]{ctx: clientCtx}
	for _, ic := range callOpts.interceptors {
		interceptor := ic.(QuorumInterceptor[Req, Resp])
		interceptor(responses)
	}

	// Default to majority threshold for correctable
	quorumSize := clientCtx.Size()/2 + 1
	return responses.WaitForLevel(quorumSize)
}
