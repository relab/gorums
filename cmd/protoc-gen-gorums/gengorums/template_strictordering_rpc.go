package gengorums

var strictOrderingRPCComment = `{{.Method.Comments.Leading}}`

var strictOrderingRPCSignature = `func (n *Node) {{$method}}(` +
	`ctx {{$context}}, in *{{$in}}, ` +
	`opts ...{{$opts}})` +
	`(resp *{{$out}}, err error) {
`

var strictOrderingRPCPreamble = `
	// get the ID which will be used to return the correct responses for a request
	msgID := n.strictOrdering.nextMsgID()
	
	// set up a channel to collect replies
	replies := make(chan *strictOrderingResult, 1)
	n.strictOrdering.recvQMut.Lock()
	n.strictOrdering.recvQ[msgID] = replies
	n.strictOrdering.recvQMut.Unlock()
	
	defer func() {
		// remove the replies channel when we are done
		n.strictOrdering.recvQMut.Lock()
		delete(n.strictOrdering.recvQ, msgID)
		n.strictOrdering.recvQMut.Unlock()
	}()
`

var strictOrderingRPCBody = `
	data, err := {{$marshalAny}}(in)
	if err != nil {
		return nil, {{$errorf}}("failed to marshal message: %w", err)
	}
	msg := &{{$gorumsMsg}}{
		ID: msgID,
		URL: "{{fullName .Method}}",
		Data: data,
	}
	n.strictOrdering.sendQ <- msg

	select {
	case r := <-replies:
		if r.err != nil {
			return nil, r.err
		}
		reply := new({{$out}})
		err := {{$unmarshalAny}}(r.reply, reply)
		if err != nil {
			return nil, {{$errorf}}("failed to unmarshal reply: %w", err)
		}
		return reply, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
`

var strictOrderingRPC = commonVariables +
	quorumCallVariables +
	strictOrderingVariables +
	strictOrderingRPCComment +
	strictOrderingRPCSignature +
	strictOrderingRPCPreamble +
	strictOrderingRPCBody +
	strictOrderingHandler
