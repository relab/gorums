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
// blocks until a message is available or [BidiStream.Close] is called, and
// returns io.EOF after Close. A stream from [NewBidiStream] discards every sent
// message; a stream from [NewEchoBidiStream] returns sent messages from Recv in
// order. The type *BidiStream[*stream.Message] satisfies stream.BidiStream.
type BidiStream[M any] struct {
	echo      chan M // nil unless the stream echoes
	done      chan struct{}
	closeOnce sync.Once
}

// NewBidiStream returns an open [BidiStream] that discards sent messages.
func NewBidiStream[M any]() *BidiStream[M] {
	return &BidiStream[M]{done: make(chan struct{})}
}

// NewEchoBidiStream returns an open [BidiStream] that returns sent messages
// from Recv. Send blocks while 16 sent messages wait to be received.
func NewEchoBidiStream[M any]() *BidiStream[M] {
	return &BidiStream[M]{echo: make(chan M, 16), done: make(chan struct{})}
}

// Send discards msg, or queues it for Recv if s echoes. Send on a closed
// echoing stream returns io.EOF.
func (s *BidiStream[M]) Send(msg M) error {
	if s.echo == nil {
		return nil
	}
	select {
	case s.echo <- msg:
		return nil
	case <-s.done:
		return io.EOF
	}
}

// Recv returns the next echoed message, or io.EOF once s is closed.
func (s *BidiStream[M]) Recv() (M, error) {
	select {
	case msg := <-s.echo:
		return msg, nil
	case <-s.done:
		var zero M
		return zero, io.EOF
	}
}

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
