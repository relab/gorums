package gengorums

var strictOrderingRPCComment = `{{.Method.Comments.Leading}}`

var strictOrderingRPCSignature = `func (n *Node) {{$method}}(` +
	`ctx {{$context}}, in *{{$in}}, ` +
	`opts ...{{$opts}})` +
	`(resp *{{$out}}, err error) {
`

var strictOrderingRPCPreamble = `
	// get the ID which will be used to return the correct responses for a request
	msgID := n.nextMsgID()
	
	// set up a channel to collect replies
	replies := make(chan *strictOrderingResult, 1)
	n.putChan(msgID, replies)
	
	// remove the replies channel when we are done
	defer n.deleteChan(msgID)
`

var strictOrderingRPCBody = `
	data, err := {{$marshalAny}}(in)
	if err != nil {
		return nil, {{$errorf}}("failed to marshal message: %w", err)
	}
	msg := &GorumsMessage{
		ID: msgID,
		Method: "{{fullName .Method}}",
		Data: data,
	}
	n.sendQ <- msg

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
