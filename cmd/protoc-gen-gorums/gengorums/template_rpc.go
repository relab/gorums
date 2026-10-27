package gengorums

var remoteCallComment = `
{{$comments := .Method.Comments.Leading}}
{{if ne $comments ""}}
{{$comments -}}
{{else}}
// {{$method}} executes a remote call on the given node and returns its reply.
// It ignores the client's configuration. It is a method of {{$service}}Client
// only to keep the service's calls together and avoid package-level name
// conflicts.
//
// Example:
//   resp, err := {{.GoPackageName}}.{{$service}}Client{}.{{$method}}(ctx, node, in)
{{end -}}
`

var remoteCallVar = `
{{$genFile := .GenFile}}
{{$rpc := use "gorumsimpl.RemoteCall" .GenFile}}
{{$node := use "gorums.Node" .GenFile}}
{{$local := localNames "c ctx node in" $in $out $context $rpc}}
`

var remoteCallSignature = `func ({{$local.c}} {{$service}}Client) {{$method}}(` +
	`{{$local.ctx}} {{$context}}, {{$local.node}} *{{$node}}, {{$local.in}} *{{$in}})` +
	` (*{{$out}}, error) {
`

var remoteCallBody = `	return {{$rpc}}[*{{$in}}, *{{$out}}]({{$local.ctx}}, {{$local.node}}, {{$local.in}}, "{{$fullName}}")
}
`

var remoteCall = commonVariables +
	remoteCallVar +
	remoteCallComment +
	remoteCallSignature +
	remoteCallBody
