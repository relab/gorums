package gengorums

// Common variables used in several template functions.
var commonVariables = `
{{$fullName := .Method.Desc.FullName}}
{{$method := .Method.GoName}}
{{$in := in .GenFile .Method}}
{{$out := out .GenFile .Method}}
{{$intOut := internalOut $out}}
{{$unexportOutput := unexport .Method.Output.GoIdent.GoName}}
`

// Common variables used in several template functions.
var quorumCallVariables = `
{{$iterator := use "gorums.Iterator" .GenFile}}
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
// with the same argument in, and returns a combined result. test
{{end -}}
{{end -}}
`

var quorumCallSignature = `func (c *Configuration) {{$method}}(` +
	`ctx {{$context}}, in *{{$in}}` +
	`{{perNodeFnType .GenFile .Method ", f"}})` +
	`{{$iterator}}[*{{$out}}] {
`

var qcVar = `
{{$callData := use "gorums.QuorumCallData" .GenFile}}
{{$genFile := .GenFile}}
{{$unexportMethod := unexport .Method.GoName}}
{{$context := use "context.Context" .GenFile}}
`

var quorumCallBody = `	cd := {{$callData}}{
		Message: in,
		Method:  "{{$fullName}}",
		ServerStream: {{isStream .Method}},
	}
{{- if hasPerNodeArg .Method}}
	{{- $protoMessage := use "proto.Message" .GenFile}}
	cd.PerNodeArgFn = func(req {{$protoMessage}}, nid uint32) {{$protoMessage}} {
		return f(req.(*{{$in}}), nid)
	}
{{- end}}

	return gorums.QuorumCall[*{{$out}}](ctx, c.RawConfiguration, cd)
}
`

var quorumCall = commonVariables +
	quorumCallVariables +
	qcVar +
	quorumCallComment +
	quorumCallSignature +
	quorumCallBody
