package gengorums

var multicastRefImports = `
{{if contains $out "."}}
// Reference imports to suppress errors if they are not otherwise used.
var _ {{$out}}
{{end}}
`

var mcVar = `
{{$genFile := .GenFile}}
{{$context := use "context.Context" .GenFile}}
{{$callOpt := use "gorums.CallOption" .GenFile}}
`

var multicastComment = `
{{$comments := .Method.Comments.Leading}}
{{if ne $comments ""}}
{{$comments -}}
{{else}}
// {{$method}} is a multicast call invoked on all nodes in configuration c,
// with the same argument in. Use WithPerNodeTransform to send different messages
// to each node. No replies are collected.
{{end -}}
`

var multicastSignature = `func (c *Configuration) {{$method}}(` +
	`ctx {{$context}}, in *{{$in}}, ` +
	`opts ...{{$callOpt}}) {
`

var multicastBody = `	c.RawConfiguration.Multicast(ctx, in, "{{$fullName}}", opts...)
}
`

var multicastCall = commonVariables +
	mcVar +
	multicastRefImports +
	multicastComment +
	multicastSignature +
	multicastBody
