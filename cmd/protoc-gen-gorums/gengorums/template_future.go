package gengorums

var futureCallVariables = `
{{$context := use "context.Context" .GenFile}}
{{$opts := use "grpc.CallOption" .GenFile}}
{{$futureOut := printf "Future%s" $out}}
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

var futureCallInterface = `
// Get returns the reply and any error associated with the {{$method}}.
// The method blocks until a reply or error is available.
func (f *{{$futureOut}}) Get() (*{{$customOut}}, error) {
	<-f.c
	return f.{{$customOut}}, f.err
}

// Done reports if a reply and/or error is available for the {{$method}}.
func (f *{{$futureOut}}) Done() bool {
	select {
	case <-f.c:
		return true
	default:
		return false
	}
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
			{{template "traceLazyLog"}}
			replyValues = append(replyValues, r.reply)
			if reply, quorum = c.qspec.{{$method}}QF({{withQFArg .Method "in, "}}replyValues); quorum {
				resp.{{$customOut}}, resp.err = reply, nil
				return
			}
		case <-ctx.Done():
			resp.{{$customOut}}, resp.err = reply, QuorumCallError{ctx.Err().Error(), len(replyValues), errs}
			return
		}
		if len(errs)+len(replyValues) == expected {
			resp.{{$customOut}}, resp.err = reply, QuorumCallError{"incomplete call", len(replyValues), errs}
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
	futureCallInterface +
	futureCallUnexportedSignature +
	quorumCallLoop +
	futureCallReply +
	nodeCallGrpc
