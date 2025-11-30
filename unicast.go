package gorums

import (
	"github.com/relab/gorums/ordering"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Unicast is a one-way call; no replies are returned to the client.
//
// By default, this method blocks until the message has been sent to the node.
// This ensures that send operations complete before the caller proceeds, which can
// be useful for observing context cancellation or for pacing message sends.
//
// With the WithNoSendWaiting call option, the method returns immediately after
// enqueueing the message (fire-and-forget semantics).
//
// This method should be used by generated code only.
func Unicast[Req proto.Message](ctx *NodeContext, msg Req, method string, opts ...CallOption) {
	n := ctx.node
	o := getCallOptions(E_Unicast, opts...)

	md := ordering.NewGorumsMetadata(ctx, n.mgr.getMsgID(), method)

	// Extract interceptors from options
	interceptors := interceptorsFromCallOptions[Req, *emptypb.Empty, *emptypb.Empty](o)

	// Base function that performs the actual sending
	base := func(ctx *ClientCtx[Req, *emptypb.Empty]) (*emptypb.Empty, error) {
		// Trigger sending by iterating responses
		for range ctx.Responses() {
		}
		return nil, nil
	}

	// Create ClientCtx
	// We need a reply channel to receive send confirmations/errors
	replyChan := make(chan NodeResponse[proto.Message], 1)

	// Create a configuration with a single node
	// Unicast uses NodeContext which has a single node.
	// ClientCtx expects RawConfiguration.
	// We can create a temporary RawConfiguration with just this node.
	// But RawConfiguration is a type alias for []*RawNode.
	cfg := RawConfiguration{n}

	clientCtx := newClientCtx[Req, *emptypb.Empty](ctx, cfg, msg, method, replyChan)
	clientCtx.expectedReplies = 1

	// Define the send logic (lazy)
	clientCtx.sendOnce = func() {
		waitSendDone := o.mustWaitSendDone()

		// Apply transforms
		m := clientCtx.applyTransforms(msg, n)
		if m == nil || !m.ProtoReflect().IsValid() {
			clientCtx.expectedReplies = 0
			return
		}

		if !waitSendDone {
			n.channel.enqueue(request{ctx: ctx, msg: NewRequestMessage(md, m)})
			clientCtx.expectedReplies = 0
			return // fire-and-forget
		}

		// Default: block until send completes
		n.channel.enqueue(request{ctx: ctx, msg: NewRequestMessage(md, m), waitSendDone: true, responseChan: replyChan})
	}

	// Execute chain
	_, _ = Chain(base, interceptors...)(clientCtx)
}
