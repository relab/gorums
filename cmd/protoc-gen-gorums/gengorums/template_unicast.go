package gengorums

var unicastVar = `
{{$genFile := .GenFile}}
{{$unicast := use "gorumsimpl.Unicast" .GenFile}}
{{$onewayCall := use "gorums.OnewayCall" .GenFile}}
{{$impl := printf "%sNodeClient" (unexport $service)}}
{{$local := localNames "c ctx in" $in $context $unicast}}
`

var unicastComment = `
{{- if .Decl}}
{{$comments := .Method.Comments.Leading}}
{{if ne $comments ""}}
{{$comments -}}
{{else}}
// {{$method}} starts a one-way call on the client's node; no reply is
// returned.
//
// Send blocks until the send completes and returns any send error.
// Async returns a handle at once; Wait on the handle returns the send error.
{{end -}}
//
// Example:
//
//	err := nodeClient.{{$method}}(ctx, in).Send()
//	h := nodeClient.{{$method}}(ctx, in).Async()
//	err = h.Wait()
{{end -}}
`

var unicastSignature = `{{if not .Decl}}func ({{$local.c}} {{$impl}}) {{end}}{{$method}}(` +
	`{{$local.ctx}} {{$context}}, {{$local.in}} *{{$in}})` +
	` *{{$onewayCall}}[*{{$in}}]`

var unicastBody = `{{if .Decl}}
{{else}} {
	return {{$unicast}}({{$local.ctx}}, {{$local.c}}.node, {{$local.in}}, "{{$fullName}}")
}
{{end}}`

var unicastCall = commonVariables +
	unicastVar +
	unicastComment +
	unicastSignature +
	unicastBody
