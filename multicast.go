package gorums

import (
	"sync"

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
// Multicast supports request transformation interceptors via the gorums.Interceptors
// option. Use gorums.MapRequest to transform requests per-node.
//
// This method should be used by generated code only.
func Multicast[Req proto.Message](ctx *ConfigContext, msg Req, method string, opts ...CallOption) {
	c := ctx.cfg
	o := getCallOptions(E_Multicast, opts...)
	md := ordering.NewGorumsMetadata(ctx, c.getMsgID(), method)

	// Create reply channel to receive send confirmations
	replyChan := make(chan NodeResponse[proto.Message], len(c))

	waitSendDone := o.mustWaitSendDone()

	// Create ClientCtx for interceptor support
	clientCtx := newClientCtx[Req, *emptypb.Empty](ctx, c, msg, method, replyChan)

	// Apply interceptors to set up transformations
	for _, ic := range o.interceptors {
		interceptor := ic.(QuorumInterceptor[Req, *emptypb.Empty])
		interceptor(&Responses[Req, *emptypb.Empty]{ctx: clientCtx})
	}

	var expected int
	for _, n := range c {
		// Apply per-node request transformations (if any)
		transformedMsg := clientCtx.applyTransforms(msg, n)
		if transformedMsg == nil {
			continue // Skip this node if transform returns nil
		}
		expected++
		// Enqueue request
		n.channel.enqueue(request{
			ctx:          ctx,
			msg:          NewRequestMessage(md, transformedMsg),
			waitSendDone: waitSendDone,
			responseChan: replyChan,
		})
	}

	// Mark as sent (no-op since we don't have lazy sending)
	clientCtx.sendOnce = sync.OnceFunc(func() {})

	// If waiting for send completion, drain the reply channel
	if waitSendDone {
		for range expected {
			select {
			case <-replyChan:
			case <-ctx.Done():
				return
			}
		}
	}
}
