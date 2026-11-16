package gengorums

var mcVar = `
{{$genFile := .GenFile}}
{{$multicast := use "gorumsimpl.Multicast" .GenFile}}
{{$onewayCall := use "gorums.OnewayCall" .GenFile}}
{{$impl := .Impl}}
{{$local := localNames "c ctx in" $in $context $multicast}}
`

var multicastComment = `
{{- if .Decl}}
{{$comments := .Method.Comments.Leading}}
{{if ne $comments ""}}
{{$comments -}}
{{else}}
// {{$method}} returns a one-way call handle for the client's configuration.
// No reply is returned. Use gorums.MapRequest to send a different message to
// each node.
//
// Send queues the request for every node and returns the admission errors;
// it does not wait for queue space or for the stream writes. Async queues the
// request the same way and returns a handle; Wait on the handle returns the
// admission errors and the stream-write errors.
{{end -}}
//
// Example:
//
//	err := client.{{$method}}(ctx, in).Send()
//
// To wait for the stream writes:
//
//	h := client.{{$method}}(ctx, in).Async()
//	err = h.Wait()
{{end -}}
`

var multicastSignature = `{{if not .Decl}}func ({{$local.c}} {{$impl}}) {{end}}{{$method}}(` +
	`{{$local.ctx}} {{$context}}, {{$local.in}} *{{$in}})` +
	` *{{$onewayCall}}[*{{$in}}]`

var multicastBody = `{{if not .Decl}} {
	return {{$multicast}}({{$local.ctx}}, {{$local.c}}.config, {{$local.in}}, "{{$fullName}}")
}
{{end}}`

var multicastCall = commonVariables +
	mcVar +
	multicastComment +
	multicastSignature +
	multicastBody
