package stream

import "time"

// This file collects exported constructors that exist only to support tests in
// other packages (package stream's own tests use unexported helpers directly).
// They live in a non-test file because Go test files are not importable across
// packages; keeping them here separates them from production code.

// NewChannelWithState creates a new Channel with a specific state for testing.
// This function should only be used in tests.
func NewChannelWithState(lastErr error) *Channel {
	return &Channel{
		lastError: lastErr,
	}
}

// NewMessageRouterWithLatency creates a new MessageRouter with an initial latency
// for testing. The latency may be updated by subsequent message routing operations.
// This function should only be used in tests.
//
// To change the latency after creation, use [MessageRouter.SetLatency].
func NewMessageRouterWithLatency(latency time.Duration) *MessageRouter {
	return &MessageRouter{
		pending: make(map[uint64]pendingRequest),
		latency: latency,
	}
}

// NewSharedTransportWithGen is like [NewSharedTransport] but overrides the
// message-ID generator, so a test can simulate a deduplicated transport that
// draws IDs from a server-initiated space while reusing an existing channel.
// This function should only be used in tests.
func NewSharedTransportWithGen(peer *Transport, msgIDGen func() uint64) *Transport {
	t := NewSharedTransport(peer)
	t.msgIDGen = msgIDGen
	return t
}
