package gengorums

var asyncCallComment = `
{{$comments := .Method.Comments.Leading}}
{{if ne $comments ""}}
{{$comments -}}
{{else}}
// {{$method}} asynchronously invokes a quorum call on the configuration in ctx
// and returns a {{$asyncOut}}, which can be used to inspect the quorum call
// reply and error when available.
{{end -}}
`

var asyncVar = `
{{$genFile := .GenFile}}
{{$configContext := use "gorums.ConfigContext" .GenFile}}
{{$asyncCall := use "gorums.AsyncCall" .GenFile}}
{{$callOption := use "gorums.CallOption" .GenFile}}
{{$asyncOut := outType .Method $out}}
`

var asyncSignature = `func {{$method}}(` +
	`ctx *{{$configContext}}, in *{{$in}}, ` +
	`opts ...{{$callOption}}) ` +
	`*{{$asyncOut}} {`

var asyncBody = `	return {{$asyncCall}}[*{{$in}}, *{{$out}}](
		ctx, in, "{{$fullName}}",
		opts...,
	)
}
`

var asyncCall = commonVariables +
	asyncVar +
	asyncCallComment +
	asyncSignature +
	asyncBody
