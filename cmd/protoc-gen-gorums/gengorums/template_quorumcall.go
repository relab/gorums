package gengorums

// Common variables used in several template functions.
var commonVariables = `
{{- $fullName := .Method.Desc.FullName}}
{{- $method := .Method.GoName}}
{{- $in := in .GenFile .Method}}
{{- $out := out .GenFile .Method}}
{{- $intOut := internalOut $out}}
{{- $unexportOutput := unexport .Method.Output.GoIdent.GoName}}
{{- $service := .Method.Parent.GoName}}
{{- $nodeName := printf "%sNode" $service}}
{{- $configurationName := printf "%sConfiguration" $service}}
`

// Common variables used in several template functions.
var quorumCallVariables = `
{{- $iterator := use "gorums.Responses" .GenFile}}
`

var quorumCallComment = `
{{$comments := .Method.Comments.Leading}}
{{if ne $comments ""}}
{{$comments -}}
{{else}}
// {{$method}} is a quorum call invoked on each node in configuration c,
{{if hasPerNodeArg .Method}}
// with the argument returned by the provided function f,
// it returns the responses as an iterator.
// The per node function f receives a copy of the {{$in}} request argument and
// returns a {{$in}} manipulated to be passed to the given nodeID.
// The function f must be thread-safe.
{{else}}
// with the same argument in, and returns the responses as an iterator.
{{end -}}
{{if isStream .Method}}
// This is a streaming quorum call, so each can respond with any amount of responses.
{{end -}}
{{end -}}
`

var quorumCallReserve = `{{reserveMethod $configurationName $method}}`

var quorumCallSignature = `func (c *{{$configurationName}}) {{$method}}(` +
	`ctx {{$context}}, in *{{$in}}` +
	`{{perNodeFnType .GenFile .Method ", f"}})` +
	`{{$iterator}}[*{{$out}}] {
`

var qcVar = `
{{- $callData := use "gorums.QuorumCallData" .GenFile}}
{{- $genFile := .GenFile}}
{{- $unexportMethod := unexport .Method.GoName}}
{{- $context := use "context.Context" .GenFile}}
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

	return gorums.QuorumCall[*{{$out}}](ctx, c.Configuration, cd)
}
`

var quorumCall = commonVariables +
	quorumCallVariables +
	qcVar +
	quorumCallComment +
	quorumCallReserve +
	quorumCallSignature +
	quorumCallBody
