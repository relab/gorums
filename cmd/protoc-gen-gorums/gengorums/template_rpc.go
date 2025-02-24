package gengorums

var rpcSignature = `func (n *Node[idType]) {{$method}}(` +
	`ctx {{$context}}, in *{{$in}}` +
	`{{perNodeFnType .GenFile .Method ", f"}}) (resp *{{$customOut}}, err error) {
`

var rpcVar = `
{{$callData := use "gorums.CallData" .GenFile}}
{{$genFile := .GenFile}}
{{$unexportMethod := unexport .Method.GoName}}
{{$context := use "context.Context" .GenFile}}
`

var rpcBody = `	cd := {{$callData}}{
		Message:  in,
		Method: "{{$fullName}}",
	}
{{- if hasPerNodeArg .Method}}
	{{$protoMessage := use "protoreflect.ProtoMessage" $genFile}}
	cd.PerNodeArgFn = func(req {{$protoMessage}}, nid idType) {{$protoMessage}} {
		return f(req.(*{{$in}}), nid)
	}
{{- end}}

	res, err := n.RawNode.RPCCall(ctx, cd)
	if err != nil {
		return nil, err
	}
	return res.(*{{$customOut}}), err
}
`

var rpcCall = commonVariables +
	rpcVar +
	quorumCallComment +
	rpcSignature +
	rpcBody
