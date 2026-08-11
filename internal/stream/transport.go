package stream

import "sync/atomic"

// channelRef is an atomically replaceable reference to a node's current
// channel. The same channelRef is referenced by an inbound peer node's
// transport and by any borrower transport derived from it, so a channel
// replacement on peer reconnect is immediately visible to both.
type channelRef struct {
	ptr atomic.Pointer[Channel]
}

// load returns the current channel; it is safe on a nil channelRef.
func (r *channelRef) load() *Channel {
	if r == nil {
		return nil
	}
	return r.ptr.Load()
}

// store replaces the current channel.
func (r *channelRef) store(ch *Channel) {
	r.ptr.Store(ch)
}

// Transport bundles everything a call needs to reach one node: the node ID, the
// channel reference, the response router, and the message-ID generator. A
// node's transport is fixed at construction; only the channel behind the shared
// channel reference changes as streams come and go. The shared flag marks a
// transport that borrows these resources from an inbound peer node.
type Transport struct {
	id       uint32
	channel  *channelRef
	router   *MessageRouter
	msgIDGen func() uint64
	shared   bool
}

// NewTransport returns an owned transport with an empty channel reference; the
// caller stores the channel with [Transport.StoreChannel] once it is created.
func NewTransport(id uint32, msgIDGen func() uint64, router *MessageRouter) *Transport {
	return &Transport{
		id:       id,
		channel:  new(channelRef),
		router:   router,
		msgIDGen: msgIDGen,
	}
}

// NewSharedTransport derives a borrower transport from a peer's transport,
// referencing the peer's channel, router, and server-space message-ID
// generator, and keeping the peer's node ID. Channel replacement on peer
// reconnect is observed through the shared channel reference; the router is
// owned by the peer and stable across reconnects.
func NewSharedTransport(peer *Transport) *Transport {
	return &Transport{
		id:       peer.id,
		channel:  peer.channel,
		router:   peer.router,
		msgIDGen: peer.msgIDGen,
		shared:   true,
	}
}

// IsShared reports whether this transport borrows an inbound peer node's
// channel rather than owning its own outbound connection. It is safe on a nil
// transport.
func (t *Transport) IsShared() bool {
	return t != nil && t.shared
}

// Router returns the transport's response router, or nil on a nil transport.
func (t *Transport) Router() *MessageRouter {
	if t == nil {
		return nil
	}
	return t.router
}

// NextMsgID returns the next message ID from the transport's ID space: the
// manager's client-initiated space for an owned transport, or the peer's
// server-initiated space for a shared transport.
func (t *Transport) NextMsgID() uint64 {
	return t.msgIDGen()
}

// LoadChannel returns the transport's current channel, or nil if the transport
// has no attached channel. It is safe on a nil transport.
func (t *Transport) LoadChannel() *Channel {
	if t == nil {
		return nil
	}
	return t.channel.load()
}

// StoreChannel replaces the transport's current channel.
func (t *Transport) StoreChannel(ch *Channel) {
	t.channel.store(ch)
}

// Enqueue sends req on the current channel. Without a channel, a shared
// transport fails the request with [ErrStreamDown] since a borrower cannot
// dial, while an owned transport silently drops it. It is safe on a nil
// transport (zero-value node).
func (t *Transport) Enqueue(req Request) {
	if t == nil {
		return
	}
	ch := t.channel.load()
	if ch == nil {
		if t.shared {
			req.ReplyError(t.id, ErrStreamDown)
		}
		return
	}
	ch.Enqueue(req)
}

// TrySend sends req on the current channel without ever blocking the caller;
// see [Channel.TrySend]. Otherwise identical to [Transport.Enqueue]: without a
// channel, a shared transport fails the request with [ErrStreamDown], while an
// owned transport silently drops it. It is safe on a nil transport.
func (t *Transport) TrySend(req Request) {
	if t == nil {
		return
	}
	ch := t.channel.load()
	if ch == nil {
		if t.shared {
			req.ReplyError(t.id, ErrStreamDown)
		}
		return
	}
	ch.TrySend(req)
}

// Close cancels all pending calls in the router and closes the owned channel.
// Closing a shared transport is a no-op: the channel and router belong to the
// inbound peer node. It is safe on a nil transport.
func (t *Transport) Close() error {
	if t == nil || t.shared {
		return nil
	}
	if t.router != nil {
		for _, req := range t.router.CancelPending() {
			req.ReplyError(t.id, ErrNodeClosed)
		}
	}
	if ch := t.channel.load(); ch != nil {
		return ch.Close()
	}
	return nil
}
