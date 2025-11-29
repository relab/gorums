package gengorums

// Common variables used in several template functions.
var commonVariables = `
{{$fullName := .Method.Desc.FullName}}
{{$method := .Method.GoName}}
{{$in := in .GenFile .Method}}
{{$out := out .GenFile .Method}}
{{$intOut := internalOut $out}}
{{$unexportOutput := unexport .Method.Output.GoIdent.GoName}}
`

var quorumCallComment = `
{{$comments := .Method.Comments.Leading}}
{{if ne $comments ""}}
{{$comments -}}
{{else}}
// {{$method}} is a quorum call invoked on all nodes in the configuration,
// with the same argument in, and returns a combined result.
// By default, a majority quorum function is used. To override the quorum function,
// use the gorums.WithQuorumFunc call option.
{{end -}}
`

var qcVar = `
{{$genFile := .GenFile}}
{{$configContext := use "gorums.ConfigContext" .GenFile}}
{{$quorumCallWithInterceptor := use "gorums.QuorumCallWithInterceptor" .GenFile}}
{{$majorityQuorum := use "gorums.MajorityQuorum" .GenFile}}
{{$callOption := use "gorums.CallOption" .GenFile}}
`

var quorumCallSignature = `func {{$method}}(` +
	`ctx *{{$configContext}}, in *{{$in}}, ` +
	`opts ...{{$callOption}})` +
	`(resp *{{$out}}, err error) {
`

var quorumCallBody = `	return {{$quorumCallWithInterceptor}}(
		ctx, in, "{{$fullName}}",
		{{$majorityQuorum}}[*{{$in}}, *{{$out}}],
		opts...,
	)
}
`

var quorumCall = commonVariables +
	qcVar +
	quorumCallComment +
	quorumCallSignature +
	quorumCallBody
