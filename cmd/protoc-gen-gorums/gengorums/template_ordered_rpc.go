package gengorums

var orderingRPCComment = `{{.Method.Comments.Leading}}`

var orderingRPCSignature = `func (n *Node) {{$method}}(` +
	`ctx {{$context}}, in *{{$in}}) ` +
	`(resp *{{$out}}, err error) {
`

var orderingRPCPreamble = `
	// get the ID which will be used to return the correct responses for a request
	msgID := n.nextMsgID()
	
	// set up a channel to collect replies
	replyChan := make(chan *orderingResult, 1)
	n.putChan(msgID, replyChan)
	
	// remove the replies channel when we are done
	defer n.deleteChan(msgID)
`

var orderingRPCBody = `
	metadata := &{{$gorumsMD}}{
		MessageID: msgID,
		MethodID: {{$unexportMethod}}MethodID,
	}
	msg := &gorumsMessage{metadata: metadata, message: in}
	n.sendQ <- msg

	select {
	case r := <-replyChan:
		if r.err != nil {
			return nil, r.err
		}
		reply := r.reply.(*{{$out}})
		return reply, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
`

var orderingRPC = commonVariables +
	orderedRPCVariables +
	orderingRPCComment +
	orderingRPCSignature +
	orderingRPCPreamble +
	orderingRPCBody
