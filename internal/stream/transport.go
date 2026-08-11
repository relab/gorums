package stream

import "sync/atomic"

// channelRef is an atomically replaceable reference to a node's current
// channel, so a channel replacement on reconnect is immediately visible to
// every holder.
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
// node's transport is fixed at construction; only the channel behind the
// channel reference changes as streams come and go.
type Transport struct {
	id       uint32
	channel  *channelRef
	router   *MessageRouter
	msgIDGen func() uint64
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

// Router returns the transport's response router, or nil on a nil transport.
func (t *Transport) Router() *MessageRouter {
	if t == nil {
		return nil
	}
	return t.router
}

// NextMsgID returns the next message ID from the transport's ID space.
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

// Enqueue sends req on the current channel, and silently drops the request if
// the transport has no channel. It is safe on a nil transport (zero-value
// node).
func (t *Transport) Enqueue(req Request) {
	if t == nil {
		return
	}
	if ch := t.channel.load(); ch != nil {
		ch.Enqueue(req)
	}
}

// TrySend sends req on the current channel without ever blocking the caller;
// see [Channel.TrySend]. Otherwise identical to [Transport.Enqueue]. It is safe
// on a nil transport.
func (t *Transport) TrySend(req Request) {
	if t == nil {
		return
	}
	if ch := t.channel.load(); ch != nil {
		ch.TrySend(req)
	}
}

// Close cancels all pending calls in the router and closes the channel. It is
// safe on a nil transport.
func (t *Transport) Close() error {
	if t == nil {
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
