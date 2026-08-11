package impl

// Async represents the eventual result of an asynchronous quorum call.
type Async[Resp any] struct {
	reply Resp
	err   error
	c     chan struct{}
}

// Get blocks until the call completes and returns its response and error.
func (f *Async[Resp]) Get() (Resp, error) {
	<-f.c
	return f.reply, f.err
}

// Done reports whether the call has completed.
func (f *Async[Resp]) Done() bool {
	select {
	case <-f.c:
		return true
	default:
		return false
	}
}

// AsyncFirst returns an Async future that resolves when the first response is received.
// Messages are sent immediately (synchronously) to preserve ordering.
func (r *Responses[Resp]) AsyncFirst() *Async[Resp] {
	return r.AsyncThreshold(1)
}

// AsyncMajority returns an Async future that resolves when a majority quorum is reached.
// Messages are sent immediately (synchronously) to preserve ordering when multiple
// async calls are created in sequence.
func (r *Responses[Resp]) AsyncMajority() *Async[Resp] {
	quorumSize := r.size/2 + 1
	return r.AsyncThreshold(quorumSize)
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
	// Send messages synchronously before spawning the goroutine to preserve ordering
	r.sendNow()

	fut := &Async[Resp]{c: make(chan struct{}, 1)}

	go func() {
		defer close(fut.c)
		fut.reply, fut.err = r.Threshold(threshold)
	}()

	return fut
}
