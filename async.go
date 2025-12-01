package gorums

// Async is a generic future type for asynchronous quorum calls.
// It encapsulates the state of an asynchronous call and provides methods
// for checking the status or waiting for completion.
//
// Type parameter Resp is the response type from nodes.
type Async[Resp any] struct {
	reply Resp
	err   error
	c     chan struct{}
}

// Get returns the reply and any error associated with the called method.
// The method blocks until a reply or error is available.
func (f *Async[Resp]) Get() (Resp, error) {
	<-f.c
	return f.reply, f.err
}

// Done reports if a reply and/or error is available for the called method.
func (f *Async[Resp]) Done() bool {
	select {
	case <-f.c:
		return true
	default:
		return false
	}
}

// AsyncMajority returns an Async future that resolves when a majority quorum is reached.
// Messages are sent immediately (synchronously) to preserve ordering when multiple
// async calls are created in sequence.
func (r *Responses[Resp]) AsyncMajority() *Async[Resp] {
	quorumSize := r.size/2 + 1
	return r.AsyncThreshold(quorumSize)
}

// AsyncFirst returns an Async future that resolves when the first response is received.
// Messages are sent immediately (synchronously) to preserve ordering.
func (r *Responses[Resp]) AsyncFirst() *Async[Resp] {
	return r.AsyncThreshold(1)
}

// AsyncAll returns an Async future that resolves when all nodes have responded.
// Messages are sent immediately (synchronously) to preserve ordering.
func (r *Responses[Resp]) AsyncAll() *Async[Resp] {
	return r.AsyncThreshold(r.size)
}

// AsyncThreshold returns an Async future that resolves when the threshold is reached.
// Messages are sent immediately (synchronously) to preserve ordering when multiple
// async calls are created in sequence.
func (r *Responses[Resp]) AsyncThreshold(threshold int) *Async[Resp] {
	fut := &Async[Resp]{c: make(chan struct{}, 1)}

	go func() {
		defer close(fut.c)
		fut.reply, fut.err = r.Threshold(threshold)
	}()

	return fut
}

// AsyncCall performs an asynchronous quorum call and returns an Async future.
// Messages are sent immediately (synchronously) to preserve ordering when multiple
// async calls are created in sequence.
//
// This function should be used by generated code only.
func AsyncCall[Req, Resp msg](
	ctx *ConfigContext,
	req Req,
	method string,
	opts ...CallOption,
) *Async[Resp] {
	callOpts := getCallOptions(E_Async, opts...)
	clientCtx := newClientCtxBuilder[Req, Resp](ctx, req, method).Build()

	// Send messages to all nodes synchronously (before spawning goroutine).
	// This ensures message ordering is preserved when multiple async calls
	// are created in sequence.
	clientCtx.sendOnce.Do(clientCtx.send)

	// Apply interceptors
	for _, ic := range callOpts.interceptors {
		interceptor := ic.(QuorumInterceptor[Req, Resp])
		interceptor(clientCtx)
	}

	// Return async majority by default (matching old behavior)
	return NewResponses(clientCtx).AsyncMajority()
}
