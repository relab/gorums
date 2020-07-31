package gengorums

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

var futureSignature = `func (c *Configuration) {{$method}}(` +
	`ctx {{$context}}, in *{{$in}}` +
	`{{perNodeFnType .GenFile .Method ", f"}}) ` +
	`*{{$futureOut}} {`

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
		return c.qspec.{{$method}}QF(req.(*{{$in}}), r)
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
	futureSignature +
	futureBody
