package gengorums

var rpcVar = `
	{{- $callData := use "gorums.CallData" .GenFile}}
	{{- $genFile := .GenFile}}
	{{- $unexportMethod := unexport .Method.GoName}}
	{{- $context := use "context.Context" .GenFile}}
`

var rpcSignature = `func (n *{{$nodeName}}) {{$method}}(` +
	`ctx {{$context}}, in *{{$in}}` +
	`{{perNodeFnType .GenFile .Method ", f"}}) (resp *{{$out}}, err error) {
`

var rpcBody = `	cd := {{$callData}}{
		Message:  in,
		Method: "{{$fullName}}",
	}
	{{- if hasPerNodeArg .Method}}
		{{$protoMessage := use "proto.Message" $genFile}}
		cd.PerNodeArgFn = func(req {{$protoMessage}}, nid uint32) {{$protoMessage}} {
			return f(req.(*{{$in}}), nid)
		}
	{{- end}}

	res, err := n.RawNode.RPCCall(ctx, cd)
	if err != nil {
		return nil, err
	}
	return res.(*{{$out}}), err
}
`

var rpcCall = commonVariables +
	rpcVar +
	quorumCallComment +
	rpcSignature +
	rpcBody
