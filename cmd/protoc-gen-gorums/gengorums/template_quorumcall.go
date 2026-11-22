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
// {{$method}} returns a quorum-call handle for the client's configuration.
// A terminal method such as Majority(), First(), or Threshold(n) dispatches
// the call and retrieves the aggregated result.
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
{{$quorumCallScatter := use "gorumsimpl.QuorumCallScatter" .GenFile}}
{{$quorumCallStreamScatter := use "gorumsimpl.QuorumCallStreamScatter" .GenFile}}
{{$errScatterCount := use "gorums.ErrScatterCount" .GenFile}}
{{$call := use "gorums.Call" .GenFile}}
{{$impl := .Impl}}
{{$scatterName := scatterName .Method}}
{{$local := localNames "c ctx in" $in $out $context $quorumCall $quorumCallStream $quorumCallScatter $quorumCallStreamScatter}}
`

var quorumCallSignature = `{{if not .Decl}}func ({{$local.c}} {{$impl}}) {{end}}{{$method}}(` +
	`{{$local.ctx}} {{$context}}, {{$local.in}} *{{$in}})` +
	` *{{$call}}[*{{$in}}, *{{$out}}]`

var quorumCallBody = `{{if not .Decl}} {
	return {{$quorumCall}}[*{{$in}}, *{{$out}}]({{$local.ctx}}, {{$local.c}}.config, {{$local.in}}, "{{$fullName}}")
}
{{end}}`

// scatterComment returns the template of the doc comment of the method that
// the scatter option adds, with the given example call.
func scatterComment(example string) string {
	return `
{{- if .Decl}}

// {{$scatterName}} is the scatter variant of {{$method}}.
// It sends in[i] to the i-th node of the client's configuration, in one
// call. A nil in[i] skips that node. If the length of in differs from the
// configuration size, the call sends nothing and fails with
// [{{$errScatterCount}}].
//
// Example:
//
//	` + example + `
{{else}}
{{end -}}
`
}

var quorumCallScatterSignature = `{{if not .Decl}}func ({{$local.c}} {{$impl}}) {{end}}{{$scatterName}}(` +
	`{{$local.ctx}} {{$context}}, {{$local.in}} []*{{$in}})` +
	` *{{$call}}[*{{$in}}, *{{$out}}]`

var quorumCallScatterBody = `{{if not .Decl}} {
	return {{$quorumCallScatter}}[*{{$in}}, *{{$out}}]({{$local.ctx}}, {{$local.c}}.config, {{$local.in}}, "{{$fullName}}")
}
{{end}}`

// quorumCallScatter generates the scatter method of a quorum call with the
// scatter option.
var quorumCallScatter = `{{if isScatter .Method}}` +
	scatterComment(`resp, err := client.{{$scatterName}}(ctx, in).Majority()`) +
	quorumCallScatterSignature + quorumCallScatterBody + `{{end}}`

var quorumCall = commonVariables +
	quorumCallVariables +
	quorumCallComment +
	quorumCallSignature +
	quorumCallBody +
	quorumCallScatter

// Streaming quorum call template
var quorumCallStreamComment = `
{{- if .Decl}}
{{$comments := .Method.Comments.Leading}}
{{if ne $comments ""}}
{{$comments -}}
{{else}}
// {{$method}} returns a streaming quorum-call handle for the client's
// configuration. The server can send multiple responses, and the response
// iterator continues until the context is canceled.
//
// Example:
//
//	corr := client.{{$method}}(ctx, in).Correctable(2)
//	<-corr.Watch(2)
//	resp, level, err := corr.Get()
{{end -}}
{{end -}}
`

var quorumCallStreamBody = `{{if not .Decl}} {
	return {{$quorumCallStream}}[*{{$in}}, *{{$out}}]({{$local.ctx}}, {{$local.c}}.config, {{$local.in}}, "{{$fullName}}")
}
{{end}}`

var quorumCallStreamScatterBody = `{{if not .Decl}} {
	return {{$quorumCallStreamScatter}}[*{{$in}}, *{{$out}}]({{$local.ctx}}, {{$local.c}}.config, {{$local.in}}, "{{$fullName}}")
}
{{end}}`

// quorumCallStreamScatter generates the scatter method of a streaming quorum
// call with the scatter option.
var quorumCallStreamScatter = `{{if isScatter .Method}}` +
	scatterComment(`corr := client.{{$scatterName}}(ctx, in).Correctable(2)`) +
	quorumCallScatterSignature + quorumCallStreamScatterBody + `{{end}}`

var quorumCallStream = commonVariables +
	quorumCallVariables +
	quorumCallStreamComment +
	quorumCallSignature +
	quorumCallStreamBody +
	quorumCallStreamScatter
