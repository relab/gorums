package gengorums

var asyncCallComment = `
{{$comments := .Method.Comments.Leading}}
{{if ne $comments ""}}
{{$comments -}}
{{else}}
// {{$method}} asynchronously invokes a quorum call on the configuration in ctx
// and returns a {{$asyncOut}}, which can be used to inspect the quorum call
// reply and error when available.
// By default, a majority quorum function is used. To override the quorum function,
// use the gorums.WithQuorumFunc call option.
{{end -}}
`

var asyncVar = `
{{$genFile := .GenFile}}
{{$configContext := use "gorums.ConfigContext" .GenFile}}
{{$asyncCall := use "gorums.AsyncCall" .GenFile}}
{{$majorityQuorum := use "gorums.MajorityQuorum" .GenFile}}
{{$callOption := use "gorums.CallOption" .GenFile}}
{{$asyncOut := outType .Method $out}}
`

var asyncSignature = `func {{$method}}(` +
	`ctx *{{$configContext}}, in *{{$in}}, ` +
	`opts ...{{$callOption}}) ` +
	`*{{$asyncOut}} {`

var asyncBody = `	return {{$asyncCall}}(
		ctx, in, "{{$fullName}}",
		{{$majorityQuorum}}[*{{$in}}, *{{$out}}],
		opts...,
	)
}
`

var asyncCall = commonVariables +
	asyncVar +
	asyncCallComment +
	asyncSignature +
	asyncBody
