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
{{$callData := use "gorums.CallData" .GenFile}}
{{$genFile := .GenFile}}
{{$context := use "context.Context" .GenFile}}
`

var rpcBody = `	cd := {{$callData}}{
		Message:  in,
		Method: "{{$fullName}}",
	}

	res, err := n.RawNode.RPCCall(ctx, cd)
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
