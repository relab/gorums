package gengorums

var orderingVariables = `
{{$marshalOptions := use "proto.MarshalOptions" .GenFile}}
{{$unmarshalOptions := use "proto.UnmarshalOptions" .GenFile}}
{{$errorf := use "fmt.Errorf" .GenFile}}
{{$gorumsMsg := use "ordering.Message" .GenFile}}
{{$unexportMethod := unexport .Method.GoName}}
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
	data, err := {{$marshalOptions}}{AllowPartial: true, Deterministic: true}.Marshal(in)
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
		data, err := {{$marshalOptions}}{AllowPartial: true, Deterministic: true}.Marshal(nodeArg)
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
			err := {{$unmarshalOptions}}{AllowPartial: true, DiscardUnknown: true}.Unmarshal(r.reply, reply)
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

var orderingHandler = `
// {{$method}}Handler is the server API for the {{$method}} rpc.
type {{$method}}Handler interface {
	{{$method}}(*{{$in}}) (*{{$out}})
}

// Register{{$method}}Handler sets the handler for {{$method}}.
func (s *GorumsServer) Register{{$method}}Handler(handler {{$method}}Handler) {
	s.srv.registerHandler({{$unexportMethod}}MethodID, func(in *{{$gorumsMsg}}) *{{$gorumsMsg}} {
		req := new({{$in}})
		err := {{$unmarshalOptions}}{AllowPartial: true, DiscardUnknown: true}.Unmarshal(in.GetData(), req)
		// TODO: how to handle marshaling errors here
		if err != nil {
			return new({{$gorumsMsg}})
		}
		resp := handler.{{$method}}(req)
		data, err := {{$marshalOptions}}{AllowPartial: true, Deterministic: true}.Marshal(resp)
		if err != nil {
			return new({{$gorumsMsg}})
		}
		return &{{$gorumsMsg}}{Data: data, MethodID: {{$unexportMethod}}MethodID}
	})
}
`

var orderingQC = commonVariables +
	quorumCallVariables +
	orderingVariables +
	quorumCallComment +
	quorumCallSignature +
	orderingPreamble +
	orderingLoop +
	orderingReply +
	orderingHandler
