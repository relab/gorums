package gengorums

var futureCallVariables = `
{{$context := use "context.Context" .GenFile}}
{{$opts := use "grpc.CallOption" .GenFile}}
{{$futureOut := outType .Method $customOut}}
`

var futureCallComment = `
{{$comments := .Method.Comments.Leading}}
{{if ne $comments ""}}
{{$comments -}}
{{else}}
{{if hasPerNodeArg .Method}}
// {{$method}} asynchronously invokes a quorum call on each node in
// configuration c, with the argument returned by the provided function f
// and returns the result as a {{$futureOut}}, which can be used to inspect
// the quorum call reply and error when available.
// The provide per node function f takes the provided {{$in}} argument
// and returns an {{$out}} object to be passed to the given nodeID.
// The per node function f should be thread-safe.
{{else}}
// {{$method}} asynchronously invokes a quorum call on configuration c
// and returns a {{$futureOut}}, which can be used to inspect the quorum call
// reply and error when available.
{{end -}}
{{end -}}
`

var futureCallSignature = `func (c *Configuration) {{$method}}(` +
	`ctx {{$context}}, in *{{$in}}` +
	`{{perNodeFnType .GenFile .Method ", f"}}` +
	`, opts ...{{$opts}}) ` +
	`*{{$futureOut}} {`

var futureCallBody = `
	fut := &{{$futureOut}}{
		NodeIDs: make([]uint32, 0, c.n),
		c:       make(chan struct{}, 1),
	}
	go func() {
		defer close(fut.c)
		c.{{unexport .Method.GoName}}(ctx, in{{perNodeArg .Method ", f"}}, fut, opts...)
	}()
	return fut
}
`

var futureCallUnexportedSignature = `
func (c *Configuration) {{unexport .Method.GoName}}(` +
	`ctx {{$context}}, in *{{$in}}` +
	`{{perNodeFnType .GenFile .Method ", f"}}` +
	`, resp *{{$futureOut}}, opts ...{{$opts}}) {
`

var futureCallReply = `
	var (
		//replyValues	= make([]*{{$out}}, 0, c.n)
		reply		*{{$customOut}}
		errs		[]GRPCError
		quorum		bool
		replys = make(map[uint32]*{{$out}})
	)

	for {
		select {
		case r := <-replyChan:
			resp.NodeIDs = append(resp.NodeIDs, r.nid)
			if r.err != nil {
				errs = append(errs, GRPCError{r.nid, r.err})
				break
			}
			{{template "traceLazyLog"}}
			replys[r.nid] = r.reply
			//replyValues = append(replyValues, r.reply)
			if reply, quorum = c.qspec.{{$method}}QF(in, replys); quorum {
				resp.{{$customOutField}}, resp.err = reply, nil
				return
			}
		case <-ctx.Done():
			resp.{{$customOutField}}, resp.err = reply, QuorumCallError{ctx.Err().Error(), len(replys), errs}
			return
		}
		if len(errs)+len(replys) == expected {
			resp.{{$customOutField}}, resp.err = reply, QuorumCallError{"incomplete call", len(replys), errs}
			return
		}
	}
}
`

var futureCall = commonVariables +
	futureCallVariables +
	futureCallComment +
	futureCallSignature +
	futureCallBody +
	futureCallUnexportedSignature +
	quorumCallLoop +
	futureCallReply +
	nodeCallGrpc
