package gengorums

var correctableCallComment = `
{{$comments := .Method.Comments.Leading}}
{{if ne $comments ""}}
{{$comments -}}
{{else}}
{{if hasPerNodeArg .Method}}
// {{$method}} asynchronously invokes a correctable quorum call on each node
// in configuration c, with the argument returned by the provided function f
// and returns a {{$correctableOut}}, which can be used to inspect
// the reply and error when available.
// The provide per node function f takes the provided {{$in}} argument
// and returns an {{$out}} object to be passed to the given nodeID.
// The per node function f should be thread-safe.
{{else}}
// {{$method}} asynchronously invokes a correctable quorum call on each node
// in configuration c and returns a {{$correctableOut}}, which can be used
// to inspect any replies or errors when available.
{{if correctableStream .Method -}}
// This method supports server-side preliminary replies (correctable stream).
{{end -}}
{{end -}}
{{end -}}
`

var correctableVar = `
{{$correctableOut := outType .Method $customOut}}
{{$protoMessage := use "protoreflect.ProtoMessage" .GenFile}}
{{$callData := use "gorums.CorrectableCallData" .GenFile}}
{{$genFile := .GenFile}}
{{$unexportMethod := unexport .Method.GoName}}
{{$context := use "context.Context" .GenFile}}
`

var correctableSignature = `func (c *{{$configurationName}}) {{$method}}(` +
	`ctx {{$context}}, in *{{$in}}` +
	`{{perNodeFnType .GenFile .Method ", f"}}) ` +
	`*{{$correctableOut}} {`

var correctableBody = `	cd := {{$callData}}{
		Message: in,
		Method:  "{{$fullName}}",
		ServerStream: {{correctableStream .Method}},
	}
	cd.QuorumFunction = func(req {{$protoMessage}}, replies map[uint32]{{$protoMessage}}) ({{$protoMessage}}, int, bool) {
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

	corr := c.RawConfiguration.CorrectableCall(ctx, cd)
	return &{{$correctableOut}}{corr}
}
`

var correctableCall = commonVariables +
	correctableVar +
	correctableCallComment +
	correctableSignature +
	correctableBody
