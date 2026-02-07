package gengorums

var rpcComment = `
{{$comments := .Method.Comments.Leading}}
{{if ne $comments ""}}
{{$comments -}}
{{else}}
// {{$method}} is an RPC call invoked on the node in ctx.
{{end -}}
`

var rpcVar = `
{{$genFile := .GenFile}}
{{$nodeContext := "NodeContext"}}
{{$rpc := use "gorums.RPCCall" .GenFile}}
{{$_ := use "gorums.EnforceVersion" .GenFile}}
`

var rpcSignature = `func {{$method}}(ctx *{{$nodeContext}}, in *{{$in}}) (*{{$out}}, error) {
`

var rpcBody = ` return {{$rpc}}[*{{$in}}, *{{$out}}](ctx, in, "{{$fullName}}")
}
`

var rpcCall = commonVariables +
	rpcVar +
	rpcComment +
	rpcSignature +
	rpcBody
