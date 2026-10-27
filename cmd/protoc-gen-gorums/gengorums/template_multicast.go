package gengorums

var mcVar = `
{{$genFile := .GenFile}}
{{$multicast := use "gorumsimpl.Multicast" .GenFile}}
{{$onewayCall := use "gorums.OnewayCall" .GenFile}}
{{$local := localNames "c ctx in" $in $context $multicast}}
`

var multicastComment = `
{{$comments := .Method.Comments.Leading}}
{{if ne $comments ""}}
{{$comments -}}
{{else}}
// {{$method}} starts a one-way call on the client's configuration; no reply
// is returned. The request is dispatched by Send or Async. Send blocks until
// every send completes and returns any send errors. Async returns at once;
// call Wait on the returned handle to get the send errors. Use
// gorums.MapRequest to send a different message to each node.
{{end -}}
//
// Example:
//   err := client.{{$method}}(ctx, in).Send()
//   h := client.{{$method}}(ctx, in).Async(); err := h.Wait()
`

var multicastSignature = `func ({{$local.c}} {{$service}}Client) {{$method}}(` +
	`{{$local.ctx}} {{$context}}, {{$local.in}} *{{$in}})` +
	` *{{$onewayCall}}[*{{$in}}] {
`

var multicastBody = `	return {{$multicast}}({{$local.ctx}}, {{$local.c}}.config, {{$local.in}}, "{{$fullName}}")
}
`

var multicastCall = commonVariables +
	mcVar +
	multicastComment +
	multicastSignature +
	multicastBody
