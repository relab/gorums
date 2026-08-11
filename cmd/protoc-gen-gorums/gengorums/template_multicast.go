package gengorums

var mcVar = `
{{$genFile := .GenFile}}
{{$configContext := "ConfigContext"}}
{{$multicast := use "gorums.Multicast" .GenFile}}
{{$onewayCall := use "gorums.OnewayCall" .GenFile}}
`

var multicastComment = `
{{$comments := .Method.Comments.Leading}}
{{if ne $comments ""}}
{{$comments -}}
{{else}}
// {{$method}} is a multicast call invoked on all nodes in the configuration in ctx.
// It returns a one-way call handle; call Send to block until every send
// completes and observe any send errors, or Async to dispatch without waiting.
// Use gorums.MapRequest to send different messages to each node.
{{end -}}
//
// Example:
//   err := {{$method}}(ctx, in).Send()
//   h := {{$method}}(ctx, in).Async(); err := h.Wait()
`

var multicastSignature = `func {{$method}}(` +
	`ctx *{{$configContext}}, in *{{$in}})` +
	` *{{$onewayCall}}[*{{$in}}] {
`

var multicastBody = `	return {{$multicast}}(ctx, in, "{{$fullName}}")
}
`

var multicastCall = commonVariables +
	mcVar +
	multicastComment +
	multicastSignature +
	multicastBody
