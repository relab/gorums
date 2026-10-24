package gengorums

var remoteCallComment = `
{{$comments := .Method.Comments.Leading}}
{{if ne $comments ""}}
{{$comments -}}
{{else}}
// {{$method}} executes the {{$service}}.{{$method}} remote call on the client's node.
//
// Example:
//   resp, err := nodeClient.{{$method}}(ctx, in)
{{end -}}
`

var remoteCallVar = `
{{$genFile := .GenFile}}
{{$rpc := use "gorumsimpl.RemoteCall" .GenFile}}
`

var remoteCallSignature = `func (c {{$service}}ClientNode) {{$method}}(ctx {{$context}}, in *{{$in}}) (*{{$out}}, error) {
`

var remoteCallBody = ` return {{$rpc}}[*{{$in}}, *{{$out}}](ctx, c.node, in, "{{$fullName}}")
}
`

var remoteCall = commonVariables +
	remoteCallVar +
	remoteCallComment +
	remoteCallSignature +
	remoteCallBody
