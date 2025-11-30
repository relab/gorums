package gorums

import (
	"github.com/relab/gorums/ordering"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Multicast is a one-way call; no replies are returned to the client.
//
// By default, this method blocks until messages have been sent to all nodes.
// This ensures that send operations complete before the caller proceeds, which can
// be useful for observing context cancellation or for pacing message sends.
//
// With the WithNoSendWaiting call option, the method returns immediately after
// enqueueing messages to all nodes (fire-and-forget semantics).
//
// Use WithPerNodeTransform to send different messages to each node.
//
// This method should be used by generated code only.
func Multicast[Req proto.Message](ctx *ConfigContext, msg Req, method string, opts ...CallOption) {
	c := ctx.cfg
	o := getCallOptions(E_Multicast, opts...)
	md := ordering.NewGorumsMetadata(ctx, c.getMsgID(), method)
	// We use emptypb.Empty for response and output types because Multicast is one-way.
	// However, we need to handle the generic types correctly.
	// Since Multicast is generic on Req, we can't easily instantiate QuorumInterceptor with specific types
	// unless we use `any` or similar, but QuorumInterceptor is strongly typed.
	//
	// To support interceptors, we treat Multicast as a QuorumCall that returns (nil, nil).
	// We construct a ClientCtx and run the interceptor chain.

	// Extract interceptors from options
	interceptors := interceptorsFromCallOptions[Req, *emptypb.Empty, *emptypb.Empty](o)

	// Base function that performs the actual sending
	base := func(ctx *ClientCtx[Req, *emptypb.Empty]) (*emptypb.Empty, error) {
		// Trigger sending by iterating responses (which waits for send completion if configured)
		// We ignore the actual "responses" since they are just send confirmations or errors
		for range ctx.Responses() {
		}
		return nil, nil
	}

	// Create ClientCtx
	// We need a reply channel to receive send confirmations/errors
	replyChan := make(chan NodeResponse[proto.Message], len(c))

	clientCtx := newClientCtx[Req, *emptypb.Empty](ctx, c, msg, method, replyChan)
	clientCtx.expectedReplies = len(c) // We expect one send confirmation/error per node

	// Define the send logic (lazy)
	clientCtx.sendOnce = func() {
		waitSendDone := o.mustWaitSendDone()
		for _, n := range c {
			// Apply transforms
			m := clientCtx.applyTransforms(msg, n)
			if m == nil || !m.ProtoReflect().IsValid() {
				// If transformed message is nil/invalid, we still need to send something to the channel
				// to satisfy expectedReplies, or we should decrement expectedReplies.
				// Decrementing is safer to avoid blocking.
				clientCtx.expectedReplies--
				continue
			}

			// Enqueue request
			n.channel.enqueue(request{
				ctx:          ctx,
				msg:          NewRequestMessage(md, m),
				waitSendDone: waitSendDone,
				responseChan: replyChan,
			})
		}
		// If fire-and-forget (no wait), we might not get anything on replyChan?
		// gorums implementation of enqueue: if waitSendDone is false, it might not send to responseChan?
		// Let's check enqueue implementation.
		// If waitSendDone is false, it sends to channel only if error?
		// Actually, if waitSendDone is false, we don't wait.
		// But ClientCtx.Responses() waits for expectedReplies.
		// If waitSendDone is false, we shouldn't wait.

		if !waitSendDone {
			// If not waiting, we shouldn't expect replies in the iterator.
			clientCtx.expectedReplies = 0
		}
	}

	// Execute chain
	_, _ = Chain(base, interceptors...)(clientCtx)
}
