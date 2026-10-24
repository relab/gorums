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
// {{$method}} starts the {{$service}}.{{$method}} one-way call
// on the client's node; no reply is returned to the client. The request is
// dispatched by Send, which blocks until the send completes and returns any
// send error, or by Async, which does not wait.
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
