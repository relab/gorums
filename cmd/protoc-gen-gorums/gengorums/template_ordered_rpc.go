package gengorums

var orderingRPCComment = `{{.Method.Comments.Leading}}`

var orderingRPCSignature = `func (n *Node) {{$method}}(` +
	`ctx {{$context}}, in *{{$in}}, ` +
	`opts ...{{$opts}})` +
	`(resp *{{$out}}, err error) {
`

var orderingRPCPreamble = `
	// get the ID which will be used to return the correct responses for a request
	msgID := n.nextMsgID()
	
	// set up a channel to collect replies
	replies := make(chan *orderingResult, 1)
	n.putChan(msgID, replies)
	
	// remove the replies channel when we are done
	defer n.deleteChan(msgID)
`

var orderingRPCBody = `
	data, err := marshaler.Marshal(in)
	if err != nil {
		return nil, {{$errorf}}("failed to marshal message: %w", err)
	}
	msg := &{{$gorumsMsg}}{
		ID: msgID,
		MethodID: {{$unexportMethod}}MethodID,
		Data: data,
	}
	n.sendQ <- msg

	select {
	case r := <-replies:
		if r.err != nil {
			return nil, r.err
		}
		reply := new({{$out}})
		err := unmarshaler.Unmarshal(r.reply, reply)
		if err != nil {
			return nil, {{$errorf}}("failed to unmarshal reply: %w", err)
		}
		return reply, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
`

var orderingRPC = commonVariables +
	quorumCallVariables +
	orderingVariables +
	orderingRPCComment +
	orderingRPCSignature +
	orderingRPCPreamble +
	orderingRPCBody
