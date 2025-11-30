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

func (c *Correctable[Out]) set(reply Out, level int, done bool, err error) {
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

// CorrectableQuorumFunc processes a correctable quorum call and returns the aggregated result.
// Unlike regular QuorumFunc, this also returns an integer level indicating the consistency level
// achieved, and a boolean indicating whether the call is complete.
//
// Type parameters:
//   - Req: The request message type sent to nodes
//   - Resp: The response message type from individual nodes
//   - Out: The final output type returned by the interceptor chain
type CorrectableQuorumFunc[Req, Resp msg, Out any] func(*ClientCtx[Req, Resp]) (Out, int, bool, error)

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
	base CorrectableQuorumFunc[Req, Resp, Out],
	opts ...CallOption,
) *Correctable[Out] {
	config := ctx.Configuration()
	callOpts := getCallOptions(E_Correctable, opts...)

	// Use CorrectableQuorumFunc from CallOptions if provided, otherwise use the base parameter
	qf := base
	if callOpts.correctableQuorumFunc != nil {
		qf = callOpts.correctableQuorumFunc.(CorrectableQuorumFunc[Req, Resp, Out])
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

	// Create the correctable result
	corr := &Correctable[Out]{
		level:  LevelNotSet,
		donech: make(chan struct{}, 1),
	}

	// Run the correctable call in a goroutine
	go func() {
		// Process responses using the correctable quorum function
		// The quorum function can return multiple times with increasing levels
		// until it indicates completion (done=true)
		corr.set(qf(clientCtx))
	}()

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
	base CorrectableQuorumFunc[Req, Resp, Out],
	opts ...CallOption,
) *Correctable[Out] {
	config := ctx.Configuration()
	callOpts := getCallOptions(E_Correctable, opts...)
	qf := base
	if callOpts.correctableQuorumFunc != nil {
		qf = callOpts.correctableQuorumFunc.(CorrectableQuorumFunc[Req, Resp, Out])
	}
	// gorums.MajorityCorrectableQuorum[*CorrectableRequest, *CorrectableResponse],

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

	// Create the correctable result
	corr := &Correctable[Out]{
		level:  LevelNotSet,
		donech: make(chan struct{}, 1),
	}

	// Run the correctable call in a goroutine
	go func() {
		// For streaming calls, clean up routers when done
		defer func() {
			for _, n := range config {
				n.channel.deleteRouter(md.GetMessageID())
			}
		}()

		// Process responses using the correctable quorum function
		// The quorum function can return multiple times with increasing levels
		// until it indicates completion (done=true)
		corr.set(qf(clientCtx))
	}()

	return corr
}
