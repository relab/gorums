package gengorums

var rpcComment = `
{{$comments := .Method.Comments.Leading}}
{{if ne $comments ""}}
{{$comments -}}
{{else}}
// {{$method}} is an RPC call invoked on the node in ctx.
{{end -}}
`

var rpcSignature = `func {{$method}}(` +
	`ctx *{{$nodeContext}}, in *{{$in}}) (resp *{{$out}}, err error) {
`

var rpcVar = `
{{$genFile := .GenFile}}
{{$nodeContext := "NodeContext"}}
{{$rpcCall := use "gorums.RPCCall" .GenFile}}
{{$_ := use "gorums.EnforceVersion" .GenFile}}
`

var rpcBody = `	res, err := {{$rpcCall}}(ctx, in, "{{$fullName}}")
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
