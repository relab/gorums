package gengorums

var mcVar = `
{{$genFile := .GenFile}}
{{$multicast := use "gorumsimpl.Multicast" .GenFile}}
{{$onewayCall := use "gorums.OnewayCall" .GenFile}}
{{$impl := printf "%sClient" (unexport $service)}}
{{$local := localNames "c ctx in" $in $context $multicast}}
`

var multicastComment = `
{{- if .Decl}}
{{$comments := .Method.Comments.Leading}}
{{if ne $comments ""}}
{{$comments -}}
{{else}}
// {{$method}} starts a one-way call on the client's configuration; no reply
// is returned. Use gorums.MapRequest to send a different message to each node.
//
// Send blocks until every send completes and returns any send errors.
// Async returns a handle at once; Wait on the handle returns the send errors.
{{end -}}
//
// Example:
//
//	err := client.{{$method}}(ctx, in).Send()
//	h := client.{{$method}}(ctx, in).Async()
//	err = h.Wait()
{{end -}}
`

var multicastSignature = `{{if not .Decl}}func ({{$local.c}} {{$impl}}) {{end}}{{$method}}(` +
	`{{$local.ctx}} {{$context}}, {{$local.in}} *{{$in}})` +
	` *{{$onewayCall}}[*{{$in}}]`

var multicastBody = `{{if .Decl}}
{{else}} {
	return {{$multicast}}({{$local.ctx}}, {{$local.c}}.config, {{$local.in}}, "{{$fullName}}")
}
{{end}}`

var multicastCall = commonVariables +
	mcVar +
	multicastComment +
	multicastSignature +
	multicastBody
