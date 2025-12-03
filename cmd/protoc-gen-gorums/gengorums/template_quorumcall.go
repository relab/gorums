package gengorums

// Common variables used in several template functions.
var commonVariables = `
{{$fullName := .Method.Desc.FullName}}
{{$method := .Method.GoName}}
{{$in := in .GenFile .Method}}
{{$out := out .GenFile .Method}}
{{$unexportOutput := unexport .Method.Output.GoIdent.GoName}}
`

var quorumCallComment = `
{{$comments := .Method.Comments.Leading}}
{{if ne $comments ""}}
{{$comments -}}
{{else}}
// {{$method}} is a quorum call invoked on all nodes in the configuration,
// with the same argument in. Use terminal methods like Majority(), First(),
// or Threshold(n) to retrieve the aggregated result.
//
// Example:
//   resp, err := {{$method}}(ctx, in).Majority()
{{end -}}
`

var qcVar = `
{{$genFile := .GenFile}}
{{$configContext := use "gorums.ConfigContext" .GenFile}}
{{$quorumCall := use "gorums.QuorumCall" .GenFile}}
{{$responses := use "gorums.Responses" .GenFile}}
{{$callOption := use "gorums.CallOption" .GenFile}}
`

var quorumCallSignature = `func {{$method}}(` +
	`ctx *{{$configContext}}, in *{{$in}}, ` +
	`opts ...{{$callOption}})` +
	` *{{$responses}}[*{{$out}}] {
`

var quorumCallBody = `	return {{$quorumCall}}[*{{$in}}, *{{$out}}](
		ctx, in, "{{$fullName}}",
		opts...,
	)
}
`

var quorumCall = commonVariables +
	qcVar +
	quorumCallComment +
	quorumCallSignature +
	quorumCallBody
