package gengorums

var asyncCallComment = `
{{$comments := .Method.Comments.Leading}}
{{if ne $comments ""}}
{{$comments -}}
{{else}}
// {{$method}} asynchronously invokes a quorum call on configuration c
// and returns a {{$asyncOut}}, which can be used to inspect the quorum call
// reply and error when available.
{{end -}}
`

var asyncSignature = `func (c *Configuration) {{$method}}(` +
	`ctx {{$context}}, in *{{$in}}) ` +
	`*{{$asyncOut}} {`

var asyncVar = `
{{$protoMessage := use "proto.Message" .GenFile}}
{{$callData := use "gorums.QuorumCallData" .GenFile}}
{{$genFile := .GenFile}}
{{$context := use "context.Context" .GenFile}}
{{$asyncOut := outType .Method $out}}
`

var asyncBody = `	cd := {{$callData}}{
		Message: in,
		Method:  "{{$fullName}}",
	}
	cd.QuorumFunction = func(req {{$protoMessage}}, replies map[uint32]{{$protoMessage}}) ({{$protoMessage}}, bool) {
		r := make(map[uint32]*{{$out}}, len(replies))
		for k, v := range replies {
			r[k] = v.(*{{$out}})
		}
		return c.qspec.{{$method}}QF(req.(*{{$in}}), r)
	}

	fut := c.RawConfiguration.AsyncCall(ctx, cd)
	return &{{$asyncOut}}{fut}
}
`

var asyncCall = commonVariables +
	asyncVar +
	asyncCallComment +
	asyncSignature +
	asyncBody
