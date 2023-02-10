package gengorums

// Common variables used in several template functions.
var commonVariables = `
{{$fullName := .Method.Desc.FullName}}
{{$method := .Method.GoName}}
{{$in := in .GenFile .Method}}
{{$out := out .GenFile .Method}}
{{$intOut := internalOut $out}}
{{$customOut := customOut .GenFile .Method}}
{{$customOutField := field $customOut}}
{{$unexportOutput := unexport .Method.Output.GoIdent.GoName}}
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
	`{{perNodeFnType .GenFile .Method ", f"}})` +
	`(resp *{{$customOut}}, err error) {
`

var qcVar = `
{{$protoMessage := use "protoreflect.ProtoMessage" .GenFile}}
{{$callData := use "gorums.QuorumCallData" .GenFile}}
{{$genFile := .GenFile}}
{{$unexportMethod := unexport .Method.GoName}}
{{$context := use "context.Context" .GenFile}}
`

var quorumCallBody = `	cd := {{$callData}}{
		Message: in,
		Method:  "{{$fullName}}",
	}
	cd.QuorumFunction = func(req {{$protoMessage}}, replies map[uint32]{{$protoMessage}}) ({{$protoMessage}}, bool) {
		r := make(map[uint32]*{{$out}}, len(replies))
		for k, v := range replies {
			r[k] = v.(*{{$out}})
		}
		return c.AsRaw().QSpec().{{$method}}QF(req.(*{{$in}}), r)
	}
{{- if hasPerNodeArg .Method}}
	cd.PerNodeArgFn = func(req {{$protoMessage}}, nid uint32) {{$protoMessage}} {
		return f(req.(*{{$in}}), nid)
	}
{{- end}}

	res, err := c.AsRaw().QuorumCall(ctx, cd)
	if err != nil {
		return nil, err
	}
	return res.(*{{$customOut}}), err
}
`

var quorumCall = commonVariables +
	qcVar +
	quorumCallComment +
	quorumCallSignature +
	quorumCallBody
