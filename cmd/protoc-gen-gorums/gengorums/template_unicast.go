package gengorums

var unicastVar = `
{{$genFile := .GenFile}}
{{$unicast := use "gorumsimpl.Unicast" .GenFile}}
{{$onewayCall := use "gorums.OnewayCall" .GenFile}}
{{$node := use "gorums.Node" .GenFile}}
{{$local := localNames "c ctx node in" $in $context $unicast}}
`

var unicastComment = `
{{$comments := .Method.Comments.Leading}}
{{if ne $comments ""}}
{{$comments -}}
{{else}}
// {{$method}} starts a one-way call on the given node; no reply is returned.
// It does not use the client's configuration. The request is dispatched by
// Send or Async. Send blocks until the send completes and returns any send
// error. Async returns at once; call Wait on the returned handle to get the
// send error.
{{end -}}
//
// Example:
//   err := client.{{$method}}(ctx, node, in).Send()
//   h := client.{{$method}}(ctx, node, in).Async(); err := h.Wait()
`

var unicastSignature = `func ({{$local.c}} {{$service}}Client) {{$method}}(` +
	`{{$local.ctx}} {{$context}}, {{$local.node}} *{{$node}}, {{$local.in}} *{{$in}})` +
	` *{{$onewayCall}}[*{{$in}}] {
`

var unicastBody = `	return {{$unicast}}({{$local.ctx}}, {{$local.node}}, {{$local.in}}, "{{$fullName}}")
}
`

var unicastCall = commonVariables +
	unicastVar +
	unicastComment +
	unicastSignature +
	unicastBody
