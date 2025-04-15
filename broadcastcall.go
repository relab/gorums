package gorums

import (
	"context"
	"slices"

	"github.com/relab/gorums/ordering"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// BroadcastCallData holds the message, destination nodes, method identifier,
// and other information necessary to perform the various quorum call types
// supported by Gorums.
type BroadcastCallData struct {
	Message           protoreflect.ProtoMessage
	Method            string
	BroadcastID       uint64 // a unique identifier for the current broadcast request
	IsBroadcastClient bool
	SenderAddr        string
	OriginAddr        string
	OriginMethod      string
	OriginPubKey      string
	OriginSignature   []byte
	OriginDigest      []byte
	ServerAddresses   []string
	SkipSelf          bool
}

// inSubset returns true if the given address is in the given subset
// of server addresses. Will return true if a subset is not given.
func (bcd *BroadcastCallData) inSubset(addr string) bool {
	if bcd == nil || len(bcd.ServerAddresses) <= 0 {
		return true
	}
	return slices.Contains(bcd.ServerAddresses, addr)
}

// BroadcastCall performs a broadcast on the configuration.
//
// This method should be used by generated code only.
func (c RawConfiguration) BroadcastCall(ctx context.Context, d BroadcastCallData, opts ...CallOption) {
	md := ordering.NewGorumsMetadata(ctx, c.getMsgID(), d.Method)
	broadcastMsg := ordering.BroadcastMsg_builder{
		IsBroadcastClient: d.IsBroadcastClient,
		BroadcastID:       d.BroadcastID,
		SenderAddr:        d.SenderAddr,
		OriginAddr:        d.OriginAddr,
		OriginMethod:      d.OriginMethod,
		OriginPubKey:      d.OriginPubKey,
		OriginSignature:   d.OriginSignature,
		OriginDigest:      d.OriginDigest,
	}.Build()
	md.SetBroadcastMsg(broadcastMsg)

	msg := &Message{Metadata: md, Message: d.Message}
	o := getCallOptions(E_Broadcast, opts)
	c.sign(msg, o.signOrigin)

	var replyChan chan response
	if !o.noSendWaiting {
		replyChan = make(chan response, len(c))
	}
	sentMsgs := 0
	notEnqueued := make([]*RawNode, 0, len(c))
	for _, n := range c {
		if d.SkipSelf && n.Address() == d.SenderAddr {
			// the node will not send the message to itself
			continue
		}
		// skip nodes not specified in subset
		if !d.inSubset(n.addr) {
			continue
		}
		sentMsgs++
		// do NOT enqueue in a goroutine. This inhibits ordering constraints.
		// the message will only be enqueued if the channel has enough capacity
		// or if the receiver is ready. This prevents a slow node from limiting the
		// enqueueing of messages to other nodes while still ensuring correct
		// ordering of messages.
		//
		// NOTE: the slow path will be invoked even though we buffer the channel. Hence,
		// the enqueueFast will provide a small performance benefit.
		enqueued := n.channel.enqueueFast(request{ctx: ctx, msg: msg, opts: o}, replyChan, false)
		if !enqueued {
			notEnqueued = append(notEnqueued, n)
		}
	}

	// it is important to retry the enqueueing for slow nodes. the method
	// will block until the message is enqueued.
	// NOTE: enqueueFast() creates a responseRouter and thus it is not
	// necessary to provide the replyChan to enqueueSlow().
	for _, n := range notEnqueued {
		n.channel.enqueueSlow(request{ctx: ctx, msg: msg, opts: o})
	}

	// if noSendWaiting is set, we will not wait for confirmation from the channel before returning.
	if o.noSendWaiting {
		return
	}

	// wait until all requests have been sent
	for ; sentMsgs > 0; sentMsgs-- {
		<-replyChan
	}
}
