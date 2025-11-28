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

var quorumCallComment = `
{{$comments := .Method.Comments.Leading}}
{{if ne $comments ""}}
{{$comments -}}
{{else}}
// {{$method}} is a quorum call invoked on all nodes in configuration c,
// with the same argument in, and returns a combined result.
{{end -}}
`

var qcVar = `
{{$genFile := .GenFile}}
{{$context := use "context.Context" .GenFile}}
{{$quorumCallWithInterceptor := use "gorums.QuorumCallWithInterceptor" .GenFile}}
{{$quorumSpecFunc := use "gorums.QuorumSpecFunc" .GenFile}}
{{$callOption := use "gorums.CallOption" .GenFile}}
`

var quorumCallSignature = `func (c *Configuration) {{$method}}(` +
	`ctx {{$context}}, in *{{$in}}, ` +
	`opts ...{{$callOption}})` +
	`(resp *{{$out}}, err error) {
`

var quorumCallBody = `	return {{$quorumCallWithInterceptor}}(
		ctx, c.RawConfiguration, in, "{{$fullName}}",
		{{$quorumSpecFunc}}(c.qspec.{{$method}}QF),
		opts...,
	)
}
`

var quorumCall = commonVariables +
	qcVar +
	quorumCallComment +
	quorumCallSignature +
	quorumCallBody
