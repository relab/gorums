package gorums

import "google.golang.org/protobuf/proto"

// Async encapsulates the state of an asynchronous quorum call,
// and has methods for checking the status of the call or waiting for it to complete.
//
// This struct should only be used by generated code.
type Async[resultType any] struct {
	reply resultType
	err   error
	c     chan struct{}
}

// Get returns the reply and any error associated with the called method.
// The method blocks until a reply or error is available.
func (f *Async[resultType]) Get() (resultType, error) {
	<-f.c
	return f.reply, f.err
}

// Done reports if a reply and/or error is available for the called method.
func (f *Async[resultType]) Done() bool {
	select {
	case <-f.c:
		return true
	default:
		return false
	}
}

func IterAsync[responseType proto.Message, resultType any](
	iter Iterator[responseType],
	asyncFunc func(Iterator[responseType]) resultType,
) Async[resultType] {
	async := Async[resultType]{
		c: make(chan struct{}),
	}

	go func() {
		async.reply = asyncFunc(iter)

		close(async.c)
	}()

	return async
}
