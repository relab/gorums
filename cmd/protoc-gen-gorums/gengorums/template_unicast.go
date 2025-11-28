package gengorums

var unicastVar = rpcVar + `{{$callOpt := use "gorums.CallOption" .GenFile}}`

var unicastComment = `
{{$comments := .Method.Comments.Leading}}
{{if ne $comments ""}}
{{$comments -}}
{{else}}
// {{$method}} is a unicast call invoked on a single node.
// No reply is returned to the client.
{{end -}}
`

var unicastSignature = `func (n *Node) {{$method}}(` +
	`ctx {{$context}}, in *{{$in}}, opts ...{{$callOpt}}) {
`

var unicastBody = `	n.RawNode.Unicast(ctx, in, "{{$fullName}}", opts...)
}
`

var unicastCall = commonVariables +
	unicastVar +
	multicastRefImports +
	unicastComment +
	unicastSignature +
	unicastBody
