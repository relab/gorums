package gengorums

var unicastVar = `
{{$genFile := .GenFile}}
{{$unicast := use "gorumsimpl.Unicast" .GenFile}}
{{$onewayCall := use "gorums.OnewayCall" .GenFile}}
`

var unicastComment = `
{{$comments := .Method.Comments.Leading}}
{{if ne $comments ""}}
{{$comments -}}
{{else}}
// {{$method}} starts a one-way call on the client's node; no reply is
// returned. The request is dispatched by Send or Async. Send blocks until the
// send completes and returns any send error. Async returns at once; call Wait
// on the returned handle to get the send error.
{{end -}}
//
// Example:
//   err := nodeClient.{{$method}}(ctx, in).Send()
//   h := nodeClient.{{$method}}(ctx, in).Async(); err := h.Wait()
`

var unicastSignature = `func (c {{$service}}ClientNode) {{$method}}(` +
	`ctx {{$context}}, in *{{$in}})` +
	` *{{$onewayCall}}[*{{$in}}] {
`

var unicastBody = `	return {{$unicast}}(ctx, c.node, in, "{{$fullName}}")
}
`

var unicastCall = commonVariables +
	unicastVar +
	unicastComment +
	unicastSignature +
	unicastBody
