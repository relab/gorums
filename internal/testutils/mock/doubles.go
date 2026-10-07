package mock

import (
	"io"
	"slices"
	"testing"
)

// The test doubles in this file must not import internal/stream or
// internal/conn: package-internal tests of both import this package.

// BidiStream is a bidirectional stream double for message type M.
// Send discards every message. Recv blocks until [BidiStream.Close] is called
// and then returns io.EOF. The type *BidiStream[*stream.Message] satisfies
// stream.BidiStream.
type BidiStream[M any] struct {
	done chan struct{}
}

// NewBidiStream returns an open [BidiStream].
func NewBidiStream[M any]() *BidiStream[M] {
	return &BidiStream[M]{done: make(chan struct{})}
}

// Send discards msg and returns nil.
func (*BidiStream[M]) Send(M) error { return nil }

// Recv blocks until s is closed and then returns io.EOF.
func (s *BidiStream[M]) Recv() (M, error) {
	<-s.done
	var zero M
	return zero, io.EOF
}

// Close closes s, which makes Recv return io.EOF. Call Close only once.
func (s *BidiStream[M]) Close() { close(s.done) }

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
