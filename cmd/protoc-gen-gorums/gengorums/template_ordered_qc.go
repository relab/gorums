package gengorums

var orderingVariables = `
{{$errorf := use "fmt.Errorf" .GenFile}}
{{$gorumsMsg := use "ordering.Message" .GenFile}}
{{$unexportMethod := unexport .Method.GoName}}
`

var orderedQCSignature = `func (c *Configuration) {{$method}}(` +
	`ctx {{$context}}, in *{{$in}}` +
	`{{perNodeFnType .GenFile .Method ", f"}})` +
	`(resp *{{$customOut}}, err error) {
`

var orderingPreamble = `
	{{- template "trace" .}}

	// get the ID which will be used to return the correct responses for a request
	msgID := c.mgr.nextMsgID()
	
	// set up a channel to collect replies
	replies := make(chan *orderingResult, c.n)
	c.mgr.putChan(msgID, replies)
	
	// remove the replies channel when we are done
	defer c.mgr.deleteChan(msgID)
`

var orderingLoop = `
{{if not (hasPerNodeArg .Method) -}}
	data, err := marshaler.Marshal(in)
	if err != nil {
		return nil, {{$errorf}}("failed to marshal message: %w", err)
	}
	msg := &{{$gorumsMsg}}{
		ID: msgID,
		MethodID: {{$unexportMethod}}MethodID,
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
		data, err := marshaler.Marshal(nodeArg)
		if err != nil {
			return nil, {{$errorf}}("failed to marshal message: %w", err)
		}
		msg := &{{$gorumsMsg}}{
			ID: msgID,
			MethodID: {{$unexportMethod}}MethodID,
			Data: data,
		}
{{- end}}
		n.sendQ <- msg
	}
`

var orderingReply = `
	var (
		replyValues = make([]*{{$out}}, 0, expected)
		errs []GRPCError
		quorum      bool
	)

	for {
		select {
		case r := <-replies:
			if r.err != nil {
				errs = append(errs, GRPCError{r.nid, r.err})
				break
			}
			{{template "traceLazyLog"}}
			reply := new({{$out}})
			err := unmarshaler.Unmarshal(r.reply, reply)
			if err != nil {
				errs = append(errs, GRPCError{r.nid, {{$errorf}}("failed to unmarshal reply: %w", err)})
				break
			}
			replyValues = append(replyValues, reply)
			if resp, quorum = c.qspec.{{$method}}QF(in, replyValues); quorum {
				return resp, nil
			}
		case <-ctx.Done():
			return resp, QuorumCallError{ctx.Err().Error(), len(replyValues), errs}
		}

		if len(errs)+len(replyValues) == expected {
			return resp, QuorumCallError{"incomplete call", len(replyValues), errs}
		}
	}
}
`

var orderingQC = commonVariables +
	quorumCallVariables +
	orderingVariables +
	quorumCallComment +
	orderedQCSignature +
	orderingPreamble +
	orderingLoop +
	orderingReply
