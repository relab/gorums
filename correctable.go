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
// Type parameter Out is the output type returned by the quorum function,
// which may differ from the RPC response type.
type Correctable[Out any] struct {
	mu       sync.Mutex
	reply    Out
	level    int
	err      error
	done     bool
	watchers []*watcher
	donech   chan struct{}
}

// NewCorrectable creates a new Correctable object.
func NewCorrectable[Out any]() *Correctable[Out] {
	return &Correctable[Out]{
		level:  LevelNotSet,
		donech: make(chan struct{}, 1),
	}
}

// Get returns the latest response, the current level, and the last error.
func (c *Correctable[Out]) Get() (Out, int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reply, c.level, c.err
}

// Done returns a channel that will close when the correctable call is completed.
func (c *Correctable[Out]) Done() <-chan struct{} {
	return c.donech
}

// Watch returns a channel that will close when the correctable call has reached a specified level.
func (c *Correctable[Out]) Watch(level int) <-chan struct{} {
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
func (c *Correctable[Out]) Update(reply Out, level int, done bool, err error) {
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

// CorrectableCall performs a correctable quorum call using an interceptor-based approach.
//
// Type parameters:
//   - Req: The request message type
//   - Resp: The response message type from individual nodes
//   - Out: The final output type returned by the quorum function
//
// The base parameter is the terminal handler that processes responses and determines
// consistency levels. The opts parameter accepts CallOption values.
//
// This function should be used by generated code only.
func CorrectableCall[Req, Resp msg, Out any](
	ctx *ConfigContext,
	req Req,
	method string,
	base QuorumFunc[Req, Resp, *Correctable[Out]],
	opts ...CallOption,
) *Correctable[Out] {
	config := ctx.Configuration()
	callOpts := getCallOptions(E_Correctable, opts...)

	// Use QuorumFunc from CallOptions if provided, otherwise use the base parameter
	qf := base
	if callOpts.quorumFunc != nil {
		qf = callOpts.quorumFunc.(QuorumFunc[Req, Resp, *Correctable[Out]])
	}

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

	// Execute the quorum function
	// The quorum function is responsible for creating the Correctable object,
	// spawning a goroutine to process responses, and returning the Correctable object.
	corr, err := qf(clientCtx)
	if err != nil {
		// If the quorum function fails immediately, return a Correctable with the error
		return &Correctable[Out]{err: err, done: true, donech: make(chan struct{})}
	}
	return corr
}

// CorrectableStreamCall performs a correctable streaming quorum call using an interceptor-based approach.
//
// Type parameters:
//   - Req: The request message type
//   - Resp: The response message type from individual nodes
//   - Out: The final output type returned by the quorum function
//
// The base parameter is the terminal handler that processes responses and determines
// consistency levels. The opts parameter accepts CallOption values.
//
// This function should be used by generated code only.
func CorrectableStreamCall[Req, Resp msg, Out any](
	ctx *ConfigContext,
	req Req,
	method string,
	base QuorumFunc[Req, Resp, *Correctable[Out]],
	opts ...CallOption,
) *Correctable[Out] {
	config := ctx.Configuration()
	callOpts := getCallOptions(E_Correctable, opts...)
	qf := base
	if callOpts.quorumFunc != nil {
		qf = callOpts.quorumFunc.(QuorumFunc[Req, Resp, *Correctable[Out]])
	}

	md := ordering.NewGorumsMetadata(ctx, config.getMsgID(), method)
	// Buffer more messages for streaming
	chanSize := len(config) * 10
	replyChan := make(chan NodeResponse[msg], chanSize)

	// Create ClientCtx first so sendOnce can access it
	clientCtx := newClientCtx[Req, Resp](ctx, config, req, method, replyChan)
	clientCtx.responses = clientCtx.streamingResponses()

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

	// Execute the quorum function
	// The quorum function is responsible for creating the Correctable object,
	// spawning a goroutine to process responses, and returning the Correctable object.
	// For streaming, the quorum function should also handle cleaning up routers.
	// However, cleaning up routers is tricky if qf spawns the goroutine.
	// The qf should probably defer cleanup in its goroutine.
	// But qf is user-provided (or generated).
	// We might need to wrap the qf or provide a helper that does cleanup.

	// Actually, the previous implementation did cleanup in the goroutine it spawned.
	// Now qf spawns the goroutine.
	// So qf must handle cleanup.
	// The generated code or helper should handle this.

	corr, err := qf(clientCtx)
	if err != nil {
		return &Correctable[Out]{err: err, done: true, donech: make(chan struct{})}
	}
	return corr
}
