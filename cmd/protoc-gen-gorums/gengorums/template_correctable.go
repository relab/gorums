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
{{$correctableCall := use "gorums.CorrectableCall" .GenFile}}
{{$correctableStreamCall := use "gorums.CorrectableStreamCall" .GenFile}}
{{$callOption := use "gorums.CallOption" .GenFile}}
`

var correctableSignature = `func {{$method}}(` +
	`ctx *{{$configContext}}, in *{{$in}}, ` +
	`opts ...{{$callOption}}) ` +
	`*{{$correctableOut}} {
`

var correctableBody = `{{- if correctableStream .Method}}
	return {{$correctableStreamCall}}[*{{$in}}, *{{$out}}](
		ctx, in, "{{$fullName}}",
		opts...,
	)
{{- else}}
	return {{$correctableCall}}[*{{$in}}, *{{$out}}](
		ctx, in, "{{$fullName}}",
		opts...,
	)
{{- end}}
}
`

var correctableCall = commonVariables +
	correctableVar +
	correctableCallComment +
	correctableSignature +
	correctableBody
