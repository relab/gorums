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
// It ignores the client's configuration. It is a method of {{$service}}Client
// only to keep the service's calls together and avoid package-level name
// conflicts.
//
// Send blocks until the send completes and returns any send error.
// Async returns a handle at once; Wait on the handle returns the send error.
{{end -}}
//
// Example:
//   err := {{.GoPackageName}}.{{$service}}Client{}.{{$method}}(ctx, node, in).Send()
//   h := {{.GoPackageName}}.{{$service}}Client{}.{{$method}}(ctx, node, in).Async()
//   err = h.Wait()
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
