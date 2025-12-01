package gorums

import (
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
	callOpts := getCallOptions(E_Multicast, opts...)
	waitSendDone := callOpts.mustWaitSendDone()

	clientCtx := newClientCtxBuilder[Req, *emptypb.Empty](ctx, msg, method).WithWaitSendDone(waitSendDone).Build()

	// Apply interceptors to set up transformations
	for _, ic := range callOpts.interceptors {
		interceptor := ic.(QuorumInterceptor[Req, *emptypb.Empty])
		interceptor(clientCtx)
	}

	// Send messages immediately (multicast doesn't use lazy sending)
	clientCtx.sendOnce.Do(clientCtx.send)

	// If waiting for send completion, drain the reply channel
	if waitSendDone {
		for range clientCtx.expectedReplies {
			select {
			case <-clientCtx.replyChan:
			case <-ctx.Done():
				return
			}
		}
	}
}
