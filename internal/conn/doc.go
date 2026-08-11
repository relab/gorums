// Package conn implements the client-side connectivity layer for Gorums.
//
// It owns the node and configuration model and the machinery that keeps them
// connected: [Node] and [Config] and their contexts ([NodeContext],
// [ConfigContext]); the [NodeSource] builders ([WithNodes], [WithNodeList]);
// the outbound connection manager and the inbound peer manager
// ([InboundManager]); dial options ([DialOptions]); and stream deduplication,
// where a higher-ID peer borrows a lower-ID peer's dialed stream. It also
// defines the call result errors that reference nodes ([QuorumCallError],
// [NodeError]).
//
// The package sits below the call engine in
// [github.com/relab/gorums/internal/impl] and exposes the send path to it
// through [NodeTransport]. Package [github.com/relab/gorums] re-exports the
// user-facing types here as aliases; application code must not import this
// package directly.
package conn
