package gengorums

var orderedFutureVariables = `
{{$context := use "context.Context" .GenFile}}
{{$futureOut := outType .Method $customOut}}
`

var orderedFutureSignature = `func (c *Configuration) {{$method}}(` +
	`ctx {{$context}}, in *{{$in}}` +
	`{{perNodeFnType .GenFile .Method ", f"}}) ` +
	`*{{$futureOut}} {`

var orderedFutureBody = `
	{{/*template "trace"*/ -}}

	fut := &{{$futureOut}}{
		NodeIDs: make([]uint32, 0, c.n),
		c:       make(chan struct{}, 1),
	}
	// get the ID which will be used to return the correct responses for a request
	msgID := c.mgr.nextMsgID()
	
	// set up a channel to collect replies
	replyChan := make(chan *orderingResult, c.n)
	c.mgr.putChan(msgID, replyChan)

	expected := c.n
{{if not (hasPerNodeArg .Method)}}
	metadata := &{{$gorumsMD}}{
		MessageID: msgID,
		MethodID: {{$unexportMethod}}MethodID,
	}
	msg := &gorumsMessage{metadata: metadata, message: in}
{{- end}}

	// push the message to the nodes
	for _, n := range c.nodes {
{{- if hasPerNodeArg .Method}}
		nodeArg := f(in, n.ID())
		if nodeArg == nil {
			expected--
			continue
		}
		metadata := &{{$gorumsMD}}{
			MessageID: msgID,
			MethodID: {{$unexportMethod}}MethodID,
		}
		msg := &gorumsMessage{metadata: metadata, message: nodeArg}
{{- end}}
		n.sendQ <- msg
	}

	go c.{{$unexportMethod}}Recv(ctx, in, msgID, expected, replyChan, fut)

	return fut
}
`

var orderedFutureRecvSignature = `
func (c *Configuration) {{$unexportMethod}}Recv(ctx {{$context}}, ` +
	`in *{{$in}},` +
	`msgID uint64, expected int, ` +
	`replyChan chan *orderingResult, fut *{{$futureOut}}) {`

var orderedFutureRecvBody = `
	defer close(fut.c)

	if fut.err != nil {
		return
	}

	defer c.mgr.deleteChan(msgID)

	var (
		reply		*{{$customOut}}
		errs		[]GRPCError
		quorum		bool
		replies = make(map[uint32]*{{$out}}, 2*c.n)
	)

	for {
		select {
		case r := <-replyChan:
			fut.NodeIDs = append(fut.NodeIDs, r.nid)
			if r.err != nil {
				errs = append(errs, GRPCError{r.nid, r.err})
				break
			}
			{{- /*template "traceLazyLog"*/}}
			data := r.reply.(*{{$out}})
			replies[r.nid] = data
			if reply, quorum = c.qspec.{{$method}}QF(in, replies); quorum {
				fut.{{$customOutField}}, fut.err = reply, nil
				return
			}
		case <-ctx.Done():
			fut.{{$customOutField}}, fut.err = reply, QuorumCallError{ctx.Err().Error(), len(replies), errs}
			return
		}
		if len(errs)+len(replies) == expected {
			fut.{{$customOutField}}, fut.err = reply, QuorumCallError{"incomplete call", len(replies), errs}
			return
		}
	}
}
`

var orderedFutureCall = commonVariables +
	orderingVariables +
	orderedFutureVariables +
	futureCallComment +
	orderedFutureSignature +
	orderedFutureBody +
	orderedFutureRecvSignature +
	orderedFutureRecvBody
