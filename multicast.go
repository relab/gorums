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
// If the sending fails, the error is returned to the caller.
//
// With the IgnoreErrors call option, the method returns nil immediately after
// enqueueing messages to all nodes (fire-and-forget semantics).
//
// Multicast supports request transformation interceptors via the gorums.Interceptors
// option. Use gorums.MapRequest to transform requests per-node.
//
// This method should be used by generated code only.
func Multicast[T NodeID, Req proto.Message](ctx *ConfigContext[T], msg Req, method string, opts ...CallOption) error {
	callOpts := getCallOptions(E_Multicast, opts...)
	waitSendDone := callOpts.mustWaitSendDone()

	clientCtx := newClientCtxBuilder[T, Req, *emptypb.Empty](ctx, msg, method).WithWaitSendDone(waitSendDone).Build()
	clientCtx.applyInterceptors(callOpts.interceptors)

	// Send messages immediately (multicast doesn't use lazy sending)
	clientCtx.sendOnce.Do(clientCtx.send)

	// If waiting for send completion, drain the reply channel and return the first error.
	if waitSendDone {
		var errs []nodeError[T]
		for range clientCtx.expectedReplies {
			select {
			case r := <-clientCtx.replyChan:
				if r.Err != nil {
					errs = append(errs, nodeError[T]{cause: r.Err, nodeID: r.NodeID})
				}
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if len(errs) > 0 {
			return QuorumCallError[T]{cause: ErrSendFailure, errors: errs}
		}
	}
	return nil
}
