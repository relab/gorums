// Package stream implements the wire-level transport that carries Gorums
// messages over gRPC bidirectional streams.
//
// It provides ordered, multiplexed message delivery for a single node: the
// [Channel] that owns the send queue and stream lifecycle, the [MessageRouter]
// that matches responses to pending calls and tracks latency, the per-node
// [Transport] that bundles a channel reference, router, and message-ID
// generator, the server side ([Server], [BidiStream]) that accepts inbound
// streams, and the [Message] envelope and message-ID space shared by both
// directions.
//
// It sits below the connectivity layer in
// [github.com/relab/gorums/internal/conn] and knows nothing about nodes,
// configurations, or quorum calls.
package stream
