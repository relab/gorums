package gengorums

var orderedFutureVariables = `
{{$context := use "context.Context" .GenFile}}
{{$futureOut := outType .Method $customOut}}
{{$unexportMethod := unexport .Method.GoName}}
`

var orderedFutureSignature = `func (c *Configuration) {{$method}}(` +
	`ctx {{$context}}, in *{{$in}}` +
	`{{perNodeFnType .GenFile .Method ", f"}}) ` +
	`(*{{$futureOut}}, error) {`

var orderedFutureBody = `
	fut := &{{$futureOut}}{
		NodeIDs: make([]uint32, 0, c.n),
		c:       make(chan struct{}, 1),
	}
	id, expected, replyChan, err := c.{{$unexportMethod}}Send(ctx, in{{perNodeArg .Method ", f"}})
	if err != nil {
		return nil, err
	}
	go func() {
		defer close(fut.c)
		c.{{$unexportMethod}}Recv(ctx, in, id, expected, replyChan, fut)
	}()
	return fut, nil
}
`

var orderedFutureSendSignature = `
func (c *Configuration) {{$unexportMethod}}Send(ctx {{$context}}, ` +
	`in *{{$in}}{{perNodeFnType .GenFile .Method ", f"}}) ` +
	`(uint64, int, chan *orderingResult, error) {
`

var orderedFutureSendPreamble = `
	{{- /*- template "trace" .*/ -}}

	// get the ID which will be used to return the correct responses for a request
	msgID := c.mgr.nextMsgID()
	
	// set up a channel to collect replies
	replyChan := make(chan *orderingResult, c.n)
	c.mgr.putChan(msgID, replyChan)
`

var orderedFutureSendLoop = `
{{if not (hasPerNodeArg .Method) -}}
	data, err := {{$marshalAny}}(in)
	if err != nil {
		return 0, 0, nil, {{$errorf}}("failed to marshal message: %w", err)
	}
	msg := &{{$gorumsMsg}}{
		ID: msgID,
		MethodID: {{$method}}MethodID,
		Data: data,
	}
{{end -}}

	// push the message to the nodes
	expected := c.n
	for _, n := range c.nodes {
{{- if hasPerNodeArg .Method}}
		nodeArg := f(in, n.ID())
		if nodeArg == nil {
			expected--
			continue
		}
		data, err := {{$marshalAny}}(nodeArg)
		if err != nil {
			return 0, 0, nil, {{$errorf}}("failed to marshal message: %w", err)
		}
		msg := &{{$gorumsMsg}}{
			ID: msgID,
			MethodID: {{$method}}MethodID,
			Data: data,
		}
		{{- end}}
		n.sendQ <- msg
	}

	return msgID, expected, replyChan, nil
}
`

var orderedFutureRecvSignature = `
func (c *Configuration) {{$unexportMethod}}Recv(ctx {{$context}}, ` +
	`in *{{$in}},` +
	`msgID uint64, expected int, ` +
	`replyChan chan *orderingResult, resp *{{$futureOut}}) {`
var orderedFutureRecvBody = `
	defer c.mgr.deleteChan(msgID)

	var (
		replyValues	= make([]*{{$out}}, 0, c.n)
		reply		*{{$customOut}}
		errs		[]GRPCError
		quorum		bool
	)

	for {
		select {
		case r := <-replyChan:
			resp.NodeIDs = append(resp.NodeIDs, r.nid)
			if r.err != nil {
				errs = append(errs, GRPCError{r.nid, r.err})
				break
			}
			{{/*template "traceLazyLog"*/}}
			data := new({{$out}})
			err := {{$unmarshalAny}}(r.reply, data)
			if err != nil {
				errs = append(errs, GRPCError{r.nid, {{$errorf}}("failed to unmarshal reply: %w", err)})
				break
			}
			replyValues = append(replyValues, data)
			if reply, quorum = c.qspec.{{$method}}QF(in, replyValues); quorum {
				resp.{{$customOutField}}, resp.err = reply, nil
				return
			}
		case <-ctx.Done():
			resp.{{$customOutField}}, resp.err = reply, QuorumCallError{ctx.Err().Error(), len(replyValues), errs}
			return
		}
		if len(errs)+len(replyValues) == expected {
			resp.{{$customOutField}}, resp.err = reply, QuorumCallError{"incomplete call", len(replyValues), errs}
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
	orderedFutureSendSignature +
	orderedFutureSendPreamble +
	orderedFutureSendLoop +
	orderedFutureRecvSignature +
	orderedFutureRecvBody +
	orderingHandler
