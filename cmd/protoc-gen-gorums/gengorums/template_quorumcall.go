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

var quorumCallVariables = `
{{$genFile := .GenFile}}
{{$configContext := "ConfigContext"}}
{{$quorumCall := use "gorumsimpl.QuorumCall" .GenFile}}
{{$quorumCallStream := use "gorumsimpl.QuorumCallStream" .GenFile}}
{{$call := use "gorums.Call" .GenFile}}
`

var quorumCallSignature = `func {{$method}}(` +
	`ctx *{{$configContext}}, in *{{$in}})` +
	` *{{$call}}[*{{$in}}, *{{$out}}] {
`

var quorumCallBody = `	return {{$quorumCall}}[*{{$in}}, *{{$out}}](
		ctx, in, "{{$fullName}}",
	)
}
`

var quorumCall = commonVariables +
	quorumCallVariables +
	quorumCallComment +
	quorumCallSignature +
	quorumCallBody

// Streaming quorum call template
var quorumCallStreamComment = `
{{$comments := .Method.Comments.Leading}}
{{if ne $comments ""}}
{{$comments -}}
{{else}}
// {{$method}} is a streaming quorum call where the server can send multiple responses.
// The response iterator continues until the context is canceled.
//
// Example:
//   corr := {{$method}}(ctx, in).Correctable(2)
//   <-corr.Watch(2)
//   resp, level, err := corr.Get()
{{end -}}
`

var quorumCallStreamBody = `	return {{$quorumCallStream}}[*{{$in}}, *{{$out}}](
		ctx, in, "{{$fullName}}",
	)
}
`

var quorumCallStream = commonVariables +
	quorumCallVariables +
	quorumCallStreamComment +
	quorumCallSignature +
	quorumCallStreamBody
