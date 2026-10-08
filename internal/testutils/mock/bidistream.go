package mock

import (
	"io"
	"sync"
)

// BidiStream is a bidirectional stream double for message type M. Recv
// returns the messages queued by [BidiStream.Deliver] in order, and io.EOF
// once [BidiStream.Close] is called. The constructor selects what Send does:
//   - [NewBidiStream] discards sent messages.
//   - [NewRecordingBidiStream] records them for [BidiStream.Sent].
//   - [NewGatedBidiStream] reports each message on [BidiStream.Entered], waits
//     for [BidiStream.Release], and then records it for [BidiStream.Sent].
//
// Close ends every wait. The type *BidiStream[*stream.Message] satisfies
// stream.BidiStream.
type BidiStream[M any] struct {
	in          chan M        // messages for Recv
	out         chan M        // sent messages; nil unless the stream records
	entered     chan M        // messages whose Send started; nil unless gated
	released    chan struct{} // closed by Release; nil unless gated
	done        chan struct{}
	releaseOnce sync.Once
	closeOnce   sync.Once
}

// bufferSize is the number of messages a [BidiStream] queues on each of its
// channels before Deliver or Send blocks.
const bufferSize = 16

func newBidiStream[M any]() *BidiStream[M] {
	return &BidiStream[M]{in: make(chan M, bufferSize), done: make(chan struct{})}
}

// NewBidiStream returns an open [BidiStream] that discards sent messages.
func NewBidiStream[M any]() *BidiStream[M] {
	return newBidiStream[M]()
}

// NewRecordingBidiStream returns an open [BidiStream] that records sent
// messages; read them from [BidiStream.Sent].
func NewRecordingBidiStream[M any]() *BidiStream[M] {
	s := newBidiStream[M]()
	s.out = make(chan M, bufferSize)
	return s
}

// NewGatedBidiStream returns an open [BidiStream] whose Send blocks until
// [BidiStream.Release] or [BidiStream.Close], like a transport whose peer
// stopped reading. Each Send reports its message on [BidiStream.Entered]
// when it starts, and records it for [BidiStream.Sent] once released.
func NewGatedBidiStream[M any]() *BidiStream[M] {
	s := NewRecordingBidiStream[M]()
	s.entered = make(chan M, bufferSize)
	s.released = make(chan struct{})
	return s
}

// Send discards, records, or holds msg, as selected by the constructor.
// Send returns io.EOF if s is closed while Send waits.
func (s *BidiStream[M]) Send(msg M) error {
	if s.entered != nil {
		if err := s.enqueue(s.entered, msg); err != nil {
			return err
		}
		select {
		case <-s.released:
		case <-s.done:
			return io.EOF
		}
	}
	if s.out == nil {
		return nil
	}
	return s.enqueue(s.out, msg)
}

// Deliver queues msg for Recv, as if the peer sent it. It returns io.EOF if s
// is closed while Deliver waits for buffer space.
func (s *BidiStream[M]) Deliver(msg M) error {
	return s.enqueue(s.in, msg)
}

func (s *BidiStream[M]) enqueue(ch chan M, msg M) error {
	select {
	case ch <- msg:
		return nil
	case <-s.done:
		return io.EOF
	}
}

// Recv returns the next queued message, or io.EOF once s is closed.
func (s *BidiStream[M]) Recv() (M, error) {
	select {
	case msg := <-s.in:
		return msg, nil
	case <-s.done:
		var zero M
		return zero, io.EOF
	}
}

// Sent returns the channel of messages sent on a stream created by
// [NewRecordingBidiStream] or [NewGatedBidiStream]. For other streams it
// returns nil.
func (s *BidiStream[M]) Sent() <-chan M { return s.out }

// Entered returns the channel of messages whose Send started on a stream
// created by [NewGatedBidiStream]. For other streams it returns nil.
func (s *BidiStream[M]) Entered() <-chan M { return s.entered }

// Release lets every waiting and future Send on a gated stream complete. It
// is safe to call Release more than once, and it does nothing on a stream
// that is not gated.
func (s *BidiStream[M]) Release() {
	if s.released != nil {
		s.releaseOnce.Do(func() { close(s.released) })
	}
}

// Close closes s, which makes Recv return io.EOF and ends every wait in Send
// and Deliver. It is safe to call Close more than once.
func (s *BidiStream[M]) Close() { s.closeOnce.Do(func() { close(s.done) }) }
