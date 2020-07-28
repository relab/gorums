package gengorums

// Common variables used in several template functions.
var commonVariables = `
{{$method := .Method.GoName}}
{{$in := in .GenFile .Method}}
{{$out := out .GenFile .Method}}
{{$intOut := internalOut $out}}
{{$customOut := customOut .GenFile .Method}}
{{$customOutField := field $customOut}}
{{$unexportOutput := unexport .Method.Output.GoIdent.GoName}}
`

var quorumCallVariables = `
{{$context := use "context.Context" .GenFile}}
{{$opts := use "grpc.CallOption" .GenFile}}
`

var quorumCallComment = `
{{$comments := .Method.Comments.Leading}}
{{if ne $comments ""}}
{{$comments -}}
{{else}}
{{if hasPerNodeArg .Method}}
// {{$method}} is a quorum call invoked on each node in configuration c,
// with the argument returned by the provided function f, and returns the combined result.
// The per node function f receives a copy of the {{$in}} request argument and
// returns a {{$in}} manipulated to be passed to the given nodeID.
// The function f must be thread-safe.
{{else}}
// {{$method}} is a quorum call invoked on all nodes in configuration c,
// with the same argument in, and returns a combined result.
{{end -}}
{{end -}}
`

var quorumCallSignature = `func (c *Configuration) {{$method}}(` +
	`ctx {{$context}}, in *{{$in}}` +
	`{{perNodeFnType .GenFile .Method ", f"}}` +
	`, opts ...{{$opts}})` +
	`(resp *{{$customOut}}, err error) {
`

var quorumCallLoop = `
	{{- template "trace" .}}
	expected := c.n
	replyChan := make(chan {{$intOut}}, expected)
	for _, n := range c.nodes {
		{{- if hasPerNodeArg .Method}}
		nodeArg := f(in, n.id)
		if nodeArg == nil {
			expected--
			continue
		}
		go n.{{$method}}(ctx, nodeArg, replyChan)
		{{else}}
		go n.{{$method}}(ctx, in, replyChan)
		{{end -}}
	}
`

var quorumCallReply = `
	var (
		errs        []GRPCError
		quorum      bool
		replies = make(map[uint32]*{{$out}})
	)

	for {
		select {
		case r := <-replyChan:
			if r.err != nil {
				errs = append(errs, GRPCError{r.nid, r.err})
				break
			}
			{{template "traceLazyLog"}}
			replies[r.nid] = r.reply
			if resp, quorum = c.qspec.{{$method}}QF(in, replies); quorum {
				return resp, nil
			}
		case <-ctx.Done():
			return resp, QuorumCallError{ctx.Err().Error(), len(replies), errs}
		}
		if len(errs)+len(replies) == expected {
			return resp, QuorumCallError{"incomplete call", len(replies), errs}
		}
	}
}
`

var nodeCallGrpc = `
func (n *Node) {{$method}}(ctx {{$context}}, in *{{$in}}, replyChan chan<- {{$intOut}}) {
	reply := new({{$out}})
	start := {{use "time.Now" .GenFile}}()
	err := n.conn.Invoke(ctx, "{{fullName .Method}}", in, reply)
	s, ok := {{use "status.FromError" .GenFile}}(err)
	if ok && (s.Code() == {{use "codes.OK" .GenFile}} || s.Code() == codes.Canceled) {
		n.setLatency(time.Since(start))
	} else {
		n.setLastErr(err)
	}
	replyChan <- {{$intOut}}{n.id, reply, err}
}
`

var qcVar = `
{{$protoMessage := use "protoreflect.ProtoMessage" .GenFile}}
{{$callData := use "gorums.CallData" .GenFile}}
{{$quorumCall := use "gorums.QuorumCall" .GenFile}}
{{$unexportMethod := unexport .Method.GoName}}
{{$context := use "context.Context" .GenFile}}
`

var quorumCallBody = `
	cd := {{$callData}}{
		Manager:  c.mgr,
		Nodes:    c.Nodes(),
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

	res, err := {{$quorumCall}}(ctx, cd)
	return res.(*{{$customOut}}), err
}
`

var quorumCall = commonVariables +
	qcVar +
	quorumCallComment +
	orderedQCSignature +
	quorumCallBody
