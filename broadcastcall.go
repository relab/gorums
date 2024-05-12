package gorums

import (
	"context"

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
	ServerAddresses   []string
}

// checks whether the given address is contained in the given subset
// of server addresses. Will return true if a subset is not given.
func (bcd *BroadcastCallData) inSubset(addr string) bool {
	if bcd.ServerAddresses == nil || len(bcd.ServerAddresses) <= 0 {
		return true
	}
	for _, srvAddr := range bcd.ServerAddresses {
		if addr == srvAddr {
			return true
		}
	}
	return false
}

// BroadcastCall performs a multicast call on the configuration.
func (c RawConfiguration) BroadcastCall(ctx context.Context, d BroadcastCallData) {
	md := &ordering.Metadata{MessageID: c.getMsgID(), Method: d.Method, BroadcastMsg: &ordering.BroadcastMsg{
		IsBroadcastClient: d.IsBroadcastClient,
		BroadcastID:       d.BroadcastID,
		SenderAddr:        d.SenderAddr,
		OriginAddr:        d.OriginAddr,
		OriginMethod:      d.OriginMethod,
	}}
	o := getCallOptions(E_Broadcast, nil)

	var replyChan chan response
	if !o.noSendWaiting {
		replyChan = make(chan response, len(c))
	}
	sentMsgs := 0
	notEnqueued := make([]*RawNode, 0, len(c))
	for _, n := range c {
		// skip nodes not specified in subset
		if !d.inSubset(n.addr) {
			continue
		}
		sentMsgs++
		msg := d.Message
		// do NOT enqueue in a goroutine. This inhibits ordering constraints.
		// the message will only be enqueued if the channel has enough capacity
		// or if the receiver is ready. This prevents a slow node from limiting the
		// enqueueing of messages to other nodes while still ensuring correct
		// ordering of messages.
		enqueued := n.channel.enqueueFast(request{ctx: ctx, msg: &Message{Metadata: md, Message: msg}, opts: o}, replyChan, false)
		if !enqueued {
			notEnqueued = append(notEnqueued, n)
		}
	}

	// it is important to retry the enqueueing for slow nodes. the method
	// will block until the message is enqueued.
	// NOTE: enqueueFast() creates a responseRouter and thus it is not
	// necessary to provide the replyChan to enqueueSlow().
	for _, n := range notEnqueued {
		msg := d.Message
		n.channel.enqueueSlow(request{ctx: ctx, msg: &Message{Metadata: md, Message: msg}, opts: o})
	}

	if o.noSendWaiting {
		return
	}

	// wait until all requests have been sent
	for ; sentMsgs > 0; sentMsgs-- {
		<-replyChan
	}
	close(replyChan)
}
