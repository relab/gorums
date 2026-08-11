package gengorums

var remoteCallComment = `
{{$comments := .Method.Comments.Leading}}
{{if ne $comments ""}}
{{$comments -}}
{{else}}
// {{$method}} is an RPC call invoked on the node in ctx.
{{end -}}
`

var remoteCallVar = `
{{$genFile := .GenFile}}
{{$nodeContext := "NodeContext"}}
{{$rpc := use "gorumsimpl.RemoteCall" .GenFile}}
`

var remoteCallSignature = `func {{$method}}(ctx *{{$nodeContext}}, in *{{$in}}) (*{{$out}}, error) {
`

var remoteCallBody = ` return {{$rpc}}[*{{$in}}, *{{$out}}](ctx, in, "{{$fullName}}")
}
`

var remoteCall = commonVariables +
	remoteCallVar +
	remoteCallComment +
	remoteCallSignature +
	remoteCallBody
