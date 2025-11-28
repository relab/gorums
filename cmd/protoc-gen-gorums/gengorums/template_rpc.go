package gengorums

var rpcComment = `
{{$comments := .Method.Comments.Leading}}
{{if ne $comments ""}}
{{$comments -}}
{{else}}
// {{$method}} is an RPC call invoked on a single node.
{{end -}}
`

var rpcSignature = `func (n *Node) {{$method}}(` +
	`ctx {{$context}}, in *{{$in}}) (resp *{{$out}}, err error) {
`

var rpcVar = `
{{$genFile := .GenFile}}
{{$context := use "context.Context" .GenFile}}
{{$_ := use "gorums.EnforceVersion" .GenFile}}
`

var rpcBody = `	res, err := n.RawNode.RPCCall(ctx, in, "{{$fullName}}")
	if err != nil {
		return nil, err
	}
	return res.(*{{$out}}), err
}
`

var rpcCall = commonVariables +
	rpcVar +
	rpcComment +
	rpcSignature +
	rpcBody
