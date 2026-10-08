package mock

import (
	"io"
	"slices"
	"sync"
	"testing"
)

// The test doubles in this file must not import internal/stream or
// internal/conn: package-internal tests of both import this package.

// BidiStream is a bidirectional stream double for message type M. Recv
// returns the messages queued by [BidiStream.Deliver] in order, and io.EOF
// once [BidiStream.Close] is called. A stream from [NewBidiStream] discards
// sent messages; a stream from [NewRecordingBidiStream] records them for
// [BidiStream.Sent]. The type *BidiStream[*stream.Message] satisfies
// stream.BidiStream.
type BidiStream[M any] struct {
	in        chan M // messages for Recv
	out       chan M // sent messages; nil unless the stream records
	done      chan struct{}
	closeOnce sync.Once
}

// bufferSize is the number of messages a [BidiStream] queues for Recv or
// records for Sent before Deliver or Send blocks.
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

// Send discards msg, or records it if s was created by
// [NewRecordingBidiStream]. Send returns io.EOF if s is closed while Send
// waits for buffer space.
func (s *BidiStream[M]) Send(msg M) error {
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
// [NewRecordingBidiStream]. For other streams it returns nil.
func (s *BidiStream[M]) Sent() <-chan M { return s.out }

// Close closes s, which makes Recv return io.EOF. It is safe to call Close
// more than once.
func (s *BidiStream[M]) Close() { s.closeOnce.Do(func() { close(s.done) }) }

// NodeAddr is a node network address that implements conn.NodeAddress,
// so a map[uint32]NodeAddr can be passed to conn.WithNodes.
type NodeAddr string

// Addr returns a as a string.
func (a NodeAddr) Addr() string { return string(a) }

// CheckNodeIDs reports a test error if cfg.NodeIDs() is not equal to wantIDs.
// The error message starts with label.
func CheckNodeIDs(t testing.TB, cfg interface{ NodeIDs() []uint32 }, wantIDs []uint32, label string) {
	t.Helper()
	if got := cfg.NodeIDs(); !slices.Equal(got, wantIDs) {
		t.Errorf("%s: config IDs = %v; want %v", label, got, wantIDs)
	}
}
