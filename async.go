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
func (r *Responses[T, Resp]) AsyncMajority() *Async[Resp] {
	quorumSize := r.size/2 + 1
	return r.AsyncThreshold(quorumSize)
}

// AsyncFirst returns an Async future that resolves when the first response is received.
// Messages are sent immediately (synchronously) to preserve ordering.
func (r *Responses[T, Resp]) AsyncFirst() *Async[Resp] {
	return r.AsyncThreshold(1)
}

// AsyncAll returns an Async future that resolves when all nodes have responded.
// Messages are sent immediately (synchronously) to preserve ordering.
func (r *Responses[T, Resp]) AsyncAll() *Async[Resp] {
	return r.AsyncThreshold(r.size)
}

// AsyncThreshold returns an Async future that resolves when the threshold is reached.
// Messages are sent immediately (synchronously) to preserve ordering when multiple
// async calls are created in sequence.
func (r *Responses[T, Resp]) AsyncThreshold(threshold int) *Async[Resp] {
	// Send messages synchronously before spawning the goroutine to preserve ordering
	r.sendNow()

	fut := &Async[Resp]{c: make(chan struct{}, 1)}

	go func() {
		defer close(fut.c)
		fut.reply, fut.err = r.Threshold(threshold)
	}()

	return fut
}
