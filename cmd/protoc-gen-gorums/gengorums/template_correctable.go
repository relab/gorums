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
{{end -}}
`

var correctableVar = `
{{$correctableOut := outType .Method $out}}
{{$genFile := .GenFile}}
{{$configContext := use "gorums.ConfigContext" .GenFile}}
{{$quorumCall := use "gorums.QuorumCall" .GenFile}}
{{$quorumCallStream := use "gorums.QuorumCallStream" .GenFile}}
{{$callOption := use "gorums.CallOption" .GenFile}}
`

var correctableSignature = `func {{$method}}(` +
	`ctx *{{$configContext}}, in *{{$in}}, ` +
	`opts ...{{$callOption}}) ` +
	`*{{$correctableOut}} {
`

var correctableBody = `{{- if correctableStream .Method}}
	responses := {{$quorumCallStream}}[*{{$in}}, *{{$out}}](
		ctx, in, "{{$fullName}}",
		opts...,
	)
	return responses.WaitForLevel(responses.Size()/2 + 1)
{{- else}}
	responses := {{$quorumCall}}[*{{$in}}, *{{$out}}](
		ctx, in, "{{$fullName}}",
		opts...,
	)
	return responses.WaitForLevel(responses.Size()/2 + 1)
{{- end}}
}
`

var correctableCall = commonVariables +
	correctableVar +
	correctableCallComment +
	correctableSignature +
	correctableBody
