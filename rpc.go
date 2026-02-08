package gorums

// RPCCall executes a remote procedure call on the node.
//
// This method should be used by generated code only.
func RPCCall[Req, Resp msg](ctx *NodeContext, req Req, method string) (Resp, error) {
	replyChan := make(chan NodeResponse[msg], 1)
	md, err := MarshalMetadata(ctx, ctx.nextMsgID(), method, req)
	if err != nil {
		var zero Resp
		return zero, err
	}
	ctx.enqueue(request{ctx: ctx, md: md, responseChan: replyChan})

	select {
	case r := <-replyChan:
		var zero Resp
		if r.Err != nil {
			return zero, r.Err
		}
		resp, ok := r.Value.(Resp)
		if !ok {
			return zero, ErrTypeMismatch
		}
		return resp, r.Err
	case <-ctx.Done():
		var zero Resp
		return zero, ctx.Err()
	}
}
