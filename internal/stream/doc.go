// Package stream implements the wire-level transport that carries Gorums
// messages over gRPC bidirectional streams.
//
// It provides ordered, multiplexed message delivery for a single node through
// a [Channel] of one of three kinds: an [OutboundChannel] over streams this
// node dials, an [InboundChannel] over a stream its server accepted, and a
// [LocalChannel] that serves requests in-process. Each stream channel sends
// queued requests on its stream, matches responses to pending calls, and runs
// the peer's requests through a handler one at a time. The package also
// provides the per-node [Transport] that bundles a channel reference, a
// [Latency] estimate, and a message-ID generator; the server side ([Server],
// [BidiStream]) that accepts inbound streams; and the [Message] envelope and
// message-ID space shared by both directions.
//
// It sits below the connectivity layer in
// [github.com/relab/gorums/internal/conn] and knows nothing about nodes,
// configurations, or quorum calls.
package stream
