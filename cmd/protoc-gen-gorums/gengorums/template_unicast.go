package gengorums

var unicastVar = `
{{$genFile := .GenFile}}
{{$nodeContext := "NodeContext"}}
{{$unicast := use "gorumsimpl.Unicast" .GenFile}}
{{$onewayCall := use "gorums.OnewayCall" .GenFile}}
`

var unicastComment = `
{{$comments := .Method.Comments.Leading}}
{{if ne $comments ""}}
{{$comments -}}
{{else}}
// {{$method}} is a unicast call invoked on the node in ctx; no reply is
// returned to the client. It returns a one-way call handle; call Send to block
// until the send completes and observe any send error, or Async to dispatch
// without waiting.
{{end -}}
//
// Example:
//   err := {{$method}}(ctx, in).Send()
//   h := {{$method}}(ctx, in).Async(); err := h.Wait()
`

var unicastSignature = `func {{$method}}(` +
	`ctx *{{$nodeContext}}, in *{{$in}})` +
	` *{{$onewayCall}}[*{{$in}}] {
`

var unicastBody = `	return {{$unicast}}(ctx, in, "{{$fullName}}")
}
`

var unicastCall = commonVariables +
	unicastVar +
	unicastComment +
	unicastSignature +
	unicastBody
