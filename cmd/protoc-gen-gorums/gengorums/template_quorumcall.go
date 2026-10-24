package gengorums

// Common variables used in several template functions.
var commonVariables = `
{{$fullName := .Method.Desc.FullName}}
{{$service := .Method.Parent.GoName}}
{{$method := .Method.GoName}}
{{$in := in .GenFile .Method}}
{{$out := out .GenFile .Method}}
{{$context := use "context.Context" .GenFile}}
{{$unexportOutput := unexport .Method.Output.GoIdent.GoName}}
`

var quorumCallComment = `
{{$comments := .Method.Comments.Leading}}
{{if ne $comments ""}}
{{$comments -}}
{{else}}
// {{$method}} executes the {{$service}}.{{$method}} quorum call on the client's
// configuration. Use terminal methods like Majority(), First(), or
// Threshold(n) to retrieve the aggregated result.
//
// Example:
//   resp, err := client.{{$method}}(ctx, in).Majority()
{{end -}}
`

var quorumCallVariables = `
{{$genFile := .GenFile}}
{{$quorumCall := use "gorumsimpl.QuorumCall" .GenFile}}
{{$quorumCallStream := use "gorumsimpl.QuorumCallStream" .GenFile}}
{{$call := use "gorums.Call" .GenFile}}
`

var quorumCallSignature = `func (c {{$service}}Client) {{$method}}(` +
	`ctx {{$context}}, in *{{$in}})` +
	` *{{$call}}[*{{$in}}, *{{$out}}] {
`

var quorumCallBody = `	return {{$quorumCall}}[*{{$in}}, *{{$out}}](ctx, c.config, in, "{{$fullName}}")
}
`

var quorumCall = commonVariables +
	quorumCallVariables +
	quorumCallComment +
	quorumCallSignature +
	quorumCallBody

// Streaming quorum call template
var quorumCallStreamComment = `
{{$comments := .Method.Comments.Leading}}
{{if ne $comments ""}}
{{$comments -}}
{{else}}
// {{$method}} executes the {{$service}}.{{$method}} streaming quorum call on the
// client's configuration. The server can send multiple responses, and the
// response iterator continues until the context is canceled.
//
// Example:
//   corr := client.{{$method}}(ctx, in).Correctable(2)
//   <-corr.Watch(2)
//   resp, level, err := corr.Get()
{{end -}}
`

var quorumCallStreamBody = `	return {{$quorumCallStream}}[*{{$in}}, *{{$out}}](ctx, c.config, in, "{{$fullName}}")
}
`

var quorumCallStream = commonVariables +
	quorumCallVariables +
	quorumCallStreamComment +
	quorumCallSignature +
	quorumCallStreamBody
