package gengorums

var mcVar = `
{{$genFile := .GenFile}}
{{$multicast := use "gorumsimpl.Multicast" .GenFile}}
{{$onewayCall := use "gorums.OnewayCall" .GenFile}}
`

var multicastComment = `
{{$comments := .Method.Comments.Leading}}
{{if ne $comments ""}}
{{$comments -}}
{{else}}
// {{$method}} starts the {{$service}}.{{$method}} one-way call
// on the client's configuration. The request is dispatched by Send, which
// blocks until every send completes and returns any send errors, or by Async,
// which does not wait. Use gorums.MapRequest to send different messages to
// each node.
{{end -}}
//
// Example:
//   err := client.{{$method}}(ctx, in).Send()
//   h := client.{{$method}}(ctx, in).Async(); err := h.Wait()
`

var multicastSignature = `func (c {{$service}}Client) {{$method}}(` +
	`ctx {{$context}}, in *{{$in}})` +
	` *{{$onewayCall}}[*{{$in}}] {
`

var multicastBody = `	return {{$multicast}}(ctx, c.config, in, "{{$fullName}}")
}
`

var multicastCall = commonVariables +
	mcVar +
	multicastComment +
	multicastSignature +
	multicastBody
