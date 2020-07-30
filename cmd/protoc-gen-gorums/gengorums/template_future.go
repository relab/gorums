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
		reply		*{{$customOut}}
		errs		[]GRPCError
		quorum		bool
		replies = make(map[uint32]*{{$out}}, 2*c.n)
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
			replies[r.nid] = r.reply
			if reply, quorum = c.qspec.{{$method}}QF(in, replies); quorum {
				resp.{{$customOutField}}, resp.err = reply, nil
				return
			}
		case <-ctx.Done():
			resp.{{$customOutField}}, resp.err = reply, QuorumCallError{ctx.Err().Error(), len(replies), errs}
			return
		}
		if len(errs)+len(replies) == expected {
			resp.{{$customOutField}}, resp.err = reply, QuorumCallError{"incomplete call", len(replies), errs}
			return
		}
	}
}
`

var futureVar = qcVar + `
{{$futureOut := outType .Method $customOut}}
`

var futureBody = `
	cd := {{$callData}}{
		Manager:  c.mgr.Manager,
		Nodes:    c.nodes,
		Message:  in,
		MethodID: {{$unexportMethod}}MethodID,
	}
	cd.QuorumFunction = func(req {{$protoMessage}}, replies map[uint32]{{$protoMessage}}) ({{$protoMessage}}, bool) {
		r := make(map[uint32]*{{$out}}, len(replies))
		for k, v := range replies {
			r[k] = v.(*{{$out}})
		}
		result, quorum := c.qspec.{{$method}}QF(req.(*{{$in}}), r)
		return result, quorum
	}
{{- if hasPerNodeArg .Method}}
	cd.PerNodeArgFn = func(req {{$protoMessage}}, nid uint32) {{$protoMessage}} {
		return f(req.(*{{$in}}), nid)
	}
{{- end}}

	fut := {{use "gorums.FutureCall" $genFile}}(ctx, cd)
	return &{{$futureOut}}{fut}
}
`

var futureCall = commonVariables +
	futureVar +
	futureCallComment +
	orderedFutureSignature +
	futureBody
