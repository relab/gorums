package gengorums

var correctableCallComment = `
{{$comments := .Method.Comments.Leading}}
{{if ne $comments ""}}
{{$comments -}}
{{else}}
// {{$method}} asynchronously invokes a correctable quorum call on each node
// in the configuration in ctx and returns a {{$correctableOut}}, which can be used
// to inspect any replies or errors when available.
{{if correctableStream .Method -}}
// This method supports server-side preliminary replies (correctable stream).
{{end -}}
// By default, a majority quorum function is used. To override the quorum function,
// use the gorums.WithCorrectableQuorumFunc call option.
{{end -}}
`

var correctableVar = `
{{$correctableOut := outType .Method $out}}
{{$genFile := .GenFile}}
{{$configContext := use "gorums.ConfigContext" .GenFile}}
{{$correctableCall := use "gorums.CorrectableCall" .GenFile}}
{{$majorityCorrectableQuorum := use "gorums.MajorityCorrectableQuorum" .GenFile}}
{{$callOption := use "gorums.CallOption" .GenFile}}
`

var correctableSignature = `func {{$method}}(` +
	`ctx *{{$configContext}}, in *{{$in}}, ` +
	`opts ...{{$callOption}}) ` +
	`*{{$correctableOut}} {`

var correctableBody = `	return {{$correctableCall}}(
		ctx, in, "{{$fullName}}", {{correctableStream .Method}},
		{{$majorityCorrectableQuorum}}[*{{$in}}, *{{$out}}],
		opts...,
	)
}
`

var correctableCall = commonVariables +
	correctableVar +
	correctableCallComment +
	correctableSignature +
	correctableBody
