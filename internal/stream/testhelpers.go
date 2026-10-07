package stream

// This file collects exported constructors that exist only to support tests in
// other packages (package stream's own tests use unexported helpers directly).
// They live in a non-test file because Go test files are not importable across
// packages; keeping them here separates them from production code.

// NewChannelWithState returns a Channel without a stream whose
// [Channel.LastErr] reports lastErr. This function should only be used in tests.
func NewChannelWithState(lastErr error) Channel {
	return stateChannel{lastErr: lastErr}
}

// stateChannel is a [Channel] that only reports a fixed health state.
type stateChannel struct {
	lastErr error
}

func (stateChannel) Enqueue(req Request)   { req.replyError(0, ErrStreamDown) }
func (stateChannel) StreamUp() bool        { return false }
func (c stateChannel) LastErr() error      { return c.lastErr }
func (stateChannel) DroppedReplies() int64 { return 0 }
func (stateChannel) PendingCount() int     { return 0 }
func (stateChannel) Close() error          { return nil }

// NewSharedTransportWithGen is like [NewSharedTransport] but overrides the
// message-ID generator, so a test can simulate a deduplicated transport that
// draws IDs from a server-initiated space while reusing an existing channel.
// This function should only be used in tests.
func NewSharedTransportWithGen(peer *Transport, msgIDGen func() uint64) *Transport {
	t := NewSharedTransport(peer)
	t.msgIDGen = msgIDGen
	return t
}
