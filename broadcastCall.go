package gorums

import (
	"context"
	"log/slog"

	"github.com/relab/gorums/ordering"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// broadcastCallData holds the message, destination nodes, method identifier,
// and other information necessary to perform the various quorum call types
// supported by Gorums.
type broadcastCallData struct {
	Message         protoreflect.ProtoMessage
	Method          string
	BroadcastID     string // a unique identifier for the current broadcast request
	Sender          string
	SenderID        string
	SenderAddr      string
	OriginID        string
	OriginAddr      string
	OriginMethod    string
	ServerAddresses []string
}

// checks whether the given address is contained in the given subset
// of server addresses. Will return true if a subset is not given.
func (bcd *broadcastCallData) inSubset(addr string) bool {
	if len(bcd.ServerAddresses) <= 0 {
		return true
	}
	for _, srvAddr := range bcd.ServerAddresses {
		if addr == srvAddr {
			return true
		}
	}
	return false
}

// broadcastCall performs a multicast call on the configuration.
func (c RawConfiguration) broadcastCall(ctx context.Context, d broadcastCallData) {
	md := &ordering.Metadata{MessageID: c.getMsgID(), Method: d.Method, BroadcastMsg: &ordering.BroadcastMsg{
		Sender:       d.Sender,
		BroadcastID:  d.BroadcastID,
		SenderID:     d.SenderID,
		SenderAddr:   d.SenderAddr,
		OriginID:     d.OriginID,
		OriginAddr:   d.OriginAddr,
		OriginMethod: d.OriginMethod,
	}}
	o := getCallOptions(E_Broadcast, nil)

	replyChan := make(chan response, len(c))
	sentMsgs := 0
	for _, n := range c {
		// skip nodes not specified in subset
		if !d.inSubset(n.addr) {
			continue
		}
		// try to reconnect to the node if prior connections have failed
		if !n.channel.isConnected() {
			n.mu.Lock()
			n.reconnect()
			n.mu.Unlock()
			if !n.channel.isConnected() {
				slog.Warn("could not connect to node", "nodeID", n.ID(), "nodeAddr", n.Address())
				continue
			} else {
				slog.Info("connected to node", "nodeID", n.ID(), "nodeAddr", n.Address())
			}
		}
		sentMsgs++
		msg := d.Message
		go n.channel.enqueue(request{ctx: ctx, msg: &Message{Metadata: md, Message: msg}, opts: o}, replyChan, false)
	}

	// wait until all requests have been sent
	for sentMsgs > 0 {
		<-replyChan
		sentMsgs--
	}
}
