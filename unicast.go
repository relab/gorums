package gorums

// Unicast is a one-way call; no replies are returned to the client.
//
// By default, this method blocks until the message has been sent to the node.
// This ensures that send operations complete before the caller proceeds, which can
// be useful for observing context cancellation or for pacing message sends.
// If the sending fails, the error is returned to the caller.
//
// With the IgnoreErrors call option, the method returns nil immediately after
// enqueueing the message (fire-and-forget semantics).
//
// This method should be used by generated code only.
func Unicast[Req msg](ctx *NodeContext, req Req, method string, opts ...CallOption) error {
	callOpts := getCallOptions(E_Unicast, opts...)
	md, err := marshalRequest(ctx, ctx.nextMsgID(), method, req)
	if err != nil {
		return err
	}

	waitSendDone := callOpts.mustWaitSendDone()
	if !waitSendDone {
		// Fire-and-forget: enqueue and return immediately
		ctx.enqueue(request{ctx: ctx, md: md})
		return nil
	}

	// Default: block until send completes
	replyChan := make(chan NodeResponse[msg], 1)
	ctx.enqueue(request{ctx: ctx, md: md, waitSendDone: true, responseChan: replyChan})

	// Wait for send confirmation
	select {
	case r := <-replyChan:
		// Unicast doesn't expect replies, but we still
		// want to report errors from the send attempt.
		return r.Err
	case <-ctx.Done():
		return ctx.Err()
	}
}
