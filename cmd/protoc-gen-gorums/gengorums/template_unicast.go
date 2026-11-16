package gengorums

var unicastVar = `
{{$genFile := .GenFile}}
{{$unicast := use "gorumsimpl.Unicast" .GenFile}}
{{$onewayCall := use "gorums.OnewayCall" .GenFile}}
{{$impl := .Impl}}
{{$local := localNames "c ctx in" $in $context $unicast}}
`

var unicastComment = `
{{- if .Decl}}
{{$comments := .Method.Comments.Leading}}
{{if ne $comments ""}}
{{$comments -}}
{{else}}
// {{$method}} returns a one-way call handle for the client's node.
// No reply is returned.
//
// Send queues the request and returns the admission error; it does not wait
// for queue space or for the stream write. Async queues the request the same
// way and returns a handle; Wait on the handle returns the admission error or
// the stream-write error.
{{end -}}
//
// Example:
//
//	err := nodeClient.{{$method}}(ctx, in).Send()
//
// To wait for the stream write:
//
//	h := nodeClient.{{$method}}(ctx, in).Async()
//	err = h.Wait()
{{end -}}
`

var unicastSignature = `{{if not .Decl}}func ({{$local.c}} {{$impl}}) {{end}}{{$method}}(` +
	`{{$local.ctx}} {{$context}}, {{$local.in}} *{{$in}})` +
	` *{{$onewayCall}}[*{{$in}}]`

var unicastBody = `{{if not .Decl}} {
	return {{$unicast}}({{$local.ctx}}, {{$local.c}}.node, {{$local.in}}, "{{$fullName}}")
}
{{end}}`

var unicastCall = commonVariables +
	unicastVar +
	unicastComment +
	unicastSignature +
	unicastBody
