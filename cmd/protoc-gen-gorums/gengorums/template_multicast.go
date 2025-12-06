package gengorums

var multicastRefImports = `
{{if contains $out "."}}
// Reference imports to suppress errors if they are not otherwise used.
var _ {{$out}}
{{end}}
`

var mcVar = `
{{$genFile := .GenFile}}
{{$configContext := use "gorums.ConfigContext" .GenFile}}
{{$multicast := use "gorums.Multicast" .GenFile}}
{{$callOpt := use "gorums.CallOption" .GenFile}}
`

var multicastComment = `
{{$comments := .Method.Comments.Leading}}
{{if ne $comments ""}}
{{$comments -}}
{{else}}
// {{$method}} is a multicast call invoked on all nodes in the configuration in ctx.
// Use gorums.MapRequest to send different messages to each node. No replies are collected.
{{end -}}
`

var multicastSignature = `func {{$method}}(` +
	`ctx *{{$configContext}}, in *{{$in}}, ` +
	`opts ...{{$callOpt}}) error {
`

var multicastBody = `	return {{$multicast}}(ctx, in, "{{$fullName}}", opts...)
}
`

var multicastCall = commonVariables +
	mcVar +
	multicastRefImports +
	multicastComment +
	multicastSignature +
	multicastBody
