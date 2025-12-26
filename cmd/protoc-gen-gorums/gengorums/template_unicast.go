package gengorums

var unicastVar = `
{{$genFile := .GenFile}}
{{$nodeContext := use "gorums.NodeContext" .GenFile}}
{{$unicast := use "gorums.Unicast" .GenFile}}
{{$callOpt := use "gorums.CallOption" .GenFile}}
`

var unicastComment = `
{{$comments := .Method.Comments.Leading}}
{{if ne $comments ""}}
{{$comments -}}
{{else}}
// {{$method}} is a unicast call invoked on the node in ctx.
// No reply is returned to the client.
{{end -}}
`

var unicastSignature = `func {{$method}}(` +
	`ctx *{{$nodeContext}}, in *{{$in}}, opts ...{{$callOpt}}) error {
`

var unicastBody = `	return {{$unicast}}(ctx, in, "{{$fullName}}", opts...)
}
`

var unicastCall = commonVariables +
	unicastVar +
	unicastComment +
	unicastSignature +
	unicastBody
