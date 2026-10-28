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
{{- if .Decl}}
{{$comments := .Method.Comments.Leading}}
{{if ne $comments ""}}
{{$comments -}}
{{else}}
// {{$method}} executes a quorum call on the client's configuration. Use
// terminal methods like Majority(), First(), or Threshold(n) to retrieve the
// aggregated result.
//
// Example:
//
//	resp, err := client.{{$method}}(ctx, in).Majority()
{{end -}}
{{end -}}
`

var quorumCallVariables = `
{{$genFile := .GenFile}}
{{$quorumCall := use "gorumsimpl.QuorumCall" .GenFile}}
{{$quorumCallStream := use "gorumsimpl.QuorumCallStream" .GenFile}}
{{$call := use "gorums.Call" .GenFile}}
{{$impl := printf "%sClient" (unexport $service)}}
{{$local := localNames "c ctx in" $in $out $context $quorumCall $quorumCallStream}}
`

var quorumCallSignature = `{{if not .Decl}}func ({{$local.c}} {{$impl}}) {{end}}{{$method}}(` +
	`{{$local.ctx}} {{$context}}, {{$local.in}} *{{$in}})` +
	` *{{$call}}[*{{$in}}, *{{$out}}]`

var quorumCallBody = `{{if .Decl}}
{{else}} {
	return {{$quorumCall}}[*{{$in}}, *{{$out}}]({{$local.ctx}}, {{$local.c}}.config, {{$local.in}}, "{{$fullName}}")
}
{{end}}`

var quorumCall = commonVariables +
	quorumCallVariables +
	quorumCallComment +
	quorumCallSignature +
	quorumCallBody

// Streaming quorum call template
var quorumCallStreamComment = `
{{- if .Decl}}
{{$comments := .Method.Comments.Leading}}
{{if ne $comments ""}}
{{$comments -}}
{{else}}
// {{$method}} executes a streaming quorum call on the client's configuration.
// The server can send multiple responses, and the response iterator continues
// until the context is canceled.
//
// Example:
//
//	corr := client.{{$method}}(ctx, in).Correctable(2)
//	<-corr.Watch(2)
//	resp, level, err := corr.Get()
{{end -}}
{{end -}}
`

var quorumCallStreamBody = `{{if .Decl}}
{{else}} {
	return {{$quorumCallStream}}[*{{$in}}, *{{$out}}]({{$local.ctx}}, {{$local.c}}.config, {{$local.in}}, "{{$fullName}}")
}
{{end}}`

var quorumCallStream = commonVariables +
	quorumCallVariables +
	quorumCallStreamComment +
	quorumCallSignature +
	quorumCallStreamBody
