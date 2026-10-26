package gengorums

var remoteCallComment = `
{{$comments := .Method.Comments.Leading}}
{{if ne $comments ""}}
{{$comments -}}
{{else}}
// {{$method}} executes a remote call on the given node and returns its reply.
// It does not use the client's configuration.
//
// Example:
//   resp, err := client.{{$method}}(ctx, node, in)
{{end -}}
`

var remoteCallVar = `
{{$genFile := .GenFile}}
{{$rpc := use "gorumsimpl.RemoteCall" .GenFile}}
{{$node := use "gorums.Node" .GenFile}}
`

var remoteCallSignature = `func (c {{$service}}Client) {{$method}}(ctx {{$context}}, node *{{$node}}, in *{{$in}}) (*{{$out}}, error) {
`

var remoteCallBody = ` return {{$rpc}}[*{{$in}}, *{{$out}}](ctx, node, in, "{{$fullName}}")
}
`

var remoteCall = commonVariables +
	remoteCallVar +
	remoteCallComment +
	remoteCallSignature +
	remoteCallBody
