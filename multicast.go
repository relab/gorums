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

	// Create clientCtx for interceptor support
	clientCtx := newMulticastClientCtx(ctx, c, msg, method, md, replyChan, waitSendDone)

	// Apply interceptors to set up transformations
	for _, ic := range o.interceptors {
		interceptor := ic.(QuorumInterceptor[Req, *emptypb.Empty])
		interceptor(clientCtx)
	}

	// Send messages immediately (multicast doesn't use lazy sending)
	clientCtx.sendOnce.Do(clientCtx.send)

	// If waiting for send completion, drain the reply channel
	if waitSendDone {
		for range clientCtx.expectedReplies {
			select {
			case <-replyChan:
			case <-ctx.Done():
				return
			}
		}
	}
}

// newMulticastClientCtx creates a clientCtx configured for multicast calls.
func newMulticastClientCtx[Req msg](
	ctx *ConfigContext,
	config RawConfiguration,
	req Req,
	method string,
	md *ordering.Metadata,
	replyChan chan NodeResponse[msg],
	waitSendDone bool,
) *clientCtx[Req, *emptypb.Empty] {
	return &clientCtx[Req, *emptypb.Empty]{
		Context:         ctx,
		config:          config,
		request:         req,
		method:          method,
		md:              md,
		replyChan:       replyChan,
		expectedReplies: config.Size(),
		waitSendDone:    waitSendDone,
	}
}
