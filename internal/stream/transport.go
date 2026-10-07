package stream

import "sync/atomic"

// channelRef is an atomically replaceable reference to a node's current
// channel. A node's transport and every borrower transport derived from it
// share one channelRef, so a channel replacement is visible to all of them.
type channelRef struct {
	ptr atomic.Pointer[Channel]
}

// load returns the current channel, or nil.
func (r *channelRef) load() Channel {
	if p := r.ptr.Load(); p != nil {
		return *p
	}
	return nil
}

// store replaces the current channel; a nil ch clears it.
func (r *channelRef) store(ch Channel) {
	if ch == nil {
		r.ptr.Store(nil)
		return
	}
	r.ptr.Store(&ch)
}

// Transport bundles everything a call needs to reach one node: the node ID,
// the channel reference, the latency estimate, and the message-ID generator.
// A node's transport is fixed at construction; only the channel behind the
// shared channel reference changes as streams come and go. A shared transport
// borrows these resources from an inbound peer node's transport.
type Transport struct {
	id       uint32
	channel  *channelRef
	latency  *Latency
	msgIDGen func() uint64
	shared   bool
}

// NewTransport returns an owned transport with no channel; attach one with
// [Transport.StoreChannel].
func NewTransport(id uint32, msgIDGen func() uint64) *Transport {
	return &Transport{
		id:       id,
		channel:  new(channelRef),
		latency:  newLatency(),
		msgIDGen: msgIDGen,
	}
}

// NewSharedTransport returns a borrower transport that shares peer's node ID,
// channel reference, latency estimate, and message-ID generator.
func NewSharedTransport(peer *Transport) *Transport {
	return &Transport{
		id:       peer.id,
		channel:  peer.channel,
		latency:  peer.latency,
		msgIDGen: peer.msgIDGen,
		shared:   true,
	}
}

// IsShared reports whether t borrows another transport's channel. It is safe
// on a nil transport.
func (t *Transport) IsShared() bool {
	return t != nil && t.shared
}

// Latency returns the transport's latency estimate, or nil on a nil transport.
func (t *Transport) Latency() *Latency {
	if t == nil {
		return nil
	}
	return t.latency
}

// NextMsgID returns the next message ID from the transport's ID space.
func (t *Transport) NextMsgID() uint64 {
	return t.msgIDGen()
}

// LoadChannel returns the current channel, or nil if there is none or t is nil.
func (t *Transport) LoadChannel() Channel {
	if t == nil {
		return nil
	}
	return t.channel.load()
}

// StoreChannel replaces the current channel; a nil ch clears it.
func (t *Transport) StoreChannel(ch Channel) {
	t.channel.store(ch)
}

// Enqueue sends req on the current channel; see [Channel.Enqueue]. Without a
// channel, or on a nil transport, it fails req with [ErrStreamDown].
func (t *Transport) Enqueue(req Request) {
	ch := t.LoadChannel()
	if ch == nil {
		var id uint32
		if t != nil {
			id = t.id
		}
		req.sendErrorResponse(id, ErrStreamDown)
		return
	}
	ch.Enqueue(req)
}

// Close closes the owned channel. Closing a shared transport, whose channel
// belongs to the inbound peer node, does nothing. It is safe on a nil
// transport.
func (t *Transport) Close() error {
	if t == nil || t.shared {
		return nil
	}
	if ch := t.channel.load(); ch != nil {
		return ch.Close()
	}
	return nil
}
